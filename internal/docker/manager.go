package docker

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/swarm"
	dockersystem "github.com/docker/docker/api/types/system"
	"github.com/docker/docker/client"
	"golang.org/x/crypto/ssh"

	"web-basic/internal/config"
	"web-basic/internal/dockerstore"
	"web-basic/pkg/crypto"
)

// Manager 负责按环境构建/缓存 Docker 客户端。
//
// 敏感字段加密密钥由 JWT secret 派生（SHA-256），随部署密钥变化，
// 因此迁移 SQLite 文件时必须同时迁移 JWT secret，否则需要重新录入连接凭据。
type Manager struct {
	store *dockerstore.Store
	cfg   config.Docker
	key   []byte

	mu      sync.Mutex
	clients map[string]*cachedClient
}

type cachedClient struct {
	fingerprint string
	client      *client.Client
	closeFn     func()
}

// NewManager 构建管理器。
func NewManager(store *dockerstore.Store, cfg config.Docker, secret string) *Manager {
	sum := sha256.Sum256([]byte(secret))
	return &Manager{
		store:   store,
		cfg:     cfg,
		key:     sum[:],
		clients: make(map[string]*cachedClient),
	}
}

// Store 返回底层存储。
func (m *Manager) Store() *dockerstore.Store { return m.store }

// Close 关闭全部缓存的 Docker 客户端。
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, c := range m.clients {
		if c.closeFn != nil {
			c.closeFn()
		}
		delete(m.clients, id)
	}
}

// Invalidate 环境配置变更后清空对应客户端缓存。
func (m *Manager) Invalidate(envID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.clients[envID]; ok {
		if c.closeFn != nil {
			c.closeFn()
		}
		delete(m.clients, envID)
	}
}

// Client 获取环境对应的 Docker 客户端（按配置指纹缓存）。
func (m *Manager) Client(ctx context.Context, env *dockerstore.Environment) (*client.Client, error) {
	fp, err := m.fingerprint(env)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	if c, ok := m.clients[env.ID]; ok && c.fingerprint == fp {
		m.mu.Unlock()
		return c.client, nil
	}
	m.mu.Unlock()

	c, closeFn, err := m.buildClient(env, fp)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	if old, ok := m.clients[env.ID]; ok && old.closeFn != nil {
		old.closeFn()
	}
	m.clients[env.ID] = &cachedClient{fingerprint: fp, client: c, closeFn: closeFn}
	m.mu.Unlock()
	return c, nil
}

// Test 测试连通性并返回主机信息。
func (m *Manager) Test(ctx context.Context, env *dockerstore.Environment) (*Info, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return nil, err
	}
	info, err := c.Info(ctx)
	if err != nil {
		m.Invalidate(env.ID)
		return nil, err
	}
	out := mapInfo(info)
	return &out, nil
}

// Info 主机信息。
func (m *Manager) Info(ctx context.Context, env *dockerstore.Environment) (*Info, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return nil, err
	}
	info, err := c.Info(ctx)
	if err != nil {
		return nil, err
	}
	out := mapInfo(info)
	return &out, nil
}

// Overview 概览：主机信息、资源计数、磁盘占用。
func (m *Manager) Overview(ctx context.Context, env *dockerstore.Environment) (*Overview, error) {
	c, err := m.Client(ctx, env)
	if err != nil {
		return nil, err
	}
	info, err := c.Info(ctx)
	if err != nil {
		return nil, err
	}
	out := &Overview{Info: mapInfo(info)}

	if dup, err := c.DiskUsage(ctx, diskUsageOptions()); err == nil {
		out.Usage = mapDiskUsage(dup)
	}
	if nets, err := c.NetworkList(ctx, networkListOptions()); err == nil {
		out.Networks = len(nets)
	}
	if info.Swarm.LocalNodeState == "active" && info.Swarm.ControlAvailable {
		if svcs, err := c.ServiceList(ctx, serviceListOptions()); err == nil {
			out.Services = len(svcs)
		}
		if nodes, err := c.NodeList(ctx, nodeListOptions()); err == nil {
			out.Nodes = len(nodes)
		}
		if tasks, err := c.TaskList(ctx, taskListOptions()); err == nil {
			out.Tasks = len(tasks)
		}
		if secrets, err := c.SecretList(ctx, secretListOptions()); err == nil {
			out.Secrets = len(secrets)
		}
		if cfgs, err := c.ConfigList(ctx, configListOptions()); err == nil {
			out.Configs = len(cfgs)
		}
	}
	return out, nil
}

