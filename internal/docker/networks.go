package docker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"

	"web-basic/internal/dockerstore"
)

// Networks 网络列表。
func (m *Manager) Networks(ctx context.Context, env *dockerstore.Environment, keyword string) ([]Network, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return nil, err
	}
	list, err := c.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		return nil, err
	}
	kw := strings.ToLower(strings.TrimSpace(keyword))
	out := make([]Network, 0, len(list))
	for _, item := range list {
		if kw != "" && !strings.Contains(strings.ToLower(item.Name), kw) && !strings.Contains(item.ID, kw) {
			continue
		}
		subnets := make([]string, 0, len(item.IPAM.Config))
		for _, cfg := range item.IPAM.Config {
			if cfg.Subnet != "" {
				subnets = append(subnets, cfg.Subnet)
			}
		}
		created := ""
		if !item.Created.IsZero() {
			created = item.Created.Format(time.RFC3339)
		}
		out = append(out, Network{
			ID:         item.ID,
			Name:       item.Name,
			Driver:     item.Driver,
			Scope:      item.Scope,
			Internal:   item.Internal,
			Attachable: item.Attachable,
			Ingress:    item.Ingress,
			EnableIPv6: item.EnableIPv6,
			Subnets:    subnets,
			Containers: len(item.Containers),
			Labels:     item.Labels,
			Created:    created,
			StackName:  item.Labels["com.docker.stack.namespace"],
		})
	}
	return out, nil
}

// NetworkInspect 网络详情。
func (m *Manager) NetworkInspect(ctx context.Context, env *dockerstore.Environment, id string) (network.Inspect, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return network.Inspect{}, err
	}
	return c.NetworkInspect(ctx, id, network.InspectOptions{Verbose: true})
}

// CreateNetwork 创建网络。
func (m *Manager) CreateNetwork(ctx context.Context, env *dockerstore.Environment, req *CreateNetworkRequest) (string, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return "", err
	}
	driver := strings.TrimSpace(req.Driver)
	if driver == "" {
		driver = "bridge"
	}
	opts := network.CreateOptions{
		Driver:     driver,
		Internal:   req.Internal,
		Attachable: req.Attachable,
		Labels:     req.Labels,
	}
	if req.EnableIPv6 {
		v := true
		opts.EnableIPv6 = &v
	}
	if req.Subnet != "" || req.Gateway != "" {
		opts.IPAM = &network.IPAM{
			Driver: "default",
			Config: []network.IPAMConfig{{
				Subnet:  strings.TrimSpace(req.Subnet),
				Gateway: strings.TrimSpace(req.Gateway),
			}},
		}
	}
	resp, err := c.NetworkCreate(ctx, req.Name, opts)
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

// RemoveNetwork 删除网络。
func (m *Manager) RemoveNetwork(ctx context.Context, env *dockerstore.Environment, id string) error {
	c, err := m.Client(ctx, env)
	if err != nil {
		return err
	}
	return c.NetworkRemove(ctx, id)
}

// ConnectNetwork 把容器接入网络。
func (m *Manager) ConnectNetwork(ctx context.Context, env *dockerstore.Environment, networkID, containerID string, aliases []string) error {
	c, err := m.Client(ctx, env)
	if err != nil {
		return err
	}
	return c.NetworkConnect(ctx, networkID, containerID, &network.EndpointSettings{Aliases: aliases})
}

// DisconnectNetwork 把容器从网络断开。
func (m *Manager) DisconnectNetwork(ctx context.Context, env *dockerstore.Environment, networkID, containerID string, force bool) error {
	c, err := m.Client(ctx, env)
	if err != nil {
		return err
	}
	return c.NetworkDisconnect(ctx, networkID, containerID, force)
}

// NetworksPrune 清理未使用网络。
func (m *Manager) NetworksPrune(ctx context.Context, env *dockerstore.Environment) (uint64, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return 0, err
	}
	report, err := c.NetworksPrune(ctx, filters.NewArgs())
	if err != nil {
		return 0, err
	}
	if len(report.NetworksDeleted) > 0 && report.NetworksDeleted[0] == "" {
		return 0, nil
	}
	return uint64(len(report.NetworksDeleted)), nil
}

var _ = fmt.Sprintf
