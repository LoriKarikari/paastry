package service

import (
	"context"
	"database/sql"

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

type Type string

const (
	TypePostgres Type = "postgres"
	TypeApp      Type = "app"
)

type State string

const StateRunning State = "running"

type ProvisionSpec struct {
	Name       string
	TenantID   string
	Type       Type
	Image      string
	Port       uint32
	DBName     string
	DBUser     string
	DBPassword string
}

type Record struct {
	ID       string
	TenantID string
	Name     string
	Type     Type
	State    State
}

type Lifecycle struct {
	db       *sql.DB
	managers map[Type]manager
}

type manager interface {
	Provision(ctx context.Context, spec provisionerSpec) (*Record, error)
	Deploy(ctx context.Context, spec provisionerSpec, serviceID string) (*Record, error)
}

type provisionerSpec struct {
	Name       string
	TenantID   string
	Network    string
	Image      string
	Port       uint32
	DBName     string
	DBUser     string
	DBPassword string
}

func NewLifecycle(db *sql.DB, docker DockerClient) *Lifecycle {
	return &Lifecycle{
		db: db,
		managers: map[Type]manager{
			TypePostgres: newPostgresManager(docker),
			TypeApp:      newAppManager(docker),
		},
	}
}
