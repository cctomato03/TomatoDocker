package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/swarm"

	"web-basic/internal/dockerstore"
)

// SwarmState 集群状态。
func (m *Manager) SwarmState(ctx context.Context, env *dockerstore.Environment) (*SwarmState, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return nil, err
	}
	info, err := c.Info(ctx)
	if err != nil {
		return nil, err
	}
	out := &SwarmState{
		Active:           info.Swarm.LocalNodeState == swarm.LocalNodeStateActive,
		NodeID:           info.Swarm.NodeID,
		NodeAddr:         info.Swarm.NodeAddr,
		LocalNodeState:   string(info.Swarm.LocalNodeState),
		ControlAvailable: info.Swarm.ControlAvailable,
		Error:            info.Swarm.Error,
		Nodes:            info.Swarm.Nodes,
		Managers:         info.Swarm.Managers,
	}
	if !out.Active {
		return out, nil
	}
	cluster, err := c.SwarmInspect(ctx)
	if err == nil {
		out.ClusterID = cluster.ID
		out.Name = cluster.Spec.Annotations.Name
		out.Labels = cluster.Spec.Annotations.Labels
		out.CreatedAt = cluster.Meta.CreatedAt.Format(time.RFC3339)
		out.UpdatedAt = cluster.Meta.UpdatedAt.Format(time.RFC3339)
	}
	return out, nil
}

// SwarmInit 初始化 Swarm。
func (m *Manager) SwarmInit(ctx context.Context, env *dockerstore.Environment, advertiseAddr, listenAddr string) (string, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(listenAddr) == "" {
		listenAddr = "0.0.0.0:2377"
	}
	return c.SwarmInit(ctx, swarm.InitRequest{
		ListenAddr:    listenAddr,
		AdvertiseAddr: strings.TrimSpace(advertiseAddr),
	})
}

// SwarmLeave 退出 Swarm。
func (m *Manager) SwarmLeave(ctx context.Context, env *dockerstore.Environment, force bool) error {
	c, err := m.Client(ctx, env)
	if err != nil {
		return err
	}
	return c.SwarmLeave(ctx, force)
}

// Nodes 节点列表。
func (m *Manager) Nodes(ctx context.Context, env *dockerstore.Environment) ([]Node, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return nil, err
	}
	list, err := c.NodeList(ctx, swarm.NodeListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]Node, 0, len(list))
	for _, n := range list {
		addr := n.Status.Addr
		if n.ManagerStatus != nil && n.ManagerStatus.Addr != "" {
			addr = n.ManagerStatus.Addr
		}
		item := Node{
			ID:           n.ID,
			Hostname:     n.Description.Hostname,
			Role:         string(n.Spec.Role),
			Availability: string(n.Spec.Availability),
			Status:       string(n.Status.State),
			Addr:         addr,
			Version:      n.Description.Engine.EngineVersion,
			PlatformOS:   n.Description.Platform.OS,
			PlatformArch: n.Description.Platform.Architecture,
			NCPU:         n.Description.Resources.NanoCPUs / 1000000000,
			MemTotal:     n.Description.Resources.MemoryBytes,
			Labels:       n.Spec.Labels,
			EngineLabels: n.Description.Engine.Labels,
			CreatedAt:    n.Meta.CreatedAt.Format(time.RFC3339),
			UpdatedAt:    n.Meta.UpdatedAt.Format(time.RFC3339),
		}
		if n.ManagerStatus != nil {
			item.Leader = n.ManagerStatus.Leader
			item.ManagerStatus = string(n.ManagerStatus.Reachability)
		}
		out = append(out, item)
	}
	return out, nil
}

