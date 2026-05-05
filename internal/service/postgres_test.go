package service_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/LoriKarikari/paastry/internal/docker"
	"github.com/LoriKarikari/paastry/internal/service"
	_ "modernc.org/sqlite"
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

func TestLifecycleProvisionsPostgres(t *testing.T) {
	ctx := context.Background()
	db := newServiceDB(t)
	dc := newFakeDocker()
	lifecycle := service.NewLifecycle(db, dc)

	rec, err := lifecycle.Provision(ctx, service.ProvisionSpec{
		Name:       "mydb",
		TenantID:   "tenant-1",
		Type:       service.TypePostgres,
		DBName:     "appdb",
		DBUser:     "appuser",
		DBPassword: "s3cret",
	})
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if rec.Name != "mydb" {
		t.Fatalf("name = %q, want mydb", rec.Name)
	}
	if rec.ID == "" {
		t.Fatal("service id is empty")
	}

	exists, _ := dc.ServiceExists(ctx, "paastry-postgres-mydb")
	if !exists {
		t.Fatal("docker service was not created")
	}

	got, err := lifecycle.Get(ctx, rec.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Type != service.TypePostgres || got.State != service.StateRunning {
		t.Fatalf("got type/state = %s/%s", got.Type, got.State)
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

func TestLifecycleProvisionDockerError(t *testing.T) {
	ctx := context.Background()
	db := newServiceDB(t)
	lifecycle := service.NewLifecycle(db, failingDocker{})

	_, err := lifecycle.Provision(ctx, service.ProvisionSpec{
		Name:       "mydb",
		TenantID:   "tenant-1",
		Type:       service.TypePostgres,
		DBName:     "appdb",
		DBUser:     "appuser",
		DBPassword: "s3cret",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func newServiceDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`
		create table tenants (
			id text primary key,
			name text not null unique,
			network_name text not null
		);
		insert into tenants (id, name, network_name) values ('tenant-1', 'default', 'paastry-tenant-default');
		create table services (
			id text primary key,
			tenant_id text not null,
			name text not null,
			type text not null,
			state text not null
		);
	`)
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	return db
}
