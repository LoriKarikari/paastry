package service

import (
	"context"

	paastryv1 "github.com/LoriKarikari/paastry/gen/paastry/v1"
	"github.com/LoriKarikari/paastry/internal/docker"
)

type DockerClient interface {
	NetworkCreate(ctx context.Context, spec docker.NetworkCreateSpec) (*docker.Network, error)
	NetworkExists(ctx context.Context, name string) (bool, error)
	NetworkRemove(ctx context.Context, id string) error
	ServiceCreate(ctx context.Context, spec docker.ServiceCreateSpec) (*docker.Service, error)
	ServiceExists(ctx context.Context, name string) (bool, error)
	ServiceRemove(ctx context.Context, id string) error
}

type ProvisionSpec struct {
	Name       string
	TenantID   string
	Network    string
	Image      string
	Port       uint32
	DBName     string
	DBUser     string
	DBPassword string
}

type Manager interface {
	Provision(ctx context.Context, spec ProvisionSpec) (*paastryv1.Service, error)
	Deploy(ctx context.Context, spec ProvisionSpec, serviceID string) (*paastryv1.Service, error)
}