// UpdateNode 修改节点角色/可用性/标签。
func (m *Manager) UpdateNode(ctx context.Context, env *dockerstore.Environment, id, role, availability string, labels map[string]string) error {
	c, err := m.Client(ctx, env)
	if err != nil {
		return err
	}
	n, _, err := c.NodeInspectWithRaw(ctx, id)
	if err != nil {
		return err
	}
	spec := n.Spec
	if role != "" {
		spec.Role = swarm.NodeRole(role)
	}
	if availability != "" {
		spec.Availability = swarm.NodeAvailability(availability)
	}
	if labels != nil {
		spec.Labels = labels
	}
	return c.NodeUpdate(ctx, id, n.Version, spec)
}

// RemoveNode 移除节点。
func (m *Manager) RemoveNode(ctx context.Context, env *dockerstore.Environment, id string, force bool) error {
	c, err := m.Client(ctx, env)
	if err != nil {
		return err
	}
	return c.NodeRemove(ctx, id, swarm.NodeRemoveOptions{Force: force})
}

// Services 服务列表。
func (m *Manager) Services(ctx context.Context, env *dockerstore.Environment, keyword string) ([]Service, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return nil, err
	}
	list, err := c.ServiceList(ctx, swarm.ServiceListOptions{Status: true})
	if err != nil {
		return nil, err
	}
	kw := strings.ToLower(strings.TrimSpace(keyword))
	out := make([]Service, 0, len(list))
	for _, s := range list {
		if kw != "" && !strings.Contains(strings.ToLower(s.Spec.Name), kw) && !strings.Contains(s.ID, kw) {
			continue
		}
		out = append(out, mapService(s))
	}
	return out, nil
}

func mapService(s swarm.Service) Service {
	image := ""
	cmd := ""
	if s.Spec.TaskTemplate.ContainerSpec != nil {
		image = s.Spec.TaskTemplate.ContainerSpec.Image
		cmd = strings.Join(append(
			append([]string{}, s.Spec.TaskTemplate.ContainerSpec.Command...),
			s.Spec.TaskTemplate.ContainerSpec.Args...,
		), " ")
	}
	mode := "replicated"
	var replicas uint64
	if s.Spec.Mode.Global != nil {
		mode = "global"
	} else if s.Spec.Mode.Replicated != nil && s.Spec.Mode.Replicated.Replicas != nil {
		replicas = *s.Spec.Mode.Replicated.Replicas
	}
	ports := make([]ServicePort, 0, len(s.Endpoint.Ports))
	for _, p := range s.Endpoint.Ports {
		ports = append(ports, ServicePort{
			Name: p.Name, Protocol: string(p.Protocol), TargetPort: p.TargetPort,
			PublishedPort: p.PublishedPort, PublishMode: string(p.PublishMode),
		})
	}
	updateStatus := ""
	if s.UpdateStatus != nil {
		updateStatus = string(s.UpdateStatus.State)
	}
	var running uint64
	if s.ServiceStatus != nil {
		running = s.ServiceStatus.RunningTasks
	}
	return Service{
		ID:           s.ID,
		Name:         s.Spec.Name,
		Image:        image,
		Mode:         mode,
		Replicas:     replicas,
		Running:      running,
		Ports:        ports,
		StackName:    s.Spec.Labels["com.docker.stack.namespace"],
		Labels:       s.Spec.Labels,
		Command:      cmd,
		CreatedAt:    s.Meta.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    s.Meta.UpdatedAt.Format(time.RFC3339),
		UpdateStatus: updateStatus,
	}
}

// ServiceDetail 服务详情。
func (m *Manager) ServiceDetail(ctx context.Context, env *dockerstore.Environment, id string) (map[string]any, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return nil, err
	}
	s, raw, err := c.ServiceInspectWithRaw(ctx, id, swarm.ServiceInspectOptions{})
	if err != nil {
		return nil, err
	}
	var spec any
	_ = jsonUnmarshal(raw, &spec)
	return map[string]any{
		"id":        s.ID,
		"name":      s.Spec.Name,
		"createdAt": s.Meta.CreatedAt,
		"updatedAt": s.Meta.UpdatedAt,
		"spec":      s.Spec,
	}, nil
}

