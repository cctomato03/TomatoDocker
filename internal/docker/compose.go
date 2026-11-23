package docker

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"

	"web-basic/internal/dockerstore"
)

// DeployOptions Stack 部署选项。
type DeployOptions struct {
	PullImages bool `json:"pullImages"`
}

// LoadCompose 解析并校验 compose 文件（供保存前校验、部署共用）。
func LoadCompose(ctx context.Context, name, composeContent, envContent string) (*types.Project, error) {
	if strings.TrimSpace(composeContent) == "" {
		return nil, fmt.Errorf("compose 文件内容不能为空")
	}
	env := parseDotEnv(envContent)
	project, err := loader.LoadWithContext(ctx, types.ConfigDetails{
		WorkingDir:  "/",
		ConfigFiles: []types.ConfigFile{{Filename: "docker-compose.yml", Content: []byte(composeContent)}},
		Environment: types.Mapping(env),
	}, func(o *loader.Options) {
		o.SetProjectName(normalizeProjectName(name), true)
		// 相对路径不解析：Web 端没有 Stack 文件目录，绑定挂载必须写绝对路径。
		o.ResolvePaths = false
		o.SkipConsistencyCheck = true
	})
	if err != nil {
		return nil, fmt.Errorf("compose 文件解析失败: %w", err)
	}
	if len(project.Services) == 0 {
		return nil, fmt.Errorf("compose 文件中没有定义任何 service")
	}
	for key, svc := range project.Services {
		for _, p := range svc.Ports {
			if strings.Contains(p.Published, "-") {
				return nil, fmt.Errorf("service「%s」使用了端口范围「%s」：Web 编辑器暂不支持端口范围，请拆成多条端口映射", key, p.Published)
			}
		}
		if strings.TrimSpace(svc.Image) == "" && svc.Build == nil {
			return nil, fmt.Errorf("service「%s」缺少 image（Web 编辑器不支持 build，请先在外部构建镜像）", key)
		}
		if svc.Build != nil {
			return nil, fmt.Errorf("service「%s」使用了 build：Web 编辑器没有构建上下文，请改为 image 或先在外部构建", key)
		}
		if len(svc.EnvFiles) > 0 {
			return nil, fmt.Errorf("service「%s」使用了 env_file：请把变量直接写进 environment，或使用 Stack 的 .env（变量插值）", key)
		}
		for _, v := range svc.Volumes {
			if v.Type == "bind" && v.Source != "" && !isAbsolutePath(v.Source) {
				return nil, fmt.Errorf("service「%s」的绑定挂载「%s」不是绝对路径：请改成 /host/path 形式", key, v.Source)
			}
		}
	}
	return project, nil
}

// DeployCompose 在单机 Docker 上部署 compose（相当于 docker compose up -d）。
func (m *Manager) DeployCompose(ctx context.Context, env *dockerstore.Environment, stack *dockerstore.Stack, opts DeployOptions) error {
	project, err := LoadCompose(ctx, stack.Name, stack.ComposeContent, stack.EnvContent)
	if err != nil {
		return err
	}
	// 单机 Compose 的 secrets/configs 需要把文件挂进容器，Web 端没有 Stack 文件目录；
	// 明确拒绝，避免"看起来部署成功但密钥没挂载"的静默错误。
	if len(project.Secrets) > 0 || len(project.Configs) > 0 {
		return fmt.Errorf("单机 Compose 暂不支持 secrets/configs：请改用 environment 变量，或把 Stack 类型切换为 Swarm（external 密钥/配置）")
	}
	for _, svc := range project.Services {
		if len(svc.Secrets) > 0 || len(svc.Configs) > 0 {
			return fmt.Errorf("service「%s」引用了 secrets/configs：单机 Compose 暂不支持，请改用 environment 变量", svc.Name)
		}
	}
	c, err := m.Client(ctx, env)
	if err != nil {
		return err
	}
	projectName := normalizeProjectName(stack.Name)

	// 1. 先确保镜像存在（缺失时拉取，避免 create 成功但 start 失败）。
	for _, svc := range project.Services {
		policy := strings.ToLower(strings.TrimSpace(svc.PullPolicy))
		if policy == "never" {
			continue
		}
		if opts.PullImages || policy == "always" || !m.ImageExists(ctx, env, svc.Image) {
			if err := m.PullImageBlocking(ctx, env, svc.Image, nil); err != nil {
				return fmt.Errorf("service「%s」拉取镜像失败: %w", svc.Name, err)
			}
		}
	}

	// 2. 网络与数据卷（先建后用）。
	networks, err := m.ensureComposeNetworks(ctx, c, project, projectName)
	if err != nil {
		return err
	}
	if err := m.ensureComposeVolumes(ctx, c, project, projectName); err != nil {
		return err
	}

	// 3. 移除同项目旧容器（保留数据卷与网络）。
	if err := m.removeComposeContainers(ctx, c, projectName, false); err != nil {
		return fmt.Errorf("清理旧容器失败: %w", err)
	}

	// 4. 按依赖顺序创建容器。
	order, err := serviceOrder(project)
	if err != nil {
		return err
	}
	configHash := composeHash(stack.ComposeContent)
	for _, name := range order {
		svc := project.Services[name]
		replicas := 1
		if svc.Scale != nil && *svc.Scale > 0 {
			replicas = *svc.Scale
		} else if svc.Deploy != nil && svc.Deploy.Replicas != nil && *svc.Deploy.Replicas > 0 {
			replicas = *svc.Deploy.Replicas
		}
		if svc.ContainerName != "" && replicas > 1 {
			return fmt.Errorf("service「%s」设置了 container_name，不能同时使用多副本", svc.Name)
		}
		for i := 1; i <= replicas; i++ {
			containerName := svc.ContainerName
			if containerName == "" {
				containerName = fmt.Sprintf("%s-%s-%d", projectName, svc.Name, i)
			}
			if err := m.createComposeContainer(ctx, c, project, projectName, &svc, networks, i, containerName, configHash); err != nil {
				return fmt.Errorf("service「%s」创建容器失败: %w", svc.Name, err)
			}
		}
	}
	return nil
}