func (m *Manager) fingerprint(env *dockerstore.Environment) (string, error) {
	creds, err := m.decrypt(env)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, _ = h.Write([]byte(strings.Join([]string{
		env.ID, env.Type, env.Host, env.SocketPath, creds.TLSCa, creds.TLSCert, creds.TLSKey,
		env.SSHUser, creds.SSHPassword, creds.SSHKey,
	}, "\x00")))
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

type credentials struct {
	TLSCa, TLSCert, TLSKey string
	SSHPassword, SSHKey    string
}

func (m *Manager) decrypt(env *dockerstore.Environment) (*credentials, error) {
	out := &credentials{}
	for _, f := range []struct {
		dst *string
		src string
	}{
		{&out.TLSCa, env.TLSCa}, {&out.TLSCert, env.TLSCert}, {&out.TLSKey, env.TLSKey},
		{&out.SSHPassword, env.SSHPassword}, {&out.SSHKey, env.SSHKey},
	} {
		if f.src == "" {
			continue
		}
		plain, err := crypto.Decrypt(f.src, m.key)
		if err != nil {
			return nil, fmt.Errorf("解密环境凭据失败（JWT secret 是否变更？）: %w", err)
		}
		*f.dst = string(plain)
	}
	return out, nil
}

func (m *Manager) buildClient(env *dockerstore.Environment, fp string) (*client.Client, func(), error) {
	creds, err := m.decrypt(env)
	if err != nil {
		return nil, nil, err
	}
	timeout := time.Duration(m.cfg.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	switch env.Type {
	case "", "local":
		host := strings.TrimSpace(env.SocketPath)
		if host == "" {
			host = strings.TrimSpace(m.cfg.Socket)
		}
		if host == "" {
			if runtime.GOOS == "windows" {
				host = "npipe:////./pipe/docker_engine"
			} else {
				host = "unix:///var/run/docker.sock"
			}
		}
		c, err := client.NewClientWithOpts(client.WithHost(host), client.WithAPIVersionNegotiation())
		if err != nil {
			return nil, nil, err
		}
		return c, func() { c.Close() }, nil

	case "tcp":
		host := normalizeTCPHost(env.Host, false)
		c, err := client.NewClientWithOpts(client.WithHost(host), client.WithAPIVersionNegotiation())
		if err != nil {
			return nil, nil, err
		}
		return c, func() { c.Close() }, nil

	case "tls":
		host := normalizeTCPHost(env.Host, true)
		tlsCfg, err := buildTLSConfig(env, creds)
		if err != nil {
			return nil, nil, err
		}
		transport := &http.Transport{
			Proxy:           http.ProxyFromEnvironment,
			TLSClientConfig: tlsCfg,
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2: false,
		}
		httpClient := &http.Client{Transport: transport, Timeout: timeout}
		c, err := client.NewClientWithOpts(
			client.WithHost(host),
			client.WithHTTPClient(httpClient),
			client.WithAPIVersionNegotiation(),
		)
		if err != nil {
			return nil, nil, err
		}
		return c, func() { c.Close() }, nil

	case "ssh":
		sshClient, err := dialSSH(env.Host, env.SSHUser, creds.SSHPassword, creds.SSHKey, timeout)
		if err != nil {
			return nil, nil, err
		}
		socket := strings.TrimSpace(env.SocketPath)
		if socket == "" {
			socket = "/var/run/docker.sock"
		}
		dialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
			type result struct {
				conn net.Conn
				err  error
			}
			ch := make(chan result, 1)
			go func() {
				conn, err := sshClient.Dial("unix", socket)
				ch <- result{conn: conn, err: err}
			}()
			select {
			case r := <-ch:
				return r.conn, r.err
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		c, err := client.NewClientWithOpts(
			client.WithHost("http://docker"),
			client.WithDialContext(dialer),
			client.WithAPIVersionNegotiation(),
		)
		if err != nil {
			sshClient.Close()
			return nil, nil, err
		}
		return c, func() {
			c.Close()
			_ = sshClient.Close()
		}, nil

	default:
		return nil, nil, fmt.Errorf("不支持的 Docker 连接类型：%s", env.Type)
	}
}

func normalizeTCPHost(host string, tlsPort bool) string {
	host = strings.TrimSpace(host)
	if host == "" {
		host = "127.0.0.1"
	}
	if !strings.Contains(host, "://") {
		host = "tcp://" + host
	}
	if tlsPort {
		// tls 模式默认 2376 端口；未显式写端口时补上。
		withoutScheme := strings.TrimPrefix(strings.TrimPrefix(host, "tcp://"), "https://")
		if _, _, err := net.SplitHostPort(withoutScheme); err != nil {
			host = "tcp://" + net.JoinHostPort(withoutScheme, "2376")
		}
	}
	return host
}

func buildTLSConfig(env *dockerstore.Environment, creds *credentials) (*tls.Config, error) {
	if creds.TLSCert == "" || creds.TLSKey == "" {
		return nil, errors.New("TLS 模式需要同时配置客户端证书与私钥")
	}
	cert, err := tls.X509KeyPair([]byte(creds.TLSCert), []byte(creds.TLSKey))
	if err != nil {
		return nil, fmt.Errorf("解析客户端证书失败: %w", err)
	}
	cfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
	if creds.TLSCa != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(creds.TLSCa)) {
			return nil, errors.New("CA 证书格式不正确")
		}
		cfg.RootCAs = pool
	}
	host := strings.TrimSpace(env.Host)
	host = strings.TrimPrefix(strings.TrimPrefix(host, "tcp://"), "https://")
	if h, _, err := net.SplitHostPort(host); err == nil {
		cfg.ServerName = h
	} else {
		cfg.ServerName = host
	}
	return cfg, nil
}

func dialSSH(addr, user, password, privateKey string, timeout time.Duration) (*ssh.Client, error) {
	if strings.TrimSpace(addr) == "" {
		return nil, errors.New("SSH 地址不能为空")
	}
	if !strings.Contains(addr, ":") {
		addr = net.JoinHostPort(addr, "22")
	}
	var auths []ssh.AuthMethod
	if password != "" {
		auths = append(auths, ssh.Password(password))
	}
	if privateKey != "" {
		signer, err := ssh.ParsePrivateKey([]byte(privateKey))
		if err != nil {
			return nil, fmt.Errorf("解析 SSH 私钥失败: %w", err)
		}
		auths = append(auths, ssh.PublicKeys(signer))
	}
	if len(auths) == 0 {
		return nil, errors.New("SSH 模式需要配置密码或私钥")
	}
	if user == "" {
		user = "root"
	}
	cfg := &ssh.ClientConfig{
		User: user,
		Auth: auths,
		// Docker 主机通常使用内网地址，这里不做 known_hosts 校验；
		// 若需要严格校验，可在后续版本增加指纹配置项。
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}
	netConn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, fmt.Errorf("连接 SSH 主机失败: %w", err)
	}
	conn, chans, reqs, err := ssh.NewClientConn(netConn, addr, cfg)
	if err != nil {
		netConn.Close()
		return nil, fmt.Errorf("SSH 认证失败: %w", err)
	}
	return ssh.NewClient(conn, chans, reqs), nil
}

