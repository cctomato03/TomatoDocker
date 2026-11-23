package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"

	"web-basic/internal/dockerstore"
)

// Images 镜像列表。
func (m *Manager) Images(ctx context.Context, env *dockerstore.Environment, all bool, keyword string) ([]Image, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return nil, err
	}
	list, err := c.ImageList(ctx, image.ListOptions{All: all})
	if err != nil {
		return nil, err
	}
	kw := strings.ToLower(strings.TrimSpace(keyword))
	out := make([]Image, 0, len(list))
	for _, item := range list {
		if kw != "" {
			hit := strings.Contains(strings.ToLower(strings.Join(item.RepoTags, " ")), kw) || strings.Contains(item.ID, kw)
			if !hit {
				continue
			}
		}
		short := strings.TrimPrefix(item.ID, "sha256:")
		if len(short) > 12 {
			short = short[:12]
		}
		out = append(out, Image{
			ID:          item.ID,
			ShortID:     short,
			RepoTags:    item.RepoTags,
			RepoDigests: item.RepoDigests,
			Created:     item.Created,
			Size:        item.Size,
			SharedSize:  item.SharedSize,
			Containers:  item.Containers,
			Labels:      item.Labels,
		})
	}
	return out, nil
}

// ImageInspect 镜像详情。
func (m *Manager) ImageInspect(ctx context.Context, env *dockerstore.Environment, id string) (image.InspectResponse, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return image.InspectResponse{}, err
	}
	return c.ImageInspect(ctx, id)
}

// ImageHistory 镜像构建历史。
func (m *Manager) ImageHistory(ctx context.Context, env *dockerstore.Environment, id string) ([]image.HistoryResponseItem, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return nil, err
	}
	return c.ImageHistory(ctx, id)
}

// PullImage 拉取镜像，返回 Docker 进度流（JSON lines）。
func (m *Manager) PullImage(ctx context.Context, env *dockerstore.Environment, ref string) (io.ReadCloser, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("镜像名称不能为空")
	}
	c, err := m.Client(ctx, env)
	if err != nil {
		return nil, err
	}
	return c.ImagePull(ctx, ref, image.PullOptions{})
}

// PullImageBlocking 拉取镜像并等待完成（Stack 部署时预拉取用）。
func (m *Manager) PullImageBlocking(ctx context.Context, env *dockerstore.Environment, ref string, progress func(string)) error {
	rc, err := m.PullImage(ctx, env, ref)
	if err != nil {
		return err
	}
	defer rc.Close()
	dec := json.NewDecoder(rc)
	for {
		var msg struct {
			Status      string `json:"status"`
			Progress    string `json:"progress"`
			Error       string `json:"error"`
			ErrorDetail struct {
				Message string `json:"message"`
			} `json:"errorDetail"`
		}
		if err := dec.Decode(&msg); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("读取镜像拉取进度失败: %w", err)
		}
		if msg.ErrorDetail.Message != "" {
			return fmt.Errorf("拉取镜像失败: %s", msg.ErrorDetail.Message)
		}
		if msg.Error != "" {
			return fmt.Errorf("拉取镜像失败: %s", msg.Error)
		}
		if progress != nil {
			line := msg.Status
			if msg.Progress != "" {
				line += " " + msg.Progress
			}
			progress(line)
		}
	}
}

// ImageRemove 删除镜像。
func (m *Manager) ImageRemove(ctx context.Context, env *dockerstore.Environment, id string, force bool) error {
	c, err := m.Client(ctx, env)
	if err != nil {
		return err
	}
	_, err = c.ImageRemove(ctx, id, image.RemoveOptions{Force: force, PruneChildren: true})
	return err
}

// ImagesPrune 清理悬空镜像。
func (m *Manager) ImagesPrune(ctx context.Context, env *dockerstore.Environment) (uint64, int64, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return 0, 0, err
	}
	report, err := c.ImagesPrune(ctx, filters.NewArgs(filters.Arg("dangling", "true")))
	if err != nil {
		return 0, 0, err
	}
	return uint64(len(report.ImagesDeleted)), int64(report.SpaceReclaimed), nil
}

// ImageTag 给镜像打标签。
func (m *Manager) ImageTag(ctx context.Context, env *dockerstore.Environment, id, repo, tag string) error {
	c, err := m.Client(ctx, env)
	if err != nil {
		return err
	}
	ref := repo
	if tag != "" {
		ref += ":" + tag
	}
	return c.ImageTag(ctx, id, ref)
}

// ImageExists 判断镜像是否已存在（部署前决定是否需要拉取）。
func (m *Manager) ImageExists(ctx context.Context, env *dockerstore.Environment, ref string) bool {
	c, err := m.Client(ctx, env)
	if err != nil {
		return false
	}
	_, err = c.ImageInspect(ctx, ref)
	return err == nil
}
