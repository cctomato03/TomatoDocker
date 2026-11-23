package service

import (
	"context"
	"io"
	"strings"
	"time"

	"web-basic/internal/docker"
	"web-basic/internal/dockerstore"
	"web-basic/internal/principal"
	"web-basic/pkg/errs"
)

// ---------------- 容器 ----------------

// DockerContainers 容器列表。
func (s *Service) DockerContainers(ctx context.Context, p *principal.Info, envID string, all bool, keyword, state string) ([]docker.Container, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessRead)
	if err != nil {
		return nil, err
	}
	out, err := s.Docker.Containers(ctx, env, all, keyword, state)
	return out, dockerErr(err)
}

// DockerContainerInspect 容器详情。
func (s *Service) DockerContainerInspect(ctx context.Context, p *principal.Info, envID, cid string) (any, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessRead)
	if err != nil {
		return nil, err
	}
	detail, err := s.Docker.ContainerInspect(ctx, env, cid)
	return detail, dockerErr(err)
}

// DockerContainerAction 容器操作。
func (s *Service) DockerContainerAction(ctx context.Context, p *principal.Info, envID, cid, action, name string) error {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessOperate)
	if err != nil {
		return err
	}
	return dockerErr(s.Docker.ContainerAction(ctx, env, cid, action, name))
}

// DockerContainerRemove 删除容器。
func (s *Service) DockerContainerRemove(ctx context.Context, p *principal.Info, envID, cid string, force, volumes bool) error {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessOperate)
	if err != nil {
		return err
	}
	return dockerErr(s.Docker.ContainerRemove(ctx, env, cid, force, volumes))
}

// DockerCreateContainer 创建容器。
func (s *Service) DockerCreateContainer(ctx context.Context, p *principal.Info, envID string, req *docker.CreateContainerRequest) (string, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessOperate)
	if err != nil {
		return "", err
	}
	req.Image = strings.TrimSpace(req.Image)
	if req.Image == "" {
		return "", errs.Invalid("镜像不能为空")
	}
	if strings.TrimSpace(req.Name) == "" {
		req.Name = "tomato-" + time.Now().Format("20060102-150405")
	}
	for _, e := range req.Env {
		if !strings.Contains(e, "=") {
			return "", errs.Invalid("环境变量必须使用 KEY=VALUE 格式")
		}
	}
	cid, err := s.Docker.CreateContainer(ctx, env, req)
	return cid, dockerErr(err)
}

// DockerContainerLogs 容器日志。
func (s *Service) DockerContainerLogs(ctx context.Context, p *principal.Info, envID, cid string, tail int, timestamps bool) (string, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessRead)
	if err != nil {
		return "", err
	}
	logCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := s.Docker.ContainerLogs(logCtx, env, cid, tail, timestamps)
	return out, dockerErr(err)
}

// DockerContainerStats 容器资源快照。
func (s *Service) DockerContainerStats(ctx context.Context, p *principal.Info, envID, cid string) (*docker.ContainerStats, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessRead)
	if err != nil {
		return nil, err
	}
	statCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := s.Docker.ContainerStats(statCtx, env, cid)
	return out, dockerErr(err)
}

// DockerContainerExec 在容器内执行命令。
func (s *Service) DockerContainerExec(ctx context.Context, p *principal.Info, envID, cid, command string) (string, int, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessOperate)
	if err != nil {
		return "", -1, err
	}
	if strings.TrimSpace(command) == "" {
		return "", -1, errs.Invalid("命令不能为空")
	}
	out, code, err := s.Docker.ContainerExec(ctx, env, cid, command, 60*time.Second)
	return out, code, dockerErr(err)
}

// DockerContainersPrune 清理已停止容器。
func (s *Service) DockerContainersPrune(ctx context.Context, p *principal.Info, envID string) (uint64, int64, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessOperate)
	if err != nil {
		return 0, 0, err
	}
	n, space, err := s.Docker.ContainersPrune(ctx, env)
	return n, space, dockerErr(err)
}

// ---------------- 镜像 ----------------

// DockerImages 镜像列表。
func (s *Service) DockerImages(ctx context.Context, p *principal.Info, envID string, all bool, keyword string) ([]docker.Image, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessRead)
	if err != nil {
		return nil, err
	}
	out, err := s.Docker.Images(ctx, env, all, keyword)
	return out, dockerErr(err)
}

// DockerImageInspect 镜像详情。
func (s *Service) DockerImageInspect(ctx context.Context, p *principal.Info, envID, id string) (any, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessRead)
	if err != nil {
		return nil, err
	}
	out, err := s.Docker.ImageInspect(ctx, env, id)
	return out, dockerErr(err)
}

// DockerImageHistory 镜像历史。
func (s *Service) DockerImageHistory(ctx context.Context, p *principal.Info, envID, id string) (any, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessRead)
	if err != nil {
		return nil, err
	}
	out, err := s.Docker.ImageHistory(ctx, env, id)
	return out, dockerErr(err)
}

