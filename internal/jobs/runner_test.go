package jobs_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/LoriKarikari/paastry/internal/jobs"
	_ "modernc.org/sqlite"
)

func TestRunnerExecutesJobAndPersistsSteps(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite database: %v", err)
	}
	defer db.Close()

	runner, err := jobs.NewRunner(context.Background(), db, jobs.Options{MaxConcurrent: 1})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	defer runner.Close()

	job, err := runner.Enqueue(context.Background(), jobs.Spec{
		ServiceID: "svc-1",
		Type:      jobs.TypeProvision,
		Steps: []jobs.Step{
			{
				Name: "create service",
				Run: func(_ context.Context, output io.Writer) error {
					if _, err := fmt.Fprint(output, "created"); err != nil {
						return fmt.Errorf("write output: %w", err)
					}
					return nil
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("enqueue job: %v", err)
	}

	if err := runner.Wait(context.Background(), job.ID); err != nil {
		t.Fatalf("wait job: %v", err)
	}

	got, err := runner.Get(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if got.Status != jobs.StatusSucceeded {
		t.Fatalf("job status = %q, want %q", got.Status, jobs.StatusSucceeded)
	}
	if len(got.Steps) != 1 {
		t.Fatalf("step count = %d, want 1", len(got.Steps))
	}
	step := got.Steps[0]
	if step.Name != "create service" {
		t.Fatalf("step name = %q", step.Name)
	}
	if step.Status != jobs.StatusSucceeded {
		t.Fatalf("step status = %q, want %q", step.Status, jobs.StatusSucceeded)
	}
	if step.Output != "created" {
		t.Fatalf("step output = %q, want %q", step.Output, "created")
	}
}

func TestRunnerMarksJobFailedWhenStepFails(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite database: %v", err)
	}
	defer db.Close()

	runner, err := jobs.NewRunner(context.Background(), db, jobs.Options{MaxConcurrent: 1})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	defer runner.Close()

	ranSecond := false
	job, err := runner.Enqueue(context.Background(), jobs.Spec{
		ServiceID: "svc-1",
		Type:      jobs.TypeProvision,
		Steps: []jobs.Step{
			{
				Name: "create service",
				Run: func(_ context.Context, output io.Writer) error {
					if _, err := fmt.Fprint(output, "partial"); err != nil {
						return fmt.Errorf("write output: %w", err)
					}
					return errors.New("boom")
				},
			},
			{
				Name: "wait for health",
				Run: func(context.Context, io.Writer) error {
					ranSecond = true
					return nil
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("enqueue job: %v", err)
	}

	if err := runner.Wait(context.Background(), job.ID); err != nil {
		t.Fatalf("wait job: %v", err)
	}

	got, err := runner.Get(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if got.Status != jobs.StatusFailed {
		t.Fatalf("job status = %q, want %q", got.Status, jobs.StatusFailed)
	}
	if got.Steps[0].Status != jobs.StatusFailed {
		t.Fatalf("first step status = %q, want %q", got.Steps[0].Status, jobs.StatusFailed)
	}
	if got.Steps[0].Output != "partial" {
		t.Fatalf("first step output = %q, want %q", got.Steps[0].Output, "partial")
	}
	if got.Steps[1].Status != jobs.StatusPending {
		t.Fatalf("second step status = %q, want %q", got.Steps[1].Status, jobs.StatusPending)
	}
	if ranSecond {
		t.Fatal("expected runner to stop after first failed step")
	}
}

func TestRunnerRespectsConcurrencyLimit(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite database: %v", err)
	}
	defer db.Close()

	runner, err := jobs.NewRunner(context.Background(), db, jobs.Options{MaxConcurrent: 1})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	defer runner.Close()

	started := make(chan string, 2)
	release := make(chan struct{})
	makeSpec := func(serviceID string) jobs.Spec {
		return jobs.Spec{
			ServiceID: serviceID,
			Type:      jobs.TypeProvision,
			Steps: []jobs.Step{
				{
					Name: "block",
					Run: func(ctx context.Context, _ io.Writer) error {
						started <- serviceID
						select {
						case <-ctx.Done():
							return ctx.Err()
						case <-release:
							return nil
						}
					},
				},
			},
		}
	}

	first, err := runner.Enqueue(context.Background(), makeSpec("svc-1"))
	if err != nil {
		t.Fatalf("enqueue first job: %v", err)
	}

	firstStarted := <-started
	second, err := runner.Enqueue(context.Background(), makeSpec("svc-2"))
	if err != nil {
		t.Fatalf("enqueue second job: %v", err)
	}

	select {
	case got := <-started:
		t.Fatalf("second job started before first released: %s", got)
	case <-time.After(25 * time.Millisecond):
	}

	release <- struct{}{}
	if err := runner.Wait(context.Background(), first.ID); err != nil {
		t.Fatalf("wait first job: %v", err)
	}
	secondStarted := <-started
	if firstStarted == secondStarted {
		t.Fatalf("expected different services to start, got %q twice", firstStarted)
	}
	release <- struct{}{}
	if err := runner.Wait(context.Background(), second.ID); err != nil {
		t.Fatalf("wait second job: %v", err)
	}
}

func TestRunnerCancelsRunningJob(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite database: %v", err)
	}
	defer db.Close()

	runner, err := jobs.NewRunner(context.Background(), db, jobs.Options{MaxConcurrent: 1})
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	defer runner.Close()

	started := make(chan struct{})
	job, err := runner.Enqueue(context.Background(), jobs.Spec{
		ServiceID: "svc-1",
		Type:      jobs.TypeProvision,
		Steps: []jobs.Step{
			{
				Name: "wait",
				Run: func(ctx context.Context, output io.Writer) error {
					close(started)
					<-ctx.Done()
					if _, err := fmt.Fprint(output, "cancelled"); err != nil {
						return fmt.Errorf("write output: %w", err)
					}
					return ctx.Err()
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("enqueue job: %v", err)
	}
	<-started

	if err := runner.Cancel(context.Background(), job.ID); err != nil {
		t.Fatalf("cancel job: %v", err)
	}
	if err := runner.Wait(context.Background(), job.ID); err != nil {
		t.Fatalf("wait job: %v", err)
	}

	got, err := runner.Get(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if got.Status != jobs.StatusCancelled {
		t.Fatalf("job status = %q, want %q", got.Status, jobs.StatusCancelled)
	}
	if got.Steps[0].Status != jobs.StatusCancelled {
		t.Fatalf("step status = %q, want %q", got.Steps[0].Status, jobs.StatusCancelled)
	}
	if got.Steps[0].Output != "cancelled" {
		t.Fatalf("step output = %q, want %q", got.Steps[0].Output, "cancelled")
	}
}
