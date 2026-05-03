//go:build integration

package docker_test

import (
	"context"
	"net/netip"
	"testing"

	"github.com/LoriKarikari/paastry/internal/docker"
	"github.com/google/uuid"
)

func mustPrefix(t *testing.T, s string) netip.Prefix {
	t.Helper()
	p, err := netip.ParsePrefix(s)
	if err != nil {
		t.Fatalf("parse prefix %q: %v", s, err)
	}
	return p
}

func mustAddr(t *testing.T, s string) netip.Addr {
	t.Helper()
	a, err := netip.ParseAddr(s)
	if err != nil {
		t.Fatalf("parse addr %q: %v", s, err)
	}
	return a
}

func TestClientNetworkLifecycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cli, err := docker.New()
	if err != nil {
		t.Skipf("docker not available: %v", err)
	}
	name := "paastry-test-" + uuid.NewString()[:8]

	net, err := cli.NetworkCreate(ctx, docker.NetworkCreateSpec{
		Name:    name,
		Driver:  "overlay",
		Subnet:  mustPrefix(t, "10.99.0.0/24"),
		Gateway: mustAddr(t, "10.99.0.1"),
	})
	if err != nil {
		t.Fatalf("create network: %v", err)
	}
	t.Cleanup(func() { _ = cli.NetworkRemove(context.Background(), net.ID) })

	if net.ID == "" || net.Name != name {
		t.Fatalf("got %+v, want non-empty ID and name=%q", net, name)
	}

	ok, err := cli.NetworkExists(ctx, name)
	if err != nil {
		t.Fatalf("network exists: %v", err)
	}
	if !ok {
		t.Fatal("expected network to exist")
	}

	insp, err := cli.NetworkInspect(ctx, net.ID)
	if err != nil {
		t.Fatalf("network inspect: %v", err)
	}
	if insp.ID != net.ID {
		t.Fatalf("inspect ID = %q, want %q", insp.ID, net.ID)
	}

	if err := cli.NetworkRemove(ctx, net.ID); err != nil {
		t.Fatalf("network remove: %v", err)
	}

	ok, err = cli.NetworkExists(ctx, name)
	if err != nil {
		t.Fatalf("network exists after remove: %v", err)
	}
	if ok {
		t.Fatal("expected network to be gone")
	}
}

func TestClientServiceLifecycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cli, err := docker.New()
	if err != nil {
		t.Skipf("docker not available: %v", err)
	}
	name := "paastry-test-" + uuid.NewString()[:8]

	// Create the overlay network the service will attach to.
	netName := "paastry-test-net-" + uuid.NewString()[:8]
	net, err := cli.NetworkCreate(ctx, docker.NetworkCreateSpec{
		Name:    netName,
		Driver:  "overlay",
		Subnet:  mustPrefix(t, "10.98.0.0/24"),
		Gateway: mustAddr(t, "10.98.0.1"),
	})
	if err != nil {
		t.Fatalf("create network: %v", err)
	}
	t.Cleanup(func() { _ = cli.NetworkRemove(context.Background(), net.ID) })

	svc, err := cli.ServiceCreate(ctx, docker.ServiceCreateSpec{
		Name:    name,
		Image:   "alpine:3.21",
		Env:     []string{"FOO=bar"},
		Network: netName,
	})
	if err != nil {
		t.Fatalf("service create: %v", err)
	}
	t.Cleanup(func() { _ = cli.ServiceRemove(context.Background(), svc.ID) })

	if svc.ID == "" || svc.Name != name {
		t.Fatalf("got %+v", svc)
	}

	ok, err := cli.ServiceExists(ctx, name)
	if err != nil {
		t.Fatalf("service exists: %v", err)
	}
	if !ok {
		t.Fatal("expected service to exist")
	}

	if err := cli.ServiceRemove(ctx, svc.ID); err != nil {
		t.Fatalf("service remove: %v", err)
	}

	ok, err = cli.ServiceExists(ctx, name)
	if err != nil {
		t.Fatalf("service exists after remove: %v", err)
	}
	if ok {
		t.Fatal("expected service to be gone")
	}
}
