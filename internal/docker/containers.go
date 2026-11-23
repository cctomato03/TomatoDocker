package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"

	"web-basic/internal/dockerstore"
)

// Containers 容器列表。
func (m *Manager) Containers(ctx context.Context, env *dockerstore.Environment, all bool, keyword, state string) ([]Container, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return nil, err
	}
	list, err := c.ContainerList(ctx, container.ListOptions{All: all})
	if err != nil {
		return nil, err
	}
	out := make([]Container, 0, len(list))
	kw := strings.ToLower(strings.TrimSpace(keyword))
	state = strings.ToLower(strings.TrimSpace(state))
	for _, item := range list {
		name := ""
		if len(item.Names) > 0 {
			name = strings.TrimPrefix(item.Names[0], "/")
		}
		if kw != "" {
			hit := strings.Contains(strings.ToLower(name), kw) ||
				strings.Contains(strings.ToLower(item.Image), kw) ||
				strings.Contains(item.ID, kw)
			if !hit {
				for k, v := range item.Labels {
					if strings.Contains(strings.ToLower(k+"="+v), kw) {
						hit = true
						break
					}
				}
			}
			if !hit {
				continue
			}
		}
		if state != "" && state != "all" && !strings.EqualFold(item.State, state) {
			continue
		}
		ports := make([]Port, 0, len(item.Ports))
		for _, p := range item.Ports {
			ports = append(ports, Port{IP: p.IP, PrivatePort: p.PrivatePort, PublicPort: p.PublicPort, Type: p.Type})
		}
		health := ""
		if strings.Contains(item.Status, "(healthy)") {
			health = "healthy"
		} else if strings.Contains(item.Status, "(unhealthy)") {
			health = "unhealthy"
		} else if strings.Contains(item.Status, "(health: starting)") {
			health = "starting"
		}
		out = append(out, Container{
			ID:           item.ID,
			Name:         name,
			Names:        item.Names,
			Image:        item.Image,
			ImageID:      item.ImageID,
			Command:      item.Command,
			Created:      item.Created,
			State:        item.State,
			Status:       item.Status,
			Ports:        ports,
			Labels:       item.Labels,
			StackName:    item.Labels["com.docker.stack.namespace"],
			ServiceName:  item.Labels["com.docker.swarm.service.name"],
			ProjectName:  item.Labels["com.docker.compose.project"],
			Health:       health,
			ComposeMode:  item.Labels["com.docker.compose.project"] != "",
			SwarmService: item.Labels["com.docker.swarm.service.id"] != "",
		})
	}
	return out, nil
}

// ContainerInspect 容器详情（直接返回 Docker inspect，前端以 JSON 展示关键字段）。
func (m *Manager) ContainerInspect(ctx context.Context, env *dockerstore.Environment, id string) (container.InspectResponse, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return container.InspectResponse{}, err
	}
	return c.ContainerInspect(ctx, id)
}

// ContainerAction 容器生命周期操作。
func (m *Manager) ContainerAction(ctx context.Context, env *dockerstore.Environment, id, action, newName string) error {
	c, err := m.Client(ctx, env)
	if err != nil {
		return err
	}
	switch action {
	case "start":
		return c.ContainerStart(ctx, id, container.StartOptions{})
	case "stop":
		timeout := 10
		return c.ContainerStop(ctx, id, container.StopOptions{Timeout: &timeout})
	case "restart":
		timeout := 10
		return c.ContainerRestart(ctx, id, container.StopOptions{Timeout: &timeout})
	case "pause":
		return c.ContainerPause(ctx, id)
	case "unpause":
		return c.ContainerUnpause(ctx, id)
	case "kill":
		return c.ContainerKill(ctx, id, "SIGKILL")
	case "rename":
		if strings.TrimSpace(newName) == "" {
			return fmt.Errorf("新容器名称不能为空")
		}
		return c.ContainerRename(ctx, id, newName)
	default:
		return fmt.Errorf("不支持的容器操作：%s", action)
	}
}

// ContainerRemove 删除容器。
func (m *Manager) ContainerRemove(ctx context.Context, env *dockerstore.Environment, id string, force, volumes bool) error {
	c, err := m.Client(ctx, env)
	if err != nil {
		return err
	}
	return c.ContainerRemove(ctx, id, container.RemoveOptions{Force: force, RemoveVolumes: volumes})
}