// StopCompose 停止 Stack 的所有容器（保留容器与数据卷）。
func (m *Manager) StopCompose(ctx context.Context, env *dockerstore.Environment, stack *dockerstore.Stack) error {
	c, err := m.Client(ctx, env)
	if err != nil {
		return err
	}
	projectName := normalizeProjectName(stack.Name)
	containers, err := c.ContainerList(ctx, container.ListOptions{All: true, Filters: filters.NewArgs(filters.Arg("label", "com.docker.compose.project="+projectName))})
	if err != nil {
		return err
	}
	for _, item := range containers {
		if !strings.EqualFold(item.State, "running") && !strings.EqualFold(item.State, "paused") {
			continue
		}
		timeout := 10
		if err := c.ContainerStop(ctx, item.ID, container.StopOptions{Timeout: &timeout}); err != nil {
			return fmt.Errorf("停止容器 %s 失败: %w", firstName(item.Names), err)
		}
	}
	return nil
}

// RemoveCompose 删除 Stack 容器、项目网络（可选删除数据卷）。
func (m *Manager) RemoveCompose(ctx context.Context, env *dockerstore.Environment, stack *dockerstore.Stack, removeVolumes bool) error {
	c, err := m.Client(ctx, env)
	if err != nil {
		return err
	}
	projectName := normalizeProjectName(stack.Name)
	if err := m.removeComposeContainers(ctx, c, projectName, false); err != nil {
		return err
	}
	// 删除带项目标签的网络（外部网络没有该标签，不会被误删）。
	nets, err := c.NetworkList(ctx, network.ListOptions{Filters: filters.NewArgs(filters.Arg("label", "com.docker.compose.project="+projectName))})
	if err == nil {
		for _, n := range nets {
			_ = c.NetworkRemove(ctx, n.ID)
		}
	}
	if removeVolumes {
		vols, err := c.VolumeList(ctx, volume.ListOptions{Filters: filters.NewArgs(filters.Arg("label", "com.docker.compose.project="+projectName))})
		if err == nil {
			for _, v := range vols.Volumes {
				if v != nil {
					_ = c.VolumeRemove(ctx, v.Name, true)
				}
			}
		}
	}
	return nil
}

// DeploySwarm 在 Swarm 集群上部署 Stack（相当于 docker stack deploy）。
func (m *Manager) DeploySwarm(ctx context.Context, env *dockerstore.Environment, stack *dockerstore.Stack, opts DeployOptions) error {
	project, err := LoadCompose(ctx, stack.Name, stack.ComposeContent, stack.EnvContent)
	if err != nil {
		return err
	}
	c, err := m.Client(ctx, env)
	if err != nil {
		return err
	}
	info, err := c.Info(ctx)
	if err != nil {
		return err
	}
	if info.Swarm.LocalNodeState != swarm.LocalNodeStateActive {
		return fmt.Errorf("当前环境未启用 Swarm，请先在「Swarm 集群」页面初始化或加入集群")
	}
	if !info.Swarm.ControlAvailable {
		return fmt.Errorf("当前节点不是 Swarm 管理节点，无法部署 Stack")
	}
	projectName := normalizeProjectName(stack.Name)

	networks, err := m.ensureSwarmNetworks(ctx, c, project, projectName)
	if err != nil {
		return err
	}
	if err := m.ensureSwarmVolumes(ctx, c, project, projectName); err != nil {
		return err
	}
	secretRefs, configRefs, err := m.resolveSwarmFileObjects(ctx, c, project, projectName)
	if err != nil {
		return err
	}

	// 已有服务（按 Stack 命名空间标签识别）。
	existing, err := c.ServiceList(ctx, swarm.ServiceListOptions{
		Filters: filters.NewArgs(filters.Arg("label", "com.docker.stack.namespace="+projectName)),
	})
	if err != nil {
		return err
	}
	existingByName := make(map[string]swarm.Service, len(existing))
	for _, s := range existing {
		existingByName[s.Spec.Name] = s
	}

	desired := make(map[string]bool)
	for _, svc := range project.Services {
		spec, err := buildSwarmServiceSpec(project, projectName, &svc, networks, secretRefs, configRefs)
		if err != nil {
			return err
		}
		desired[spec.Annotations.Name] = true
		if old, ok := existingByName[spec.Annotations.Name]; ok {
			if _, err := c.ServiceUpdate(ctx, old.ID, old.Version, spec, swarm.ServiceUpdateOptions{}); err != nil {
				return fmt.Errorf("更新服务「%s」失败: %w", svc.Name, err)
			}
		} else {
			if _, err := c.ServiceCreate(ctx, spec, swarm.ServiceCreateOptions{}); err != nil {
				return fmt.Errorf("创建服务「%s」失败: %w", svc.Name, err)
			}
		}
	}
	for name, old := range existingByName {
		if desired[name] {
			continue
		}
		if err := c.ServiceRemove(ctx, old.ID); err != nil {
			return fmt.Errorf("移除旧服务「%s」失败: %w", name, err)
		}
	}
	return nil
}

// StopSwarm 停止 Stack（把 replicated 服务副本数置 0）。
func (m *Manager) StopSwarm(ctx context.Context, env *dockerstore.Environment, stack *dockerstore.Stack) error {
	c, err := m.Client(ctx, env)
	if err != nil {
		return err
	}
	projectName := normalizeProjectName(stack.Name)
	services, err := c.ServiceList(ctx, swarm.ServiceListOptions{
		Filters: filters.NewArgs(filters.Arg("label", "com.docker.stack.namespace="+projectName)),
	})
	if err != nil {
		return err
	}
	zero := uint64(0)
	for _, s := range services {
		if s.Spec.Mode.Global != nil {
			return fmt.Errorf("服务「%s」是 global 模式，不能缩容到 0；请直接删除 Stack 或修改 compose", s.Spec.Name)
		}
		spec := s.Spec
		spec.Mode = swarm.ServiceMode{Replicated: &swarm.ReplicatedService{Replicas: &zero}}
		if _, err := c.ServiceUpdate(ctx, s.ID, s.Version, spec, swarm.ServiceUpdateOptions{}); err != nil {
			return fmt.Errorf("停止服务「%s」失败: %w", s.Spec.Name, err)
		}
	}
	return nil
}