// CreateService 创建 Swarm 服务。
func (m *Manager) CreateService(ctx context.Context, env *dockerstore.Environment, req *CreateServiceRequest) (string, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return "", err
	}
	spec, err := m.buildServiceSpec(ctx, env, req, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.ServiceCreate(ctx, spec, swarm.ServiceCreateOptions{})
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (m *Manager) buildServiceSpec(ctx context.Context, env *dockerstore.Environment, req *CreateServiceRequest, base *swarm.ServiceSpec) (swarm.ServiceSpec, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return swarm.ServiceSpec{}, err
	}
	labels := map[string]string{}
	annotations := swarm.Annotations{Name: req.Name, Labels: labels}
	for k, v := range req.Labels {
		labels[k] = v
	}
	containerSpec := &swarm.ContainerSpec{
		Image: req.Image,
		Env:   req.Env,
	}
	if req.Command != "" {
		containerSpec.Args = splitCommand(req.Command)
	}
	spec := swarm.ServiceSpec{
		Annotations:  annotations,
		TaskTemplate: swarm.TaskSpec{ContainerSpec: containerSpec},
	}
	if base != nil {
		spec = *base
		spec.TaskTemplate.ContainerSpec = containerSpec
		if req.Name != "" {
			spec.Annotations.Name = req.Name
		}
		if req.Labels != nil {
			spec.Annotations.Labels = req.Labels
		}
	}
	spec.Mode = swarm.ServiceMode{Replicated: &swarm.ReplicatedService{Replicas: &req.Replicas}}
	if req.RestartPolicy != "" {
		cond := swarm.RestartPolicyConditionAny
		switch strings.ToLower(req.RestartPolicy) {
		case "none":
			cond = swarm.RestartPolicyConditionNone
		case "on-failure":
			cond = swarm.RestartPolicyConditionOnFailure
		}
		spec.TaskTemplate.RestartPolicy = &swarm.RestartPolicy{Condition: cond}
	}
	if len(req.Constraints) > 0 {
		spec.TaskTemplate.Placement = &swarm.Placement{Constraints: req.Constraints}
	}
	if req.NetworkID != "" {
		n, err := c.NetworkInspect(ctx, req.NetworkID, network.InspectOptions{})
		if err != nil {
			return swarm.ServiceSpec{}, fmt.Errorf("查询网络失败: %w", err)
		}
		spec.TaskTemplate.Networks = []swarm.NetworkAttachmentConfig{{Target: n.ID, Aliases: []string{req.Name}}}
	}
	ports := make([]swarm.PortConfig, 0, len(req.Ports))
	for _, p := range req.Ports {
		if p.ContainerPort <= 0 {
			continue
		}
		proto := swarm.PortConfigProtocolTCP
		if strings.EqualFold(p.Protocol, "udp") {
			proto = swarm.PortConfigProtocolUDP
		}
		ports = append(ports, swarm.PortConfig{
			Protocol:      proto,
			TargetPort:    uint32(p.ContainerPort),
			PublishedPort: uint32(p.HostPort),
			PublishMode:   swarm.PortConfigPublishModeIngress,
		})
	}
	spec.EndpointSpec = &swarm.EndpointSpec{Ports: ports}
	return spec, nil
}

