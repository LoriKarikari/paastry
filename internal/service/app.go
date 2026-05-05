package service

import (
	"context"
	"fmt"

	"github.com/LoriKarikari/paastry/internal/docker"
	"github.com/google/uuid"
)

type appManager struct {
	docker DockerClient
}

func newAppManager(docker DockerClient) manager {
	return &appManager{docker: docker}
}

func (m *appManager) Provision(ctx context.Context, spec provisionerSpec) (*Record, error) {
	id := uuid.NewString()
	svcName := "paastry-app-" + spec.Name

	_, err := m.docker.ServiceCreate(ctx, docker.ServiceCreateSpec{
		Name:    svcName,
		Image:   spec.Image,
		Network: spec.Network,
		Port:    spec.Port,
	})
	if err != nil {
		return nil, fmt.Errorf("create app service: %w", err)
	}

	return &Record{
		ID:       id,
		TenantID: spec.TenantID,
		Name:     spec.Name,
		Type:     TypeApp,
		State:    StateRunning,
	}, nil
}

func (m *appManager) Deploy(ctx context.Context, spec provisionerSpec, serviceID string) (*Record, error) {
	svcName := "paastry-app-" + spec.Name

	_, err := m.docker.ServiceCreate(ctx, docker.ServiceCreateSpec{
		Name:    svcName,
		Image:   spec.Image,
		Network: spec.Network,
		Port:    spec.Port,
	})
	if err != nil {
		return nil, fmt.Errorf("deploy app service: %w", err)
	}

	return &Record{
		ID:       serviceID,
		TenantID: spec.TenantID,
		Name:     spec.Name,
		Type:     TypeApp,
		State:    StateRunning,
	}, nil
}
