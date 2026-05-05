package service

import (
	"context"
	"fmt"

	"github.com/LoriKarikari/paastry/internal/docker"
	"github.com/google/uuid"
)

type postgresManager struct {
	docker DockerClient
}

func newPostgresManager(docker DockerClient) manager {
	return &postgresManager{docker: docker}
}

func (m *postgresManager) Provision(ctx context.Context, spec provisionerSpec) (*Record, error) {
	id := uuid.NewString()
	svcName := "paastry-postgres-" + spec.Name

	_, err := m.docker.ServiceCreate(ctx, docker.ServiceCreateSpec{
		Name:    svcName,
		Image:   "postgres:18-alpine",
		Network: spec.Network,
		Env: []string{
			"POSTGRES_DB=" + spec.DBName,
			"POSTGRES_USER=" + spec.DBUser,
			"POSTGRES_PASSWORD=" + spec.DBPassword,
		},
		Mounts: []docker.Mount{
			{Source: "paastry-pg-" + spec.Name + "-data", Target: "/var/lib/postgresql/data"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create postgres service: %w", err)
	}

	return &Record{
		ID:       id,
		TenantID: spec.TenantID,
		Name:     spec.Name,
		Type:     TypePostgres,
		State:    StateRunning,
	}, nil
}

func (m *postgresManager) Deploy(_ context.Context, _ provisionerSpec, _ string) (*Record, error) {
	return nil, fmt.Errorf("deploy not supported for postgres")
}