// UpdateService 修改 Swarm 服务。
func (m *Manager) UpdateService(ctx context.Context, env *dockerstore.Environment, id string, req *UpdateServiceRequest) error {
	c, err := m.Client(ctx, env)
	if err != nil {
		return err
	}
	cur, _, err := c.ServiceInspectWithRaw(ctx, id, swarm.ServiceInspectOptions{})
	if err != nil {
		return err
	}
	spec := cur.Spec
	cs := spec.TaskTemplate.ContainerSpec
	if cs == nil {
		cs = &swarm.ContainerSpec{}
	}
	if req.Image != "" {
		cs.Image = req.Image
	}
	if req.Env != nil {
		cs.Env = req.Env
	}
	if req.Command != "" {
		cs.Args = splitCommand(req.Command)
	}
	if req.Constraints != nil {
		spec.TaskTemplate.Placement = &swarm.Placement{Constraints: req.Constraints}
	}
	if req.Labels != nil {
		spec.Annotations.Labels = req.Labels
	}
	// global 服务没有副本数概念：编辑时不要因为前端默认值把 global 改成 replicated 0。
	if req.Replicas != nil && spec.Mode.Global == nil {
		spec.Mode = swarm.ServiceMode{Replicated: &swarm.ReplicatedService{Replicas: req.Replicas}}
	}
	if req.NetworkID != "" {
		n, err := c.NetworkInspect(ctx, req.NetworkID, network.InspectOptions{})
		if err != nil {
			return fmt.Errorf("查询网络失败: %w", err)
		}
		spec.TaskTemplate.Networks = []swarm.NetworkAttachmentConfig{{
			Target: n.ID, Aliases: []string{cur.Spec.Name},
		}}
	}
	if req.Ports != nil {
		ports := make([]swarm.PortConfig, 0, len(req.Ports))
		for _, p := range req.Ports {
			if p.ContainerPort <= 0 {
				continue
			}
			ports = append(ports, swarm.PortConfig{
				Protocol:      swarm.PortConfigProtocolTCP,
				TargetPort:    uint32(p.ContainerPort),
				PublishedPort: uint32(p.HostPort),
				PublishMode:   swarm.PortConfigPublishModeIngress,
			})
		}
		spec.EndpointSpec = &swarm.EndpointSpec{Ports: ports}
	}
	spec.TaskTemplate.ContainerSpec = cs
	resp, err := c.ServiceUpdate(ctx, id, cur.Version, spec, swarm.ServiceUpdateOptions{})
	if err != nil {
		return err
	}
	if len(resp.Warnings) > 0 {
		return fmt.Errorf("%s", strings.Join(resp.Warnings, "; "))
	}
	return nil
}

// ScaleService 调整副本数。
func (m *Manager) ScaleService(ctx context.Context, env *dockerstore.Environment, id string, replicas uint64) error {
	c, err := m.Client(ctx, env)
	if err != nil {
		return err
	}
	cur, _, err := c.ServiceInspectWithRaw(ctx, id, swarm.ServiceInspectOptions{})
	if err != nil {
		return err
	}
	spec := cur.Spec
	if spec.Mode.Global != nil {
		return fmt.Errorf("global 服务不支持修改副本数")
	}
	spec.Mode = swarm.ServiceMode{Replicated: &swarm.ReplicatedService{Replicas: &replicas}}
	_, err = c.ServiceUpdate(ctx, id, cur.Version, spec, swarm.ServiceUpdateOptions{})
	return err
}

// RemoveService 删除服务。
func (m *Manager) RemoveService(ctx context.Context, env *dockerstore.Environment, id string) error {
	c, err := m.Client(ctx, env)
	if err != nil {
		return err
	}
	return c.ServiceRemove(ctx, id)
}

// ServiceLogs 服务日志。
func (m *Manager) ServiceLogs(ctx context.Context, env *dockerstore.Environment, id string, tail int) (string, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return "", err
	}
	if tail <= 0 {
		tail = 200
	}
	if m.cfg.MaxLogLines > 0 && tail > m.cfg.MaxLogLines {
		tail = m.cfg.MaxLogLines
	}
	rc, err := c.ServiceLogs(ctx, id, container.LogsOptions{
		ShowStdout: true, ShowStderr: true, Tail: strconv.Itoa(tail), Timestamps: true,
	})
	if err != nil {
		return "", err
	}
	defer rc.Close()
	return readLogStream(rc, false, 8*1024*1024)
}