// ContainersPrune 清理已停止容器。
func (m *Manager) ContainersPrune(ctx context.Context, env *dockerstore.Environment) (uint64, int64, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return 0, 0, err
	}
	report, err := c.ContainersPrune(ctx, filters.NewArgs())
	if err != nil {
		return 0, 0, err
	}
	return uint64(len(report.ContainersDeleted)), int64(report.SpaceReclaimed), nil
}

// ContainerLogs 拉取容器日志（非流式，返回最后 N 行）。
func (m *Manager) ContainerLogs(ctx context.Context, env *dockerstore.Environment, id string, tail int, timestamps bool) (string, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return "", err
	}
	insp, err := c.ContainerInspect(ctx, id)
	if err != nil {
		return "", err
	}
	if tail <= 0 {
		tail = 200
	}
	if m.cfg.MaxLogLines > 0 && tail > m.cfg.MaxLogLines {
		tail = m.cfg.MaxLogLines
	}
	rc, err := c.ContainerLogs(ctx, id, container.LogsOptions{
		ShowStdout: true, ShowStderr: true, Tail: strconv.Itoa(tail), Timestamps: timestamps,
	})
	if err != nil {
		return "", err
	}
	defer rc.Close()
	return readLogStream(rc, insp.Config != nil && insp.Config.Tty, 8*1024*1024)
}

// ContainerStats 容器资源快照。
func (m *Manager) ContainerStats(ctx context.Context, env *dockerstore.Environment, id string) (*ContainerStats, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return nil, err
	}
	resp, err := c.ContainerStatsOneShot(ctx, id)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var raw container.StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("解析容器资源统计失败: %w", err)
	}
	out := &ContainerStats{
		MemUsage: int64(raw.MemoryStats.Usage),
		MemLimit: int64(raw.MemoryStats.Limit),
		Pids:     int64(raw.PidsStats.Current),
	}
	if out.MemLimit > 0 {
		out.MemPercent = float64(out.MemUsage) / float64(out.MemLimit) * 100
	}
	cpuDelta := float64(raw.CPUStats.CPUUsage.TotalUsage) - float64(raw.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(raw.CPUStats.SystemUsage) - float64(raw.PreCPUStats.SystemUsage)
	online := raw.CPUStats.OnlineCPUs
	if online == 0 {
		online = uint32(len(raw.CPUStats.CPUUsage.PercpuUsage))
	}
	if sysDelta > 0 && cpuDelta > 0 && online > 0 {
		out.CPUPercent = cpuDelta / sysDelta * float64(online) * 100
	}
	for _, n := range raw.Networks {
		out.NetRx += int64(n.RxBytes)
		out.NetTx += int64(n.TxBytes)
	}
	for _, b := range raw.BlkioStats.IoServiceBytesRecursive {
		switch strings.ToLower(b.Op) {
		case "read":
			out.BlockRead += int64(b.Value)
		case "write":
			out.BlockWrite += int64(b.Value)
		}
	}
	return out, nil
}

// ContainerExec 在容器内执行命令（非交互，返回 stdout+stderr 与退出码）。
func (m *Manager) ContainerExec(ctx context.Context, env *dockerstore.Environment, id, command string, timeout time.Duration) (string, int, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return "", -1, err
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	exec, err := c.ContainerExecCreate(execCtx, id, container.ExecOptions{
		Cmd:          []string{"/bin/sh", "-c", command},
		AttachStdout: true,
		AttachStderr: true,
		Tty:          false,
	})
	if err != nil {
		return "", -1, err
	}
	resp, err := c.ContainerExecAttach(execCtx, exec.ID, container.ExecAttachOptions{Tty: false})
	if err != nil {
		return "", -1, err
	}
	defer resp.Close()
	var stdout, stderr bytes.Buffer
	_, _ = stdcopy.StdCopy(&stdout, &stderr, io.LimitReader(resp.Reader, 1024*1024))
	buf := stdout
	buf.Write(stderr.Bytes())
	inspect, err := c.ContainerExecInspect(execCtx, exec.ID)
	if err != nil {
		return buf.String(), -1, err
	}
	return buf.String(), inspect.ExitCode, nil
}

