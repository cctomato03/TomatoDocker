package docker

import (
	"context"
	"strings"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/volume"

	"web-basic/internal/dockerstore"
)

// Volumes 数据卷列表。
func (m *Manager) Volumes(ctx context.Context, env *dockerstore.Environment, keyword string) ([]Volume, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return nil, err
	}
	list, err := c.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		return nil, err
	}
	kw := strings.ToLower(strings.TrimSpace(keyword))
	out := make([]Volume, 0, len(list.Volumes))
	for _, item := range list.Volumes {
		if item == nil {
			continue
		}
		if kw != "" && !strings.Contains(strings.ToLower(item.Name), kw) {
			continue
		}
		inUse := item.UsageData != nil && item.UsageData.RefCount > 0
		out = append(out, Volume{
			Name:       item.Name,
			Driver:     item.Driver,
			Mountpoint: item.Mountpoint,
			Scope:      item.Scope,
			CreatedAt:  item.CreatedAt,
			Labels:     item.Labels,
			StackName:  item.Labels["com.docker.stack.namespace"],
			InUse:      inUse,
		})
	}
	return out, nil
}

// VolumeInspect 数据卷详情。
func (m *Manager) VolumeInspect(ctx context.Context, env *dockerstore.Environment, name string) (volume.Volume, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return volume.Volume{}, err
	}
	return c.VolumeInspect(ctx, name)
}

// CreateVolume 创建数据卷。
func (m *Manager) CreateVolume(ctx context.Context, env *dockerstore.Environment, req *CreateVolumeRequest) (volume.Volume, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return volume.Volume{}, err
	}
	return c.VolumeCreate(ctx, volume.CreateOptions{
		Name: req.Name, Driver: req.Driver, DriverOpts: req.DriverOpts, Labels: req.Labels,
	})
}

// RemoveVolume 删除数据卷。
func (m *Manager) RemoveVolume(ctx context.Context, env *dockerstore.Environment, name string, force bool) error {
	c, err := m.Client(ctx, env)
	if err != nil {
		return err
	}
	return c.VolumeRemove(ctx, name, force)
}

// VolumesPrune 清理未使用数据卷。
func (m *Manager) VolumesPrune(ctx context.Context, env *dockerstore.Environment) (uint64, int64, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return 0, 0, err
	}
	report, err := c.VolumesPrune(ctx, filters.NewArgs())
	if err != nil {
		return 0, 0, err
	}
	return uint64(len(report.VolumesDeleted)), int64(report.SpaceReclaimed), nil
}

// VolumeExists 判断数据卷是否存在。
func (m *Manager) VolumeExists(ctx context.Context, env *dockerstore.Environment, name string) bool {
	c, err := m.Client(ctx, env)
	if err != nil {
		return false
	}
	_, err = c.VolumeInspect(ctx, name)
	return err == nil
}