// DockerPullImage 拉取镜像（返回进度流，由 handler 流式输出）。
func (s *Service) DockerPullImage(ctx context.Context, p *principal.Info, envID, ref string) (io.ReadCloser, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessOperate)
	if err != nil {
		return nil, err
	}
	rc, err := s.Docker.PullImage(ctx, env, ref)
	if err != nil {
		return nil, dockerErr(err)
	}
	return rc, nil
}

// DockerImageRemove 删除镜像。
func (s *Service) DockerImageRemove(ctx context.Context, p *principal.Info, envID, id string, force bool) error {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessOperate)
	if err != nil {
		return err
	}
	return dockerErr(s.Docker.ImageRemove(ctx, env, id, force))
}

// DockerImagesPrune 清理悬空镜像。
func (s *Service) DockerImagesPrune(ctx context.Context, p *principal.Info, envID string) (uint64, int64, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessOperate)
	if err != nil {
		return 0, 0, err
	}
	n, space, err := s.Docker.ImagesPrune(ctx, env)
	return n, space, dockerErr(err)
}

// ---------------- 网络 ----------------

// DockerNetworks 网络列表。
func (s *Service) DockerNetworks(ctx context.Context, p *principal.Info, envID, keyword string) ([]docker.Network, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessRead)
	if err != nil {
		return nil, err
	}
	out, err := s.Docker.Networks(ctx, env, keyword)
	return out, dockerErr(err)
}

// DockerNetworkInspect 网络详情。
func (s *Service) DockerNetworkInspect(ctx context.Context, p *principal.Info, envID, id string) (any, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessRead)
	if err != nil {
		return nil, err
	}
	out, err := s.Docker.NetworkInspect(ctx, env, id)
	return out, dockerErr(err)
}

// DockerCreateNetwork 创建网络。
func (s *Service) DockerCreateNetwork(ctx context.Context, p *principal.Info, envID string, req *docker.CreateNetworkRequest) (string, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessOperate)
	if err != nil {
		return "", err
	}
	id, err := s.Docker.CreateNetwork(ctx, env, req)
	return id, dockerErr(err)
}

// DockerRemoveNetwork 删除网络。
func (s *Service) DockerRemoveNetwork(ctx context.Context, p *principal.Info, envID, id string) error {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessOperate)
	if err != nil {
		return err
	}
	return dockerErr(s.Docker.RemoveNetwork(ctx, env, id))
}

// DockerConnectNetwork 容器接入网络。
func (s *Service) DockerConnectNetwork(ctx context.Context, p *principal.Info, envID, networkID, containerID string, aliases []string) error {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessOperate)
	if err != nil {
		return err
	}
	return dockerErr(s.Docker.ConnectNetwork(ctx, env, networkID, containerID, aliases))
}

// DockerDisconnectNetwork 容器断开网络。
func (s *Service) DockerDisconnectNetwork(ctx context.Context, p *principal.Info, envID, networkID, containerID string, force bool) error {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessOperate)
	if err != nil {
		return err
	}
	return dockerErr(s.Docker.DisconnectNetwork(ctx, env, networkID, containerID, force))
}

// DockerNetworksPrune 清理未使用网络。
func (s *Service) DockerNetworksPrune(ctx context.Context, p *principal.Info, envID string) (uint64, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessOperate)
	if err != nil {
		return 0, err
	}
	n, err := s.Docker.NetworksPrune(ctx, env)
	return n, dockerErr(err)
}

// ---------------- 数据卷 ----------------

// DockerVolumes 数据卷列表。
func (s *Service) DockerVolumes(ctx context.Context, p *principal.Info, envID, keyword string) ([]docker.Volume, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessRead)
	if err != nil {
		return nil, err
	}
	out, err := s.Docker.Volumes(ctx, env, keyword)
	return out, dockerErr(err)
}

// DockerVolumeInspect 数据卷详情。
func (s *Service) DockerVolumeInspect(ctx context.Context, p *principal.Info, envID, name string) (any, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessRead)
	if err != nil {
		return nil, err
	}
	out, err := s.Docker.VolumeInspect(ctx, env, name)
	return out, dockerErr(err)
}

// DockerCreateVolume 创建数据卷。
func (s *Service) DockerCreateVolume(ctx context.Context, p *principal.Info, envID string, req *docker.CreateVolumeRequest) (any, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessOperate)
	if err != nil {
		return nil, err
	}
	out, err := s.Docker.CreateVolume(ctx, env, req)
	return out, dockerErr(err)
}

// DockerRemoveVolume 删除数据卷。
func (s *Service) DockerRemoveVolume(ctx context.Context, p *principal.Info, envID, name string, force bool) error {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessOperate)
	if err != nil {
		return err
	}
	return dockerErr(s.Docker.RemoveVolume(ctx, env, name, force))
}

// DockerVolumesPrune 清理未使用数据卷。
func (s *Service) DockerVolumesPrune(ctx context.Context, p *principal.Info, envID string) (uint64, int64, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessOperate)
	if err != nil {
		return 0, 0, err
	}
	n, space, err := s.Docker.VolumesPrune(ctx, env)
	return n, space, dockerErr(err)
}

// 确保 dockerstore 被引用（环境类型在方法签名中隐式使用）。
var _ = (*dockerstore.Environment)(nil)
