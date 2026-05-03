package jobs

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Type string

const TypeProvision Type = "provision"

type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

// Options configures a Runner.
type Options struct {
	MaxConcurrent int
}

// Step is one named unit of work within a job.
type Step struct {
	Name string
	Run  func(ctx context.Context, output io.Writer) error
}

// Spec describes a job to enqueue.
type Spec struct {
	ServiceID string
	Type      Type
	Steps     []Step
}

// Job is the persisted view of a job and its steps.
type Job struct {
	ID        string
	ServiceID string
	Type      Type
	Status    Status
	Steps     []StepResult
}

// StepResult is the persisted view of a step.
type StepResult struct {
	Name   string
	Status Status
	Output string
}

// Runner executes jobs in a bounded pool and persists state.
type Runner struct {
	db      *sql.DB
	sem     chan struct{}
	mu      sync.Mutex
	done    map[string]chan struct{}
	cancels map[string]context.CancelFunc
}

// NewRunner creates a Runner backed by db.
func NewRunner(ctx context.Context, db *sql.DB, opts Options) (*Runner, error) {
	if opts.MaxConcurrent <= 0 {
		return nil, errors.New("max concurrent must be positive")
	}
	db.SetMaxOpenConns(1)
	if err := initialize(ctx, db); err != nil {
		return nil, err
	}
	return &Runner{
		db:      db,
		sem:     make(chan struct{}, opts.MaxConcurrent),
		done:    make(map[string]chan struct{}),
		cancels: make(map[string]context.CancelFunc),
	}, nil
}

// Enqueue persists a job and starts execution.
func (r *Runner) Enqueue(ctx context.Context, spec Spec) (*Job, error) {
	if len(spec.Steps) == 0 {
		return nil, errors.New("job must have at least one step")
	}
	jobID := uuid.NewString()
	if err := r.insertJob(ctx, jobID, spec); err != nil {
		return nil, err
	}
	jobCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	r.mu.Lock()
	r.done[jobID] = done
	r.cancels[jobID] = cancel
	r.mu.Unlock()

	go func() {
		defer close(done)
		defer func() {
			r.mu.Lock()
			delete(r.cancels, jobID)
			r.mu.Unlock()
		}()
		r.runJob(jobCtx, jobID, spec)
	}()

	return r.Get(ctx, jobID)
}

// Get returns a persisted job by ID.
func (r *Runner) Get(ctx context.Context, jobID string) (*Job, error) {
	var job Job
	var typ string
	var status string
	if err := r.db.QueryRowContext(
		ctx,
		`select id, service_id, type, status from jobs where id = ?`,
		jobID,
	).Scan(&job.ID, &job.ServiceID, &typ, &status); err != nil {
		return nil, fmt.Errorf("get job: %w", err)
	}
	job.Type = Type(typ)
	job.Status = Status(status)

	rows, err := r.db.QueryContext(
		ctx,
		`select name, status, output from job_steps where job_id = ? order by step_order`,
		jobID,
	)
	if err != nil {
		return nil, fmt.Errorf("list job steps: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var step StepResult
		var stepStatus string
		if err := rows.Scan(&step.Name, &stepStatus, &step.Output); err != nil {
			return nil, fmt.Errorf("scan job step: %w", err)
		}
		step.Status = Status(stepStatus)
		job.Steps = append(job.Steps, step)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate job steps: %w", err)
	}
	return &job, nil
}

// Cancel requests graceful cancellation.
func (r *Runner) Cancel(_ context.Context, jobID string) error {
	r.mu.Lock()
	cancel, ok := r.cancels[jobID]
	r.mu.Unlock()
	if !ok {
		return nil
	}
	cancel()
	return nil
}

// Wait blocks until a job finishes.
func (r *Runner) Wait(ctx context.Context, jobID string) error {
	r.mu.Lock()
	done, ok := r.done[jobID]
	r.mu.Unlock()
	if !ok {
		return nil
	}
	select {
	case <-ctx.Done():
		return fmt.Errorf("wait job: %w", ctx.Err())
	case <-done:
		return nil
	}
}

// Close releases runner resources.
func (r *Runner) Close() {}

func initialize(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `
		create table if not exists jobs (
			id text primary key,
			service_id text not null,
			type text not null,
			status text not null,
			created_at text not null,
			updated_at text not null
		);
		create table if not exists job_steps (
			job_id text not null,
			step_order integer not null,
			name text not null,
			status text not null,
			output text not null default '',
			primary key (job_id, step_order)
		);
	`); err != nil {
		return fmt.Errorf("initialize jobs tables: %w", err)
	}
	return nil
}

func (r *Runner) insertJob(ctx context.Context, jobID string, spec Spec) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin insert job: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(
		ctx,
		`insert into jobs (id, service_id, type, status, created_at, updated_at) values (?, ?, ?, ?, ?, ?)`,
		jobID,
		spec.ServiceID,
		string(spec.Type),
		string(StatusPending),
		now,
		now,
	); err != nil {
		return fmt.Errorf("insert job: %w", err)
	}
	for i, step := range spec.Steps {
		if _, err := tx.ExecContext(
			ctx,
			`insert into job_steps (job_id, step_order, name, status) values (?, ?, ?, ?)`,
			jobID,
			i,
			step.Name,
			string(StatusPending),
		); err != nil {
			return fmt.Errorf("insert job step: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit insert job: %w", err)
	}
	return nil
}

func (r *Runner) runJob(ctx context.Context, jobID string, spec Spec) {
	select {
	case r.sem <- struct{}{}:
		defer func() { <-r.sem }()
	case <-ctx.Done():
		_ = r.updateJobStatus(context.Background(), jobID, StatusCancelled)
		return
	}
	if err := r.updateJobStatus(ctx, jobID, StatusRunning); err != nil {
		return
	}
	for i, step := range spec.Steps {
		var output bytes.Buffer
		if err := r.updateStep(ctx, jobID, i, StatusRunning, ""); err != nil {
			return
		}
		if err := step.Run(ctx, &output); err != nil {
			status := StatusFailed
			if errors.Is(err, context.Canceled) {
				status = StatusCancelled
			}
			_ = r.updateStep(context.Background(), jobID, i, status, output.String())
			_ = r.updateJobStatus(context.Background(), jobID, status)
			return
		}
		if err := r.updateStep(ctx, jobID, i, StatusSucceeded, output.String()); err != nil {
			return
		}
	}
	_ = r.updateJobStatus(ctx, jobID, StatusSucceeded)
}

func (r *Runner) updateJobStatus(ctx context.Context, jobID string, status Status) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.db.ExecContext(ctx, `update jobs set status = ?, updated_at = ? where id = ?`, string(status), now, jobID)
	if err != nil {
		return fmt.Errorf("update job status: %w", err)
	}
	return nil
}

func (r *Runner) updateStep(ctx context.Context, jobID string, order int, status Status, output string) error {
	_, err := r.db.ExecContext(
		ctx,
		`update job_steps set status = ?, output = ? where job_id = ? and step_order = ?`,
		string(status),
		output,
		jobID,
		order,
	)
	if err != nil {
		return fmt.Errorf("update job step: %w", err)
	}
	return nil
}