// RemoveSwarm 删除 Stack 服务与项目网络/数据卷。
func (m *Manager) RemoveSwarm(ctx context.Context, env *dockerstore.Environment, stack *dockerstore.Stack, removeVolumes bool) error {
	c, err := m.Client(ctx, env)
	if err != nil {
		return err
	}
	projectName := normalizeProjectName(stack.Name)
	services, err := c.ServiceList(ctx, swarm.ServiceListOptions{
		Filters: filters.NewArgs(filters.Arg("label", "com.docker.stack.namespace="+projectName)),
	})
	if err != nil {
		return err
	}
	for _, s := range services {
		if err := c.ServiceRemove(ctx, s.ID); err != nil {
			return fmt.Errorf("删除服务「%s」失败: %w", s.Spec.Name, err)
		}
	}
	nets, err := c.NetworkList(ctx, network.ListOptions{Filters: filters.NewArgs(filters.Arg("label", "com.docker.stack.namespace="+projectName))})
	if err == nil {
		for _, n := range nets {
			_ = c.NetworkRemove(ctx, n.ID)
		}
	}
	if removeVolumes {
		vols, err := c.VolumeList(ctx, volume.ListOptions{Filters: filters.NewArgs(filters.Arg("label", "com.docker.stack.namespace="+projectName))})
		if err == nil {
			for _, v := range vols.Volumes {
				if v != nil {
					_ = c.VolumeRemove(ctx, v.Name, true)
				}
			}
		}
	}
	return nil
}

// ensureComposeNetworks 创建 compose 顶层网络，返回 compose key → Docker 网络名。
func (m *Manager) ensureComposeNetworks(ctx context.Context, c *client.Client, project *types.Project, projectName string) (map[string]string, error) {
	out := make(map[string]string, len(project.Networks)+1)
	for key, cfg := range project.Networks {
		name := cfg.Name
		if name == "" {
			name = projectName + "_" + key
		}
		out[key] = name
		if cfg.External {
			if _, err := c.NetworkInspect(ctx, name, network.InspectOptions{}); err != nil {
				return nil, fmt.Errorf("外部网络「%s」不存在: %w", name, err)
			}
			continue
		}
		if _, err := c.NetworkInspect(ctx, name, network.InspectOptions{}); err == nil {
			continue
		}
		opts := network.CreateOptions{
			Driver:     defaultString(cfg.Driver, "bridge"),
			Internal:   cfg.Internal,
			Attachable: cfg.Attachable,
			Labels: mergeLabels(cfg.Labels, map[string]string{
				"com.docker.compose.project": projectName,
				"com.docker.compose.network": key,
			}),
		}
		if cfg.EnableIPv6 != nil {
			opts.EnableIPv6 = cfg.EnableIPv6
		}
		if len(cfg.Ipam.Config) > 0 {
			ipam := &network.IPAM{Driver: defaultString(cfg.Ipam.Driver, "default")}
			for _, pool := range cfg.Ipam.Config {
				if pool == nil {
					continue
				}
				ipam.Config = append(ipam.Config, network.IPAMConfig{
					Subnet: pool.Subnet, Gateway: pool.Gateway, IPRange: pool.IPRange,
				})
			}
			opts.IPAM = ipam
		}
		if _, err := c.NetworkCreate(ctx, name, opts); err != nil {
			return nil, fmt.Errorf("创建网络「%s」失败: %w", name, err)
		}
	}
	// 没有显式网络的 service 使用默认网络。
	if len(out) == 0 {
		out["default"] = projectName + "_default"
	}
	defaultName := out["default"]
	if defaultName == "" {
		defaultName = projectName + "_default"
		out["default"] = defaultName
	}
	if _, err := c.NetworkInspect(ctx, defaultName, network.InspectOptions{}); err != nil {
		if _, err := c.NetworkCreate(ctx, defaultName, network.CreateOptions{
			Driver: "bridge",
			Labels: map[string]string{
				"com.docker.compose.project": projectName,
				"com.docker.compose.network": "default",
			},
		}); err != nil {
			return nil, fmt.Errorf("创建默认网络「%s」失败: %w", defaultName, err)
		}
	}
	return out, nil
}

// ensureComposeVolumes 创建 compose 顶层数据卷。
func (m *Manager) ensureComposeVolumes(ctx context.Context, c *client.Client, project *types.Project, projectName string) error {
	for key, cfg := range project.Volumes {
		name := cfg.Name
		if name == "" {
			name = projectName + "_" + key
		}
		if cfg.External {
			if _, err := c.VolumeInspect(ctx, name); err != nil {
				return fmt.Errorf("外部数据卷「%s」不存在: %w", name, err)
			}
			continue
		}
		if _, err := c.VolumeInspect(ctx, name); err == nil {
			continue
		}
		_, err := c.VolumeCreate(ctx, volume.CreateOptions{
			Name: name, Driver: cfg.Driver, DriverOpts: cfg.DriverOpts,
			Labels: mergeLabels(cfg.Labels, map[string]string{
				"com.docker.compose.project": projectName,
				"com.docker.compose.volume":  key,
			}),
		})
		if err != nil {
			return fmt.Errorf("创建数据卷「%s」失败: %w", name, err)
		}
	}
	return nil
}