func mapInfo(info dockersystem.Info) Info {
	out := Info{
		Name:            info.Name,
		ServerVersion:   info.ServerVersion,
		OperatingSystem: info.OperatingSystem,
		OSType:          info.OSType,
		Architecture:    info.Architecture,
		KernelVersion:   info.KernelVersion,
		NCPU:            info.NCPU,
		MemTotal:        info.MemTotal,
		Containers:      info.Containers,
		Running:         info.ContainersRunning,
		Paused:          info.ContainersPaused,
		Stopped:         info.ContainersStopped,
		Images:          info.Images,
		Driver:          info.Driver,
		DockerRootDir:   info.DockerRootDir,
		LoggingDriver:   info.LoggingDriver,
		CgroupDriver:    info.CgroupDriver,
		SwarmActive:     info.Swarm.LocalNodeState == swarm.LocalNodeStateActive,
		SwarmNodeID:     info.Swarm.NodeID,
		SwarmNodeAddr:   info.Swarm.NodeAddr,
		SwarmNodes:      info.Swarm.Nodes,
		SwarmManagers:   info.Swarm.Managers,
		Warnings:        info.Warnings,
	}
	if out.SwarmActive {
		if info.Swarm.ControlAvailable {
			out.SwarmRole = "manager"
		} else {
			out.SwarmRole = "worker"
		}
	}
	return out
}

func mapDiskUsage(u dockertypes.DiskUsage) DiskUsage {
	out := DiskUsage{
		LayersSize: u.LayersSize,
		Images:     len(u.Images),
		Containers: len(u.Containers),
		Volumes:    len(u.Volumes),
	}
	for _, img := range u.Images {
		if img != nil {
			out.ImagesSize += img.Size
		}
	}
	for _, c := range u.BuildCache {
		if c != nil {
			out.BuildCache += c.Size
		}
	}
	return out
}

func diskUsageOptions() dockertypes.DiskUsageOptions { return dockertypes.DiskUsageOptions{} }
func networkListOptions() network.ListOptions        { return network.ListOptions{} }
func serviceListOptions() swarm.ServiceListOptions   { return swarm.ServiceListOptions{} }
func nodeListOptions() swarm.NodeListOptions         { return swarm.NodeListOptions{} }
func taskListOptions() swarm.TaskListOptions         { return swarm.TaskListOptions{} }
func secretListOptions() swarm.SecretListOptions     { return swarm.SecretListOptions{} }
func configListOptions() swarm.ConfigListOptions     { return swarm.ConfigListOptions{} }

// 以下变量仅用于确保导入被使用（具体资源操作实现分布在同包其它文件）。
var (
	_ = container.ListOptions{}
	_ = filters.Args{}
)
