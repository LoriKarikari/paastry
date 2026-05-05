package docker

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/api/types/swarm"
	"github.com/moby/moby/client"
)

type Client struct {
	inner *client.Client
}

type NetworkCreateSpec struct {
	Name    string
	Driver  string
	Subnet  netip.Prefix
	Gateway netip.Addr
}

type Network struct {
	ID   string
	Name string
}

type Mount struct {
	Source string
	Target string
}

type ServiceCreateSpec struct {
	Name    string
	Image   string
	Env     []string
	Network string
	Mounts  []Mount
}

type Service struct {
	ID   string
	Name string
}

func New() (*Client, error) {
	cli, err := client.New(client.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	return &Client{inner: cli}, nil
}

func (c *Client) NetworkCreate(ctx context.Context, spec NetworkCreateSpec) (*Network, error) {
	resp, err := c.inner.NetworkCreate(ctx, spec.Name, client.NetworkCreateOptions{
		Driver: spec.Driver,
		IPAM: &network.IPAM{
			Config: []network.IPAMConfig{
				{Subnet: spec.Subnet, Gateway: spec.Gateway},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("network create: %w", err)
	}
	return &Network{ID: resp.ID, Name: spec.Name}, nil
}

func (c *Client) NetworkInspect(ctx context.Context, id string) (*Network, error) {
	resp, err := c.inner.NetworkInspect(ctx, id, client.NetworkInspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("network inspect: %w", err)
	}
	return &Network{ID: resp.Network.ID, Name: resp.Network.Name}, nil
}

func (c *Client) NetworkExists(ctx context.Context, name string) (bool, error) {
	nets, err := c.inner.NetworkList(ctx, client.NetworkListOptions{
		Filters: client.Filters{"name": {name: true}},
	})
	if err != nil {
		return false, fmt.Errorf("network list: %w", err)
	}
	for _, net := range nets.Items {
		if net.Name == name {
			return true, nil
		}
	}
	return false, nil
}

func (c *Client) NetworkRemove(ctx context.Context, id string) error {
	_, err := c.inner.NetworkRemove(ctx, id, client.NetworkRemoveOptions{})
	if err != nil {
		return fmt.Errorf("network remove: %w", err)
	}
	return nil
}

func (c *Client) ServiceCreate(ctx context.Context, spec ServiceCreateSpec) (*Service, error) {
	cspec := &swarm.ContainerSpec{
		Image: spec.Image,
		Env:   spec.Env,
	}
	for _, m := range spec.Mounts {
		cspec.Mounts = append(cspec.Mounts, mount.Mount{
			Type:   mount.TypeVolume,
			Source: m.Source,
			Target: m.Target,
		})
	}
	task := swarm.TaskSpec{ContainerSpec: cspec}
	if spec.Network != "" {
		task.Networks = []swarm.NetworkAttachmentConfig{{Target: spec.Network}}
	}

	resp, err := c.inner.ServiceCreate(ctx, client.ServiceCreateOptions{
		Spec: swarm.ServiceSpec{
			Annotations:  swarm.Annotations{Name: spec.Name},
			TaskTemplate: task,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("service create: %w", err)
	}
	return &Service{ID: resp.ID, Name: spec.Name}, nil
}

func (c *Client) ServiceExists(ctx context.Context, name string) (bool, error) {
	svcs, err := c.inner.ServiceList(ctx, client.ServiceListOptions{
		Filters: client.Filters{"name": {name: true}},
	})
	if err != nil {
		return false, fmt.Errorf("service list: %w", err)
	}
	for _, svc := range svcs.Items {
		if svc.Spec.Name == name {
			return true, nil
		}
	}
	return false, nil
}

func (c *Client) ServiceRemove(ctx context.Context, id string) error {
	_, err := c.inner.ServiceRemove(ctx, id, client.ServiceRemoveOptions{})
	if err != nil {
		return fmt.Errorf("service remove: %w", err)
	}
	return nil
}