func (m *Manager) ensureSwarmNetworks(ctx context.Context, c *client.Client, project *types.Project, projectName string) (map[string]string, error) {
	out := make(map[string]string, len(project.Networks)+1)
	for key, cfg := range project.Networks {
		name := cfg.Name
		if name == "" {
			name = projectName + "_" + key
		}
		out[key] = name
		if cfg.External {
			if _, err := c.NetworkInspect(ctx, name, network.InspectOptions{}); err != nil {
				return nil, fmt.Errorf("外部网络「%s」不存在: %w", name, err)
			}
			continue
		}
		if _, err := c.NetworkInspect(ctx, name, network.InspectOptions{}); err == nil {
			continue
		}
		opts := network.CreateOptions{
			Driver:     defaultString(cfg.Driver, "overlay"),
			Internal:   cfg.Internal,
			Attachable: true,
			Labels: mergeLabels(cfg.Labels, map[string]string{
				"com.docker.stack.namespace": projectName,
				"com.docker.stack.network":   key,
			}),
		}
		if cfg.EnableIPv6 != nil {
			opts.EnableIPv6 = cfg.EnableIPv6
		}
		if len(cfg.Ipam.Config) > 0 {
			ipam := &network.IPAM{Driver: defaultString(cfg.Ipam.Driver, "default")}
			for _, pool := range cfg.Ipam.Config {
				if pool == nil {
					continue
				}
				ipam.Config = append(ipam.Config, network.IPAMConfig{Subnet: pool.Subnet, Gateway: pool.Gateway, IPRange: pool.IPRange})
			}
			opts.IPAM = ipam
		}
		if _, err := c.NetworkCreate(ctx, name, opts); err != nil {
			return nil, fmt.Errorf("创建 Swarm 网络「%s」失败: %w", name, err)
		}
	}
	if out["default"] == "" {
		name := projectName + "_default"
		out["default"] = name
		if _, err := c.NetworkInspect(ctx, name, network.InspectOptions{}); err != nil {
			if _, err := c.NetworkCreate(ctx, name, network.CreateOptions{
				Driver: "overlay", Attachable: true,
				Labels: map[string]string{"com.docker.stack.namespace": projectName, "com.docker.stack.network": "default"},
			}); err != nil {
				return nil, fmt.Errorf("创建 Swarm 默认网络「%s」失败: %w", name, err)
			}
		}
	}
	return out, nil
}

func (m *Manager) ensureSwarmVolumes(ctx context.Context, c *client.Client, project *types.Project, projectName string) error {
	for key, cfg := range project.Volumes {
		name := cfg.Name
		if name == "" {
			name = projectName + "_" + key
		}
		if cfg.External {
			if _, err := c.VolumeInspect(ctx, name); err != nil {
				return fmt.Errorf("外部数据卷「%s」不存在: %w", name, err)
			}
			continue
		}
		if _, err := c.VolumeInspect(ctx, name); err == nil {
			continue
		}
		_, err := c.VolumeCreate(ctx, volume.CreateOptions{
			Name: name, Driver: cfg.Driver, DriverOpts: cfg.DriverOpts,
			Labels: mergeLabels(cfg.Labels, map[string]string{
				"com.docker.stack.namespace": projectName,
				"com.docker.stack.volume":    key,
			}),
		})
		if err != nil {
			return fmt.Errorf("创建数据卷「%s」失败: %w", name, err)
		}
	}
	return nil
}

// resolveSwarmFileObjects 解析 Stack 引用的 external secret/config。
// Web 端不保存密钥明文文件，因此只支持 external；文件型会在 LoadCompose 阶段被拒绝。
func (m *Manager) resolveSwarmFileObjects(ctx context.Context, c *client.Client, project *types.Project, projectName string) (map[string]swarm.SecretReference, map[string]swarm.ConfigReference, error) {
	secretRefs := map[string]swarm.SecretReference{}
	configRefs := map[string]swarm.ConfigReference{}
	if len(project.Secrets) == 0 && len(project.Configs) == 0 {
		return secretRefs, configRefs, nil
	}
	secrets, _ := c.SecretList(ctx, swarm.SecretListOptions{})
	secretByName := make(map[string]swarm.Secret, len(secrets))
	for _, s := range secrets {
		secretByName[s.Spec.Name] = s
	}
	configs, _ := c.ConfigList(ctx, swarm.ConfigListOptions{})
	configByName := make(map[string]swarm.Config, len(configs))
	for _, cfg := range configs {
		configByName[cfg.Spec.Name] = cfg
	}
	for key, cfg := range project.Secrets {
		if !cfg.External {
			return nil, nil, fmt.Errorf("密钥「%s」使用了 file/content：Web 端不保存密钥明文，请先在 Swarm 中创建同名密钥并声明 external: true", key)
		}
		name := cfg.Name
		if name == "" {
			name = key
		}
		s, ok := secretByName[name]
		if !ok {
			return nil, nil, fmt.Errorf("外部密钥「%s」不存在，请先在 Swarm 密钥页面创建", name)
		}
		secretRefs[key] = swarm.SecretReference{SecretID: s.ID, SecretName: s.Spec.Name}
	}
	for key, cfg := range project.Configs {
		if !cfg.External {
			return nil, nil, fmt.Errorf("配置「%s」使用了 file/content：Web 端不保存配置明文，请先在 Swarm 中创建同名配置并声明 external: true", key)
		}
		name := cfg.Name
		if name == "" {
			name = key
		}
		cfgObj, ok := configByName[name]
		if !ok {
			return nil, nil, fmt.Errorf("外部配置「%s」不存在，请先在 Swarm 配置页面创建", name)
		}
		configRefs[key] = swarm.ConfigReference{ConfigID: cfgObj.ID, ConfigName: cfgObj.Spec.Name}
	}
	return secretRefs, configRefs, nil
}

