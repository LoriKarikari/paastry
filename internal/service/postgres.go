package service

import (
	"context"
	"fmt"

	paastryv1 "github.com/LoriKarikari/paastry/gen/paastry/v1"
	"github.com/LoriKarikari/paastry/internal/docker"
	"github.com/google/uuid"
)

type postgresManager struct {
	docker DockerClient
}

func NewPostgresManager(docker DockerClient) Manager {
	return &postgresManager{docker: docker}
}

func (m *postgresManager) Provision(ctx context.Context, spec ProvisionSpec) (*paastryv1.Service, error) {
	id := uuid.NewString()
	svcName := "paastry-postgres-" + spec.Name

	_, err := m.docker.ServiceCreate(ctx, docker.ServiceCreateSpec{
		Name:    svcName,
		Image:   "postgres:16-alpine",
		Network: spec.Network,
		Env: []string{
			"POSTGRES_DB=" + spec.DBName,
			"POSTGRES_USER=" + spec.DBUser,
			"POSTGRES_PASSWORD=" + spec.DBPassword,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create postgres service: %w", err)
	}

	return &paastryv1.Service{
		Id:    id,
		Name:  spec.Name,
		Type:  paastryv1.ServiceType_SERVICE_TYPE_POSTGRES,
		State: paastryv1.ServiceState_SERVICE_STATE_RUNNING,
	}, nil
}
