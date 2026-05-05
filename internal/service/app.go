package service

import (
	"context"
	"fmt"

	paastryv1 "github.com/LoriKarikari/paastry/gen/paastry/v1"
	"github.com/LoriKarikari/paastry/internal/docker"
	"github.com/google/uuid"
)

type appManager struct {
	docker DockerClient
}

func NewAppManager(docker DockerClient) Manager {
	return &appManager{docker: docker}
}

func (m *appManager) Provision(ctx context.Context, spec ProvisionSpec) (*paastryv1.Service, error) {
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

	return &paastryv1.Service{
		Id:    id,
		Name:  spec.Name,
		Type:  paastryv1.ServiceType_SERVICE_TYPE_APP,
		State: paastryv1.ServiceState_SERVICE_STATE_RUNNING,
	}, nil
}

func (m *appManager) Deploy(ctx context.Context, spec ProvisionSpec, serviceID string) (*paastryv1.Service, error) {
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

	return &paastryv1.Service{
		Id:    serviceID,
		Name:  spec.Name,
		Type:  paastryv1.ServiceType_SERVICE_TYPE_APP,
		State: paastryv1.ServiceState_SERVICE_STATE_RUNNING,
	}, nil
}