// createComposeContainer 依据 compose service 配置创建并启动单个容器。
func (m *Manager) createComposeContainer(
	ctx context.Context, c *client.Client, project *types.Project, projectName string,
	svc *types.ServiceConfig, networks map[string]string, number int, name, configHash string,
) error {
	cfg := &container.Config{
		Image:        svc.Image,
		Hostname:     svc.Hostname,
		Domainname:   svc.DomainName,
		User:         svc.User,
		Env:          composeEnv(svc.Environment),
		WorkingDir:   svc.WorkingDir,
		Tty:          svc.Tty,
		OpenStdin:    svc.StdinOpen,
		StopSignal:   svc.StopSignal,
		ExposedPorts: nat.PortSet{},
		Labels:       mergeLabels(svc.Labels, composeLabels(projectName, svc.Name, number, configHash)),
	}
	if svc.Command != nil {
		cfg.Cmd = []string(svc.Command)
	}
	if svc.Entrypoint != nil {
		cfg.Entrypoint = []string(svc.Entrypoint)
	}
	if hc := composeHealthcheck(svc.HealthCheck); hc != nil {
		cfg.Healthcheck = hc
	}

	hostCfg := &container.HostConfig{
		Privileged:     svc.Privileged,
		ReadonlyRootfs: svc.ReadOnly,
		Mounts:         []mount.Mount{},
		PortBindings:   nat.PortMap{},
		Init:           svc.Init,
		NetworkMode:    container.NetworkMode(svc.NetworkMode),
		CapAdd:         ensureCapPrefix(svc.CapAdd),
		CapDrop:        ensureCapPrefix(svc.CapDrop),
		SecurityOpt:    svc.SecurityOpt,
		Sysctls:        svc.Sysctls,
		GroupAdd:       svc.GroupAdd,
		ExtraHosts:     svc.ExtraHosts.AsList(":"),
		DNS:            []string(svc.DNS),
		DNSSearch:      []string(svc.DNSSearch),
		ShmSize:        int64(svc.ShmSize),
		Tmpfs:          tmpfsMap(svc.Tmpfs),
		PidMode:        container.PidMode(svc.Pid),
		IpcMode:        container.IpcMode(svc.Ipc),
		UTSMode:        container.UTSMode(svc.Uts),
		Runtime:        svc.Runtime,
		OomScoreAdj:    int(svc.OomScoreAdj),
	}
	if svc.Logging != nil {
		hostCfg.LogConfig = container.LogConfig{Type: svc.Logging.Driver, Config: svc.Logging.Options}
	}
	if svc.StopGracePeriod != nil {
		sec := int(time.Duration(*svc.StopGracePeriod).Seconds())
		cfg.StopTimeout = &sec
	}
	if len(svc.Ulimits) > 0 {
		names := make([]string, 0, len(svc.Ulimits))
		for n := range svc.Ulimits {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			u := svc.Ulimits[n]
			if u == nil {
				continue
			}
			soft, hard := u.Soft, u.Hard
			if u.Single > 0 {
				soft, hard = u.Single, u.Single
			}
			hostCfg.Ulimits = append(hostCfg.Ulimits, &container.Ulimit{Name: n, Soft: int64(soft), Hard: int64(hard)})
		}
	}
	for _, d := range svc.Devices {
		hostCfg.Devices = append(hostCfg.Devices, container.DeviceMapping{
			PathOnHost: d.Source, PathInContainer: d.Target, CgroupPermissions: d.Permissions,
		})
	}
	applyComposeResources(hostCfg, svc)

	// 端口映射
	for _, p := range svc.Ports {
		if p.Target == 0 {
			continue
		}
		proto := defaultString(p.Protocol, "tcp")
		port, err := nat.NewPort(proto, strconv.Itoa(int(p.Target)))
		if err != nil {
			return fmt.Errorf("端口配置错误: %w", err)
		}
		cfg.ExposedPorts[port] = struct{}{}
		binding := nat.PortBinding{HostIP: p.HostIP}
		if p.Published != "" {
			binding.HostPort = p.Published
		}
		hostCfg.PortBindings[port] = append(hostCfg.PortBindings[port], binding)
	}
	for _, e := range svc.Expose {
		port, err := nat.NewPort("tcp", e)
		if err == nil {
			cfg.ExposedPorts[port] = struct{}{}
		}
	}

	// 挂载
	mounts, err := composeMounts(project, projectName, svc)
	if err != nil {
		return err
	}
	hostCfg.Mounts = append(hostCfg.Mounts, mounts...)

	// 网络
	var networking *network.NetworkingConfig
	if svc.NetworkMode == "" {
		serviceNets := svc.Networks
		if len(serviceNets) == 0 {
			serviceNets = map[string]*types.ServiceNetworkConfig{"default": nil}
		}
		endpoints := make(map[string]*network.EndpointSettings, len(serviceNets))
		first := ""
		for key, netCfg := range serviceNets {
			netName, ok := networks[key]
			if !ok {
				return fmt.Errorf("service「%s」引用了未定义网络「%s」", svc.Name, key)
			}
			if first == "" {
				first = netName
			}
			ep := &network.EndpointSettings{Aliases: []string{svc.Name, name}}
			if netCfg != nil {
				ep.Aliases = append(ep.Aliases, netCfg.Aliases...)
				if netCfg.Ipv4Address != "" || netCfg.Ipv6Address != "" || netCfg.MacAddress != "" {
					ep.IPAMConfig = &network.EndpointIPAMConfig{
						IPv4Address: netCfg.Ipv4Address, IPv6Address: netCfg.Ipv6Address,
						LinkLocalIPs: netCfg.LinkLocalIPs,
					}
				}
				if netCfg.MacAddress != "" {
					ep.MacAddress = netCfg.MacAddress
				}
				if len(netCfg.DriverOpts) > 0 {
					ep.DriverOpts = netCfg.DriverOpts
				}
			}
			endpoints[netName] = ep
		}
		hostCfg.NetworkMode = container.NetworkMode(first)
		networking = &network.NetworkingConfig{EndpointsConfig: endpoints}
	}

	resp, err := c.ContainerCreate(ctx, cfg, hostCfg, networking, nil, name)
	if err != nil {
		return err
	}
	if err := c.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("容器 %s 创建成功但启动失败: %w", name, err)
	}
	return nil
}

func composeMounts(project *types.Project, projectName string, svc *types.ServiceConfig) ([]mount.Mount, error) {
	out := make([]mount.Mount, 0, len(svc.Volumes))
	for _, v := range svc.Volumes {
		switch v.Type {
		case "bind":
			if !isAbsolutePath(v.Source) {
				return nil, fmt.Errorf("service「%s」的绑定挂载「%s」不是绝对路径", svc.Name, v.Source)
			}
			m := mount.Mount{Type: mount.TypeBind, Source: v.Source, Target: v.Target, ReadOnly: v.ReadOnly}
			if v.Bind != nil && v.Bind.Propagation != "" {
				m.BindOptions = &mount.BindOptions{Propagation: mount.Propagation(v.Bind.Propagation)}
			}
			out = append(out, m)
		case "tmpfs":
			m := mount.Mount{Type: mount.TypeTmpfs, Target: v.Target}
			if v.Tmpfs != nil {
				m.TmpfsOptions = &mount.TmpfsOptions{SizeBytes: int64(v.Tmpfs.Size), Mode: os.FileMode(v.Tmpfs.Mode)}
			}
			out = append(out, m)
		case "volume", "":
			source := v.Source
			if source == "" {
				return nil, fmt.Errorf("service「%s」的数据卷挂载缺少 source", svc.Name)
			}
			if top, ok := project.Volumes[source]; ok {
				if top.Name != "" {
					source = top.Name
				} else if !top.External {
					source = projectName + "_" + source
				}
			}
			m := mount.Mount{Type: mount.TypeVolume, Source: source, Target: v.Target, ReadOnly: v.ReadOnly}
			if v.Volume != nil {
				m.VolumeOptions = &mount.VolumeOptions{NoCopy: v.Volume.NoCopy}
			}
			out = append(out, m)
		case "image":
			return nil, fmt.Errorf("service「%s」使用了 image 类型挂载，暂不支持", svc.Name)
		default:
			return nil, fmt.Errorf("service「%s」使用了不支持的挂载类型「%s」", svc.Name, v.Type)
		}
	}
	return out, nil
}