// CreateContainer 创建并可选启动容器。
func (m *Manager) CreateContainer(ctx context.Context, env *dockerstore.Environment, req *CreateContainerRequest) (string, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return "", err
	}
	cfg := &container.Config{
		Image:        req.Image,
		Env:          req.Env,
		Labels:       req.Labels,
		Tty:          req.Tty,
		OpenStdin:    req.Tty,
		ExposedPorts: nat.PortSet{},
	}
	if cmd := splitCommand(req.Command); len(cmd) > 0 {
		cfg.Cmd = cmd
	}
	if ep := splitCommand(req.Entrypoint); len(ep) > 0 {
		cfg.Entrypoint = ep
	}
	if cfg.Labels == nil {
		cfg.Labels = map[string]string{}
	}

	hostCfg := &container.HostConfig{
		Privileged:    req.Privileged,
		RestartPolicy: parseRestartPolicy(req.RestartPolicy),
		Mounts:        []mount.Mount{},
		PortBindings:  nat.PortMap{},
	}
	for _, p := range req.Ports {
		if p.ContainerPort <= 0 {
			continue
		}
		proto := strings.ToLower(strings.TrimSpace(p.Protocol))
		if proto == "" {
			proto = "tcp"
		}
		port, err := nat.NewPort(proto, strconv.Itoa(p.ContainerPort))
		if err != nil {
			return "", fmt.Errorf("非法端口 %d/%s: %w", p.ContainerPort, proto, err)
		}
		cfg.ExposedPorts[port] = struct{}{}
		hostPort := ""
		if p.HostPort > 0 {
			hostPort = strconv.Itoa(p.HostPort)
		}
		hostCfg.PortBindings[port] = append(hostCfg.PortBindings[port], nat.PortBinding{HostIP: p.HostIP, HostPort: hostPort})
	}
	for _, v := range req.Volumes {
		if strings.TrimSpace(v.Source) == "" || strings.TrimSpace(v.Target) == "" {
			continue
		}
		typ := mount.TypeVolume
		if strings.HasPrefix(v.Source, "/") || strings.HasPrefix(v.Source, ".") ||
			(len(v.Source) > 2 && v.Source[1] == ':') {
			typ = mount.TypeBind
		}
		hostCfg.Mounts = append(hostCfg.Mounts, mount.Mount{
			Type: typ, Source: v.Source, Target: v.Target, ReadOnly: v.ReadOnly,
		})
	}

	var networking *network.NetworkingConfig
	if req.NetworkID != "" {
		n, err := c.NetworkInspect(ctx, req.NetworkID, network.InspectOptions{})
		if err != nil {
			return "", fmt.Errorf("查询网络失败: %w", err)
		}
		hostCfg.NetworkMode = container.NetworkMode(n.Name)
		networking = &network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{n.Name: {Aliases: []string{req.Name}}},
		}
	}

	created, err := c.ContainerCreate(ctx, cfg, hostCfg, networking, nil, req.Name)
	if err != nil {
		return "", err
	}
	if req.AutoStart {
		if err := c.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
			return created.ID, err
		}
	}
	return created.ID, nil
}

func parseRestartPolicy(v string) container.RestartPolicy {
	v = strings.TrimSpace(strings.ToLower(v))
	switch {
	case v == "", v == "no", v == "none":
		return container.RestartPolicy{Name: container.RestartPolicyDisabled}
	case v == "always":
		return container.RestartPolicy{Name: container.RestartPolicyAlways}
	case v == "unless-stopped":
		return container.RestartPolicy{Name: container.RestartPolicyUnlessStopped}
	case strings.HasPrefix(v, "on-failure"):
		rp := container.RestartPolicy{Name: container.RestartPolicyOnFailure}
		if _, n, ok := strings.Cut(v, ":"); ok {
			if max, err := strconv.Atoi(n); err == nil {
				rp.MaximumRetryCount = max
			}
		}
		return rp
	default:
		return container.RestartPolicy{Name: container.RestartPolicyDisabled}
	}
}

func readLogStream(r io.Reader, tty bool, limit int64) (string, error) {
	var buf bytes.Buffer
	if tty {
		_, _ = io.Copy(&buf, io.LimitReader(r, limit))
		return buf.String(), nil
	}
	var stdout, stderr bytes.Buffer
	_, err := stdcopy.StdCopy(&stdout, &stderr, io.LimitReader(r, limit))
	if err != nil && stdout.Len() == 0 && stderr.Len() == 0 {
		return "", err
	}
	return stdout.String() + stderr.String(), nil
}

// splitCommand 解析简单命令行（支持单/双引号，避免把参数里的空格切开）。
func splitCommand(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var out []string
	var cur strings.Builder
	var quote rune
	escaped := false
	for _, r := range s {
		if escaped {
			cur.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case ' ', '\t', '\n':
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}
