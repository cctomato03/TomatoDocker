package service

import (
	"context"
	"strings"
	"time"

	"web-basic/internal/docker"
	"web-basic/internal/principal"
	"web-basic/pkg/errs"
)

// DockerSwarmState 集群状态。
func (s *Service) DockerSwarmState(ctx context.Context, p *principal.Info, envID string) (*docker.SwarmState, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessRead)
	if err != nil {
		return nil, err
	}
	out, err := s.Docker.SwarmState(ctx, env)
	return out, dockerErr(err)
}

// DockerSwarmInit 初始化集群。
func (s *Service) DockerSwarmInit(ctx context.Context, p *principal.Info, envID, advertiseAddr, listenAddr string) (string, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessOperate)
	if err != nil {
		return "", err
	}
	nodeID, err := s.Docker.SwarmInit(ctx, env, advertiseAddr, listenAddr)
	return nodeID, dockerErr(err)
}

// DockerSwarmLeave 退出集群。
func (s *Service) DockerSwarmLeave(ctx context.Context, p *principal.Info, envID string, force bool) error {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessOperate)
	if err != nil {
		return err
	}
	return dockerErr(s.Docker.SwarmLeave(ctx, env, force))
}

// DockerNodes 节点列表。
func (s *Service) DockerNodes(ctx context.Context, p *principal.Info, envID string) ([]docker.Node, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessRead)
	if err != nil {
		return nil, err
	}
	out, err := s.Docker.Nodes(ctx, env)
	return out, dockerErr(err)
}

// DockerUpdateNode 修改节点。
func (s *Service) DockerUpdateNode(ctx context.Context, p *principal.Info, envID, nodeID, role, availability string, labels map[string]string) error {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessOperate)
	if err != nil {
		return err
	}
	return dockerErr(s.Docker.UpdateNode(ctx, env, nodeID, role, availability, labels))
}

// DockerRemoveNode 移除节点。
func (s *Service) DockerRemoveNode(ctx context.Context, p *principal.Info, envID, nodeID string, force bool) error {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessOperate)
	if err != nil {
		return err
	}
	return dockerErr(s.Docker.RemoveNode(ctx, env, nodeID, force))
}

// DockerServices 服务列表。
func (s *Service) DockerServices(ctx context.Context, p *principal.Info, envID, keyword string) ([]docker.Service, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessRead)
	if err != nil {
		return nil, err
	}
	out, err := s.Docker.Services(ctx, env, keyword)
	return out, dockerErr(err)
}

// DockerServiceDetail 服务详情。
func (s *Service) DockerServiceDetail(ctx context.Context, p *principal.Info, envID, sid string) (any, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessRead)
	if err != nil {
		return nil, err
	}
	out, err := s.Docker.ServiceDetail(ctx, env, sid)
	return out, dockerErr(err)
}

// DockerCreateService 创建服务。
func (s *Service) DockerCreateService(ctx context.Context, p *principal.Info, envID string, req *docker.CreateServiceRequest) (string, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessOperate)
	if err != nil {
		return "", err
	}
	if err := s.ensureSwarm(ctx, env); err != nil {
		return "", err
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Image) == "" {
		return "", errs.Invalid("服务名称与镜像不能为空")
	}
	sid, err := s.Docker.CreateService(ctx, env, req)
	return sid, dockerErr(err)
}

// DockerUpdateService 修改服务。
func (s *Service) DockerUpdateService(ctx context.Context, p *principal.Info, envID, sid string, req *docker.UpdateServiceRequest) error {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessOperate)
	if err != nil {
		return err
	}
	if err := s.ensureSwarm(ctx, env); err != nil {
		return err
	}
	return dockerErr(s.Docker.UpdateService(ctx, env, sid, req))
}

// DockerScaleService 调整副本数。
func (s *Service) DockerScaleService(ctx context.Context, p *principal.Info, envID, sid string, replicas uint64) error {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessOperate)
	if err != nil {
		return err
	}
	return dockerErr(s.Docker.ScaleService(ctx, env, sid, replicas))
}

// DockerRemoveService 删除服务。
func (s *Service) DockerRemoveService(ctx context.Context, p *principal.Info, envID, sid string) error {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessOperate)
	if err != nil {
		return err
	}
	return dockerErr(s.Docker.RemoveService(ctx, env, sid))
}

// DockerServiceLogs 服务日志。
func (s *Service) DockerServiceLogs(ctx context.Context, p *principal.Info, envID, sid string, tail int) (string, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessRead)
	if err != nil {
		return "", err
	}
	logCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := s.Docker.ServiceLogs(logCtx, env, sid, tail)
	return out, dockerErr(err)
}

// DockerTasks 任务列表。
func (s *Service) DockerTasks(ctx context.Context, p *principal.Info, envID string) ([]docker.Task, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessRead)
	if err != nil {
		return nil, err
	}
	out, err := s.Docker.Tasks(ctx, env)
	return out, dockerErr(err)
}

// DockerSecrets 密钥列表。
func (s *Service) DockerSecrets(ctx context.Context, p *principal.Info, envID string) ([]docker.Secret, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessRead)
	if err != nil {
		return nil, err
	}
	out, err := s.Docker.Secrets(ctx, env)
	return out, dockerErr(err)
}

// DockerCreateSecret 创建密钥。
func (s *Service) DockerCreateSecret(ctx context.Context, p *principal.Info, envID string, req *docker.CreateSecretRequest) (string, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessOperate)
	if err != nil {
		return "", err
	}
	if err := s.ensureSwarm(ctx, env); err != nil {
		return "", err
	}
	sid, err := s.Docker.CreateSecret(ctx, env, req)
	return sid, dockerErr(err)
}

// DockerRemoveSecret 删除密钥。
func (s *Service) DockerRemoveSecret(ctx context.Context, p *principal.Info, envID, sid string) error {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessOperate)
	if err != nil {
		return err
	}
	return dockerErr(s.Docker.RemoveSecret(ctx, env, sid))
}

// DockerConfigs 配置列表。
func (s *Service) DockerConfigs(ctx context.Context, p *principal.Info, envID string) ([]docker.ConfigObj, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessRead)
	if err != nil {
		return nil, err
	}
	out, err := s.Docker.Configs(ctx, env)
	return out, dockerErr(err)
}

// DockerCreateConfig 创建配置。
func (s *Service) DockerCreateConfig(ctx context.Context, p *principal.Info, envID string, req *docker.CreateSecretRequest) (string, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessOperate)
	if err != nil {
		return "", err
	}
	if err := s.ensureSwarm(ctx, env); err != nil {
		return "", err
	}
	cid, err := s.Docker.CreateConfig(ctx, env, req)
	return cid, dockerErr(err)
}

// DockerRemoveConfig 删除配置。
func (s *Service) DockerRemoveConfig(ctx context.Context, p *principal.Info, envID, cid string) error {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessOperate)
	if err != nil {
		return err
	}
	return dockerErr(s.Docker.RemoveConfig(ctx, env, cid))
}