func applyComposeResources(hostCfg *container.HostConfig, svc *types.ServiceConfig) {
	if svc.CPUS > 0 {
		hostCfg.NanoCPUs = int64(float64(svc.CPUS) * 1e9)
	}
	if svc.CPUShares > 0 {
		hostCfg.CPUShares = svc.CPUShares
	}
	if svc.MemLimit > 0 {
		hostCfg.Memory = int64(svc.MemLimit)
	}
	if svc.MemReservation > 0 {
		hostCfg.MemoryReservation = int64(svc.MemReservation)
	}
	if svc.PidsLimit > 0 {
		v := svc.PidsLimit
		hostCfg.PidsLimit = &v
	}
	if svc.Deploy != nil {
		if limits := svc.Deploy.Resources.Limits; limits != nil {
			if limits.NanoCPUs > 0 {
				hostCfg.NanoCPUs = int64(float64(limits.NanoCPUs) * 1e9)
			}
			if limits.MemoryBytes > 0 {
				hostCfg.Memory = int64(limits.MemoryBytes)
			}
			if limits.Pids > 0 {
				v := limits.Pids
				hostCfg.PidsLimit = &v
			}
		}
		if reservations := svc.Deploy.Resources.Reservations; reservations != nil {
			if reservations.MemoryBytes > 0 {
				hostCfg.MemoryReservation = int64(reservations.MemoryBytes)
			}
			if reservations.NanoCPUs > 0 {
				hostCfg.NanoCPUs = int64(float64(reservations.NanoCPUs) * 1e9)
			}
		}
	}
	if svc.Restart != "" {
		hostCfg.RestartPolicy = parseRestartPolicy(svc.Restart)
	} else if svc.Deploy != nil && svc.Deploy.RestartPolicy != nil {
		hostCfg.RestartPolicy = parseComposeRestart(svc.Deploy.RestartPolicy)
	}
}

func parseComposeRestart(rp *types.RestartPolicy) container.RestartPolicy {
	cond := container.RestartPolicyDisabled
	switch strings.ToLower(rp.Condition) {
	case "any":
		cond = container.RestartPolicyAlways
	case "on-failure":
		cond = container.RestartPolicyOnFailure
	}
	out := container.RestartPolicy{Name: cond}
	if rp.MaxAttempts != nil {
		out.MaximumRetryCount = int(*rp.MaxAttempts)
	}
	return out
}

func composeHealthcheck(hc *types.HealthCheckConfig) *container.HealthConfig {
	if hc == nil || hc.Disable || len(hc.Test) == 0 {
		return nil
	}
	out := &container.HealthConfig{Test: []string(hc.Test)}
	if hc.Interval != nil {
		out.Interval = time.Duration(*hc.Interval)
	}
	if hc.Timeout != nil {
		out.Timeout = time.Duration(*hc.Timeout)
	}
	if hc.StartPeriod != nil {
		out.StartPeriod = time.Duration(*hc.StartPeriod)
	}
	if hc.StartInterval != nil {
		out.StartInterval = time.Duration(*hc.StartInterval)
	}
	if hc.Retries != nil {
		out.Retries = int(*hc.Retries)
	}
	return out
}