// Tasks 任务列表。
func (m *Manager) Tasks(ctx context.Context, env *dockerstore.Environment) ([]Task, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return nil, err
	}
	list, err := c.TaskList(ctx, swarm.TaskListOptions{})
	if err != nil {
		return nil, err
	}
	services, _ := c.ServiceList(ctx, swarm.ServiceListOptions{})
	svcNames := make(map[string]string, len(services))
	for _, s := range services {
		svcNames[s.ID] = s.Spec.Name
	}
	nodes, _ := c.NodeList(ctx, swarm.NodeListOptions{})
	nodeNames := make(map[string]string, len(nodes))
	for _, n := range nodes {
		nodeNames[n.ID] = n.Description.Hostname
	}
	out := make([]Task, 0, len(list))
	for _, t := range list {
		name := svcNames[t.ServiceID]
		if name != "" {
			name += "." + strconv.Itoa(t.Slot)
		}
		out = append(out, Task{
			ID:           t.ID,
			Name:         name,
			ServiceID:    t.ServiceID,
			ServiceName:  svcNames[t.ServiceID],
			NodeID:       t.NodeID,
			NodeName:     nodeNames[t.NodeID],
			Slot:         t.Slot,
			State:        string(t.Status.State),
			DesiredState: string(t.DesiredState),
			Message:      t.Status.Message,
			Err:          t.Status.Err,
			CreatedAt:    t.Meta.CreatedAt.Format(time.RFC3339),
			UpdatedAt:    t.Meta.UpdatedAt.Format(time.RFC3339),
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out, nil
}

// Secrets 密钥列表。
func (m *Manager) Secrets(ctx context.Context, env *dockerstore.Environment) ([]Secret, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return nil, err
	}
	list, err := c.SecretList(ctx, swarm.SecretListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]Secret, 0, len(list))
	for _, s := range list {
		driver := ""
		if s.Spec.Driver != nil {
			driver = s.Spec.Driver.Name
		}
		out = append(out, Secret{
			ID: s.ID, Name: s.Spec.Name, Labels: s.Spec.Labels, Driver: driver,
			CreatedAt: s.Meta.CreatedAt.Format(time.RFC3339), UpdatedAt: s.Meta.UpdatedAt.Format(time.RFC3339),
			StackName: s.Spec.Labels["com.docker.stack.namespace"],
		})
	}
	return out, nil
}

// CreateSecret 创建密钥。
func (m *Manager) CreateSecret(ctx context.Context, env *dockerstore.Environment, req *CreateSecretRequest) (string, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return "", err
	}
	resp, err := c.SecretCreate(ctx, swarm.SecretSpec{
		Annotations: swarm.Annotations{Name: req.Name, Labels: req.Labels},
		Data:        []byte(req.Data),
	})
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

// RemoveSecret 删除密钥。
func (m *Manager) RemoveSecret(ctx context.Context, env *dockerstore.Environment, id string) error {
	c, err := m.Client(ctx, env)
	if err != nil {
		return err
	}
	return c.SecretRemove(ctx, id)
}

// Configs 配置列表。
func (m *Manager) Configs(ctx context.Context, env *dockerstore.Environment) ([]ConfigObj, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return nil, err
	}
	list, err := c.ConfigList(ctx, swarm.ConfigListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]ConfigObj, 0, len(list))
	for _, cfg := range list {
		out = append(out, ConfigObj{
			ID: cfg.ID, Name: cfg.Spec.Name, Labels: cfg.Spec.Labels,
			CreatedAt: cfg.Meta.CreatedAt.Format(time.RFC3339), UpdatedAt: cfg.Meta.UpdatedAt.Format(time.RFC3339),
			StackName: cfg.Spec.Labels["com.docker.stack.namespace"],
		})
	}
	return out, nil
}

// CreateConfig 创建配置。
func (m *Manager) CreateConfig(ctx context.Context, env *dockerstore.Environment, req *CreateSecretRequest) (string, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return "", err
	}
	resp, err := c.ConfigCreate(ctx, swarm.ConfigSpec{
		Annotations: swarm.Annotations{Name: req.Name, Labels: req.Labels},
		Data:        []byte(req.Data),
	})
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

