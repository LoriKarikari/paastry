package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/LoriKarikari/paastry/internal/docker"
	"github.com/LoriKarikari/paastry/internal/service"
)

type fakeDocker struct {
	networks map[string]struct{}
	services map[string]struct{}
}

func newFakeDocker() *fakeDocker {
	return &fakeDocker{
		networks: make(map[string]struct{}),
		services: make(map[string]struct{}),
	}
}

func (f *fakeDocker) NetworkCreate(_ context.Context, spec docker.NetworkCreateSpec) (*docker.Network, error) {
	f.networks[spec.Name] = struct{}{}
	return &docker.Network{ID: "net-" + spec.Name, Name: spec.Name}, nil
}

func (f *fakeDocker) NetworkExists(_ context.Context, name string) (bool, error) {
	_, ok := f.networks[name]
	return ok, nil
}

func (f *fakeDocker) ServiceCreate(_ context.Context, spec docker.ServiceCreateSpec) (*docker.Service, error) {
	f.services[spec.Name] = struct{}{}
	return &docker.Service{ID: "svc-" + spec.Name, Name: spec.Name}, nil
}

func (f *fakeDocker) ServiceExists(_ context.Context, name string) (bool, error) {
	_, ok := f.services[name]
	return ok, nil
}

func (f *fakeDocker) ServiceRemove(_ context.Context, _ string) error {
	return nil
}

func (f *fakeDocker) NetworkRemove(_ context.Context, _ string) error {
	return nil
}

func TestPostgresProvision(t *testing.T) {
	ctx := context.Background()
	dc := newFakeDocker()
	mgr := service.NewPostgresManager(dc)

	svc, err := mgr.Provision(ctx, service.ProvisionSpec{
		Name:       "mydb",
		TenantID:   "tenant-1",
		Network:    "paastry-tenant-default",
		DBName:     "appdb",
		DBUser:     "appuser",
		DBPassword: "s3cret",
	})
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if svc.Name != "mydb" {
		t.Fatalf("name = %q, want mydb", svc.Name)
	}
	if svc.Id == "" {
		t.Fatal("service id is empty")
	}

	exists, _ := dc.ServiceExists(ctx, "paastry-postgres-mydb")
	if !exists {
		t.Fatal("docker service was not created")
	}
}

type failingDocker struct{}

func (failingDocker) NetworkCreate(_ context.Context, _ docker.NetworkCreateSpec) (*docker.Network, error) {
	return nil, errors.New("docker down")
}

func (failingDocker) NetworkExists(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (failingDocker) ServiceCreate(_ context.Context, _ docker.ServiceCreateSpec) (*docker.Service, error) {
	return nil, errors.New("docker down")
}

func (failingDocker) ServiceExists(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (failingDocker) ServiceRemove(_ context.Context, _ string) error {
	return nil
}

func (failingDocker) NetworkRemove(_ context.Context, _ string) error {
	return nil
}

func TestPostgresProvisionDockerError(t *testing.T) {
	ctx := context.Background()
	mgr := service.NewPostgresManager(failingDocker{})

	_, err := mgr.Provision(ctx, service.ProvisionSpec{
		Name:       "mydb",
		TenantID:   "tenant-1",
		Network:    "paastry-tenant-default",
		DBName:     "appdb",
		DBUser:     "appuser",
		DBPassword: "s3cret",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