func buildSwarmServiceSpec(
	project *types.Project, projectName string, svc *types.ServiceConfig,
	networks map[string]string, secretRefs map[string]swarm.SecretReference, configRefs map[string]swarm.ConfigReference,
) (swarm.ServiceSpec, error) {
	if svc.Build != nil {
		return swarm.ServiceSpec{}, fmt.Errorf("service「%s」使用了 build：Swarm Stack 不支持在服务端构建镜像", svc.Name)
	}
	if svc.Privileged {
		return swarm.ServiceSpec{}, fmt.Errorf("service「%s」使用了 privileged：Docker Swarm 不支持该选项", svc.Name)
	}
	labels := mergeLabels(svc.Labels, map[string]string{
		"com.docker.stack.namespace": projectName,
		"com.docker.stack.image":     svc.Image,
	})
	if svc.Deploy != nil {
		labels = mergeLabels(labels, svc.Deploy.Labels)
	}
	cs := &swarm.ContainerSpec{
		Image:       svc.Image,
		Env:         composeEnv(svc.Environment),
		Hostname:    svc.Hostname,
		Dir:         svc.WorkingDir,
		User:        svc.User,
		Groups:      svc.GroupAdd,
		Init:        svc.Init,
		TTY:         svc.Tty,
		OpenStdin:   svc.StdinOpen,
		ReadOnly:    svc.ReadOnly,
		StopSignal:  svc.StopSignal,
		Hosts:       svc.ExtraHosts.AsList(":"),
		Sysctls:     svc.Sysctls,
		OomScoreAdj: svc.OomScoreAdj,
	}
	if svc.Command != nil {
		cs.Args = []string(svc.Command)
	}
	if svc.Entrypoint != nil {
		cs.Command = []string(svc.Entrypoint)
	}
	if hc := composeHealthcheck(svc.HealthCheck); hc != nil {
		cs.Healthcheck = hc
	}
	if svc.StopGracePeriod != nil {
		d := time.Duration(*svc.StopGracePeriod)
		cs.StopGracePeriod = &d
	}
	if len(svc.DNS) > 0 || len(svc.DNSSearch) > 0 {
		cs.DNSConfig = &swarm.DNSConfig{Nameservers: []string(svc.DNS), Search: []string(svc.DNSSearch)}
	}
	if len(svc.Ulimits) > 0 {
		for n, u := range svc.Ulimits {
			if u == nil {
				continue
			}
			soft, hard := u.Soft, u.Hard
			if u.Single > 0 {
				soft, hard = u.Single, u.Single
			}
			cs.Ulimits = append(cs.Ulimits, &container.Ulimit{Name: n, Soft: int64(soft), Hard: int64(hard)})
		}
	}
	cs.CapabilityAdd = ensureCapPrefix(svc.CapAdd)
	cs.CapabilityDrop = ensureCapPrefix(svc.CapDrop)
	for _, m := range svc.Volumes {
		mm, err := swarmMount(project, projectName, svc.Name, m)
		if err != nil {
			return swarm.ServiceSpec{}, err
		}
		cs.Mounts = append(cs.Mounts, mm)
	}
	for _, ref := range svc.Secrets {
		sr, ok := secretRefs[ref.Source]
		if !ok {
			return swarm.ServiceSpec{}, fmt.Errorf("service「%s」引用了未声明的密钥「%s」", svc.Name, ref.Source)
		}
		target := ref.Target
		if target == "" {
			target = ref.Source
		}
		sr.File = &swarm.SecretReferenceFileTarget{Name: target, UID: ref.UID, GID: ref.GID}
		if ref.Mode != nil {
			sr.File.Mode = os.FileMode(*ref.Mode)
		}
		cs.Secrets = append(cs.Secrets, &sr)
	}
	for _, ref := range svc.Configs {
		cr, ok := configRefs[ref.Source]
		if !ok {
			return swarm.ServiceSpec{}, fmt.Errorf("service「%s」引用了未声明的配置「%s」", svc.Name, ref.Source)
		}
		target := ref.Target
		if target == "" {
			target = ref.Source
		}
		cr.File = &swarm.ConfigReferenceFileTarget{Name: target, UID: ref.UID, GID: ref.GID}
		if ref.Mode != nil {
			cr.File.Mode = os.FileMode(*ref.Mode)
		}
		cs.Configs = append(cs.Configs, &cr)
	}

	spec := swarm.ServiceSpec{
		Annotations:  swarm.Annotations{Name: projectName + "_" + svc.Name, Labels: labels},
		TaskTemplate: swarm.TaskSpec{ContainerSpec: cs},
	}

	// 网络
	serviceNets := svc.Networks
	if len(serviceNets) == 0 {
		serviceNets = map[string]*types.ServiceNetworkConfig{"default": nil}
	}
	for key, netCfg := range serviceNets {
		name, ok := networks[key]
		if !ok {
			return swarm.ServiceSpec{}, fmt.Errorf("service「%s」引用了未定义网络「%s」", svc.Name, key)
		}
		aliases := []string{svc.Name}
		if netCfg != nil {
			aliases = append(aliases, netCfg.Aliases...)
		}
		spec.TaskTemplate.Networks = append(spec.TaskTemplate.Networks, swarm.NetworkAttachmentConfig{Target: name, Aliases: aliases})
	}

	// 端口
	ports := make([]swarm.PortConfig, 0, len(svc.Ports))
	for _, p := range svc.Ports {
		if p.Target == 0 {
			continue
		}
		proto := swarm.PortConfigProtocolTCP
		switch strings.ToLower(p.Protocol) {
		case "udp":
			proto = swarm.PortConfigProtocolUDP
		case "sctp":
			proto = swarm.PortConfigProtocolSCTP
		}
		mode := swarm.PortConfigPublishModeIngress
		if strings.EqualFold(p.Mode, "host") {
			mode = swarm.PortConfigPublishModeHost
		}
		var published uint32
		if p.Published != "" {
			if n, err := strconv.ParseUint(p.Published, 10, 32); err == nil {
				published = uint32(n)
			}
		}
		ports = append(ports, swarm.PortConfig{
			Name: p.Name, Protocol: proto, TargetPort: p.Target, PublishedPort: published, PublishMode: mode,
		})
	}
	spec.EndpointSpec = &swarm.EndpointSpec{Ports: ports}

	// 副本模式 / 资源 / 重启策略 / 调度约束
	deploy := svc.Deploy
	if deploy == nil {
		deploy = &types.DeployConfig{}
	}
	if strings.EqualFold(deploy.Mode, "global") {
		spec.Mode = swarm.ServiceMode{Global: &swarm.GlobalService{}}
	} else {
		replicas := uint64(1)
		if deploy.Replicas != nil && *deploy.Replicas >= 0 {
			replicas = uint64(*deploy.Replicas)
		}
		spec.Mode = swarm.ServiceMode{Replicated: &swarm.ReplicatedService{Replicas: &replicas}}
	}
	if deploy.RestartPolicy != nil {
		rp := &swarm.RestartPolicy{Condition: swarm.RestartPolicyConditionAny}
		switch strings.ToLower(deploy.RestartPolicy.Condition) {
		case "none":
			rp.Condition = swarm.RestartPolicyConditionNone
		case "on-failure":
			rp.Condition = swarm.RestartPolicyConditionOnFailure
		}
		if deploy.RestartPolicy.Delay != nil {
			d := time.Duration(*deploy.RestartPolicy.Delay)
			rp.Delay = &d
		}
		if deploy.RestartPolicy.MaxAttempts != nil {
			rp.MaxAttempts = deploy.RestartPolicy.MaxAttempts
		}
		if deploy.RestartPolicy.Window != nil {
			d := time.Duration(*deploy.RestartPolicy.Window)
			rp.Window = &d
		}
		spec.TaskTemplate.RestartPolicy = rp
	}
	res := &swarm.ResourceRequirements{}
	if limits := deploy.Resources.Limits; limits != nil {
		res.Limits = &swarm.Limit{
			NanoCPUs: int64(float64(limits.NanoCPUs) * 1e9), MemoryBytes: int64(limits.MemoryBytes), Pids: limits.Pids,
		}
	}
	if reservations := deploy.Resources.Reservations; reservations != nil {
		res.Reservations = &swarm.Resources{
			NanoCPUs: int64(float64(reservations.NanoCPUs) * 1e9), MemoryBytes: int64(reservations.MemoryBytes),
		}
	}
	if res.Limits != nil || res.Reservations != nil {
		spec.TaskTemplate.Resources = res
	}
	if len(deploy.Placement.Constraints) > 0 || deploy.Placement.MaxReplicas > 0 {
		spec.TaskTemplate.Placement = &swarm.Placement{
			Constraints: deploy.Placement.Constraints, MaxReplicas: deploy.Placement.MaxReplicas,
		}
	}
	if deploy.UpdateConfig != nil {
		spec.UpdateConfig = convertUpdateConfig(deploy.UpdateConfig)
	}
	if deploy.RollbackConfig != nil {
		spec.RollbackConfig = convertUpdateConfig(deploy.RollbackConfig)
	}
	return spec, nil
}