// RemoveConfig 删除配置。
func (m *Manager) RemoveConfig(ctx context.Context, env *dockerstore.Environment, id string) error {
	c, err := m.Client(ctx, env)
	if err != nil {
		return err
	}
	return c.ConfigRemove(ctx, id)
}

// StackServices 某 Stack 命名空间下的服务。
func (m *Manager) StackServices(ctx context.Context, env *dockerstore.Environment, namespace string) ([]swarm.Service, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return nil, err
	}
	return c.ServiceList(ctx, swarm.ServiceListOptions{
		Status:  true,
		Filters: filters.NewArgs(filters.Arg("label", "com.docker.stack.namespace="+namespace)),
	})
}

// StackRuntimeMap 一次拉取容器/服务并聚合出所有 Stack 的运行态（避免列表页 N+1 查询）。
func (m *Manager) StackRuntimeMap(ctx context.Context, env *dockerstore.Environment) (map[string]StackRuntime, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return nil, err
	}
	out := make(map[string]StackRuntime)

	containers, err := c.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, err
	}
	for _, item := range containers {
		project := item.Labels["com.docker.compose.project"]
		if project == "" {
			continue
		}
		key := "compose:" + project
		rt := out[key]
		rt.ContainersTotal++
		if strings.EqualFold(item.State, "running") {
			rt.ContainersRunning++
		}
		if len(rt.Resources) < 50 {
			rt.Resources = append(rt.Resources, RuntimeResource{
				ID: item.ID, Name: firstName(item.Names), Kind: "container",
				State: item.State, Image: item.Image, Detail: item.Status,
			})
		}
		out[key] = rt
	}

	info, err := c.Info(ctx)
	if err == nil && info.Swarm.LocalNodeState == swarm.LocalNodeStateActive && info.Swarm.ControlAvailable {
		services, err := c.ServiceList(ctx, swarm.ServiceListOptions{Status: true})
		if err == nil {
			for _, s := range services {
				ns := s.Spec.Labels["com.docker.stack.namespace"]
				if ns == "" {
					continue
				}
				key := "swarm:" + ns
				rt := out[key]
				rt.ServicesTotal++
				var running, desired uint64
				if s.ServiceStatus != nil {
					running, desired = s.ServiceStatus.RunningTasks, s.ServiceStatus.DesiredTasks
				}
				if running > 0 && running >= desired {
					rt.ServicesRunning++
				}
				if len(rt.Resources) < 50 {
					image := ""
					if s.Spec.TaskTemplate.ContainerSpec != nil {
						image = s.Spec.TaskTemplate.ContainerSpec.Image
					}
					state := "degraded"
					if running >= desired && desired > 0 {
						state = "running"
					}
					rt.Resources = append(rt.Resources, RuntimeResource{
						ID: s.ID, Name: s.Spec.Name, Kind: "service", State: state, Image: image,
						Detail: fmt.Sprintf("%d/%d 任务运行", running, desired),
					})
				}
				out[key] = rt
			}
		}
	}

	for key, rt := range out {
		rt.Status = "inactive"
		if rt.ContainersRunning > 0 || rt.ServicesRunning > 0 {
			rt.Status = "active"
		}
		out[key] = rt
	}
	return out, nil
}

// StackRuntime 单个 Stack 运行态（优先从批量结果取）。
func (m *Manager) StackRuntime(ctx context.Context, env *dockerstore.Environment, typ, name string) (StackRuntime, error) {
	all, err := m.StackRuntimeMap(ctx, env)
	if err != nil {
		return StackRuntime{}, err
	}
	rt := all[typ+":"+name]
	if rt.Status == "" {
		rt.Status = "inactive"
	}
	return rt, nil
}

func firstName(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return strings.TrimPrefix(names[0], "/")
}

func jsonUnmarshal(raw []byte, v any) error {
	return json.Unmarshal(raw, v)
}