func swarmMount(project *types.Project, projectName, serviceName string, v types.ServiceVolumeConfig) (mount.Mount, error) {
	switch v.Type {
	case "bind":
		if !isAbsolutePath(v.Source) {
			return mount.Mount{}, fmt.Errorf("service「%s」的绑定挂载「%s」不是绝对路径", serviceName, v.Source)
		}
		m := mount.Mount{Type: mount.TypeBind, Source: v.Source, Target: v.Target, ReadOnly: v.ReadOnly}
		if v.Bind != nil && v.Bind.Propagation != "" {
			m.BindOptions = &mount.BindOptions{Propagation: mount.Propagation(v.Bind.Propagation)}
		}
		return m, nil
	case "tmpfs":
		m := mount.Mount{Type: mount.TypeTmpfs, Target: v.Target}
		if v.Tmpfs != nil {
			m.TmpfsOptions = &mount.TmpfsOptions{SizeBytes: int64(v.Tmpfs.Size), Mode: os.FileMode(v.Tmpfs.Mode)}
		}
		return m, nil
	case "volume", "":
		source := v.Source
		if source == "" {
			return mount.Mount{}, fmt.Errorf("service「%s」的数据卷挂载缺少 source", serviceName)
		}
		if top, ok := project.Volumes[source]; ok {
			if top.Name != "" {
				source = top.Name
			} else if !top.External {
				source = projectName + "_" + source
			}
		}
		m := mount.Mount{Type: mount.TypeVolume, Source: source, Target: v.Target, ReadOnly: v.ReadOnly}
		if v.Volume != nil {
			m.VolumeOptions = &mount.VolumeOptions{NoCopy: v.Volume.NoCopy}
		}
		return m, nil
	default:
		return mount.Mount{}, fmt.Errorf("service「%s」使用了不支持的挂载类型「%s」", serviceName, v.Type)
	}
}

func convertUpdateConfig(u *types.UpdateConfig) *swarm.UpdateConfig {
	if u == nil {
		return nil
	}
	out := &swarm.UpdateConfig{Parallelism: 0, FailureAction: u.FailureAction, MaxFailureRatio: u.MaxFailureRatio, Order: u.Order}
	if u.Parallelism != nil {
		out.Parallelism = *u.Parallelism
	}
	out.Delay = time.Duration(u.Delay)
	out.Monitor = time.Duration(u.Monitor)
	return out
}

func (m *Manager) removeComposeContainers(ctx context.Context, c *client.Client, projectName string, stopOnly bool) error {
	list, err := c.ContainerList(ctx, container.ListOptions{
		All: true, Filters: filters.NewArgs(filters.Arg("label", "com.docker.compose.project="+projectName)),
	})
	if err != nil {
		return err
	}
	for _, item := range list {
		timeout := 5
		_ = c.ContainerStop(ctx, item.ID, container.StopOptions{Timeout: &timeout})
		if stopOnly {
			continue
		}
		if err := c.ContainerRemove(ctx, item.ID, container.RemoveOptions{Force: true}); err != nil {
			return err
		}
	}
	return nil
}

func serviceOrder(project *types.Project) ([]string, error) {
	visited := map[string]int{}
	var out []string
	var visit func(string) error
	visit = func(name string) error {
		switch visited[name] {
		case 1:
			return fmt.Errorf("service 依赖存在环：%s", name)
		case 2:
			return nil
		}
		visited[name] = 1
		svc, ok := project.Services[name]
		if !ok {
			return fmt.Errorf("service「%s」不存在", name)
		}
		deps := make([]string, 0, len(svc.DependsOn))
		for dep := range svc.DependsOn {
			deps = append(deps, dep)
		}
		sort.Strings(deps)
		for _, dep := range deps {
			if err := visit(dep); err != nil {
				return err
			}
		}
		visited[name] = 2
		out = append(out, name)
		return nil
	}
	names := make([]string, 0, len(project.Services))
	for n := range project.Services {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if err := visit(n); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func composeLabels(project, service string, number int, configHash string) map[string]string {
	return map[string]string{
		"com.docker.compose.project":              project,
		"com.docker.compose.service":              service,
		"com.docker.compose.container-number":     strconv.Itoa(number),
		"com.docker.compose.oneoff":               "False",
		"com.docker.compose.project.config_files": "docker-compose.yml",
		"com.docker.compose.config-hash":          configHash,
	}
}

func composeEnv(env types.MappingWithEquals) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		v := env[k]
		if v == nil {
			if val, ok := os.LookupEnv(k); ok {
				out = append(out, k+"="+val)
			}
			continue
		}
		out = append(out, k+"="+*v)
	}
	return out
}

func composeHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", sum[:8])
}

func parseDotEnv(content string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if len(v) >= 2 {
			if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
				v = v[1 : len(v)-1]
			}
		}
		out[k] = v
	}
	return out
}

func normalizeProjectName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-_")
	if out == "" {
		out = "stack"
	}
	if len(out) > 60 {
		out = out[:60]
	}
	return out
}

func tmpfsMap(list types.StringList) map[string]string {
	if len(list) == 0 {
		return nil
	}
	out := make(map[string]string, len(list))
	for _, target := range list {
		out[target] = ""
	}
	return out
}

func defaultString(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func mergeLabels(base map[string]string, extra map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

func ensureCapPrefix(caps []string) []string {
	out := make([]string, 0, len(caps))
	for _, c := range caps {
		if c == "" {
			continue
		}
		if !strings.HasPrefix(strings.ToUpper(c), "CAP_") {
			c = "CAP_" + strings.ToUpper(c)
		}
		out = append(out, c)
	}
	return out
}

func isAbsolutePath(p string) bool {
	return filepath.IsAbs(p) || (len(p) > 2 && p[1] == ':' && (p[2] == '\\' || p[2] == '/'))
}
