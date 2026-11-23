package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"web-basic/internal/docker"
	"web-basic/internal/dockerstore"
	"web-basic/internal/model/entity"
	"web-basic/internal/principal"
	"web-basic/pkg/crypto"
	"web-basic/pkg/errs"
	"web-basic/pkg/id"
)

// Docker 环境访问级别。
const (
	DockerAccessRead    = 1
	DockerAccessOperate = 2
	DockerAccessManage  = 3
)

var dockerNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// DockerEnvironmentView 环境出参（不含任何明文凭据）。
type DockerEnvironmentView struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	Type           string `json:"type"`
	Host           string `json:"host"`
	SocketPath     string `json:"socketPath"`
	SSHUser        string `json:"sshUser"`
	IsDefault      bool   `json:"isDefault"`
	Status         string `json:"status"`
	StatusMessage  string `json:"statusMessage"`
	AccessLevel    int    `json:"accessLevel"`
	HasTLS         bool   `json:"hasTls"`
	HasSSHPassword bool   `json:"hasSshPassword"`
	HasSSHKey      bool   `json:"hasSshKey"`
	SwarmActive    bool   `json:"swarmActive"`
	SwarmRole      string `json:"swarmRole"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

// DockerEnvironmentReq 新增/修改环境入参。
type DockerEnvironmentReq struct {
	Name             string `json:"name" binding:"required,max=64"`
	Description      string `json:"description" binding:"max=500"`
	Type             string `json:"type" binding:"required,oneof=local tcp tls ssh"`
	Host             string `json:"host" binding:"max=255"`
	SocketPath       string `json:"socketPath" binding:"max=255"`
	TLSCa            string `json:"tlsCa"`
	TLSCert          string `json:"tlsCert"`
	TLSKey           string `json:"tlsKey"`
	SSHUser          string `json:"sshUser" binding:"max=64"`
	SSHPassword      string `json:"sshPassword"`
	SSHKey           string `json:"sshKey"`
	IsDefault        bool   `json:"isDefault"`
	ClearCredentials bool   `json:"clearCredentials"`
}

// DockerGrantOption 资源授权可选项。
type DockerGrantOption struct {
	SubjectType string `json:"subjectType"`
	SubjectID   string `json:"subjectId"`
	Name        string `json:"name"`
	Detail      string `json:"detail"`
}

// DockerGrantView 已授权项（带名称）。
type DockerGrantView struct {
	ID          string `json:"id"`
	SubjectType string `json:"subjectType"`
	SubjectID   string `json:"subjectId"`
	Name        string `json:"name"`
	AccessLevel int    `json:"accessLevel"`
	CreatedAt   string `json:"createdAt"`
}

// DockerGrantReq 授权保存入参。
type DockerGrantReq struct {
	Grants []DockerGrantItem `json:"grants"`
}

// DockerGrantItem 单条授权。
type DockerGrantItem struct {
	SubjectType string `json:"subjectType" binding:"required,oneof=role user"`
	SubjectID   string `json:"subjectId" binding:"required"`
	AccessLevel int    `json:"accessLevel" binding:"required,oneof=1 2 3"`
}

// DockerStackView Stack 列表/详情出参。
type DockerStackView struct {
	ID              string              `json:"id"`
	EnvironmentID   string              `json:"environmentId"`
	EnvironmentName string              `json:"environmentName"`
	Name            string              `json:"name"`
	Type            string              `json:"type"`
	Status          string              `json:"status"`
	StatusMessage   string              `json:"statusMessage"`
	Version         int                 `json:"version"`
	LastDeployedAt  *string             `json:"lastDeployedAt"`
	CreatedBy       string              `json:"createdBy"`
	UpdatedBy       string              `json:"updatedBy"`
	CreatedAt       string              `json:"createdAt"`
	UpdatedAt       string              `json:"updatedAt"`
	AccessLevel     int                 `json:"accessLevel"`
	PendingChanges  bool                `json:"pendingChanges"`
	Runtime         docker.StackRuntime `json:"runtime"`
	RuntimeError    string              `json:"runtimeError,omitempty"`
}

// DockerStackDetail Stack 详情（含 compose 内容、历史与操作记录）。
type DockerStackDetail struct {
	DockerStackView
	ComposeContent string                 `json:"composeContent"`
	EnvContent     string                 `json:"envContent"`
	Revisions      []dockerstore.Revision `json:"revisions"`
	Activities     []dockerstore.Activity `json:"activities"`
}

// DockerStackReq 新建 Stack。
type DockerStackReq struct {
	EnvironmentID  string `json:"environmentId" binding:"required"`
	Name           string `json:"name" binding:"required,max=64"`
	Type           string `json:"type" binding:"required,oneof=compose swarm"`
	ComposeContent string `json:"composeContent" binding:"required"`
	EnvContent     string `json:"envContent"`
}

// DockerStackUpdateReq 保存 Stack 内容。
type DockerStackUpdateReq struct {
	Type           string `json:"type" binding:"required,oneof=compose swarm"`
	ComposeContent string `json:"composeContent" binding:"required"`
	EnvContent     string `json:"envContent"`
}

// DockerStackExport Stack 导出结构（用于整库恢复）。
type DockerStackExport struct {
	EnvironmentName string `json:"environmentName"`
	Name            string `json:"name"`
	Type            string `json:"type"`
	ComposeContent  string `json:"composeContent"`
	EnvContent      string `json:"envContent"`
}

// DockerStackImportReq 导入 Stack。
type DockerStackImportReq struct {
	Overwrite bool                `json:"overwrite"`
	Stacks    []DockerStackExport `json:"stacks" binding:"required"`
}

// DockerOverviewView 环境概览。
type DockerOverviewView struct {
	docker.Overview
	StackList []DockerStackView `json:"stackList"`
}

func (s *Service) dockerReady() error {
	if s.Docker == nil || s.DockerStore == nil {
		return errs.New(errs.ErrUnavailable.Code, "Docker 管理模块未启用，请检查 docker.enabled 配置")
	}
	return nil
}

func dockerErr(err error) error {
	if err == nil {
		return nil
	}
	var ae *errs.AppError
	if errors.As(err, &ae) {
		return err
	}
	msg := strings.TrimSpace(err.Error())
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "not found"), strings.Contains(lower, "no such"):
		return errs.ErrNotFound.WithErr(err)
	case strings.Contains(lower, "already exists"), strings.Contains(lower, "conflict"):
		return errs.ErrDuplicate.WithErr(err)
	case strings.Contains(lower, "permission denied"), strings.Contains(lower, "access denied"):
		return errs.New(errs.ErrForbidden.Code, "Docker 拒绝了本次操作："+truncateText(msg, 300))
	default:
		return errs.New(50005, "Docker 操作失败："+truncateText(msg, 500))
	}
}

func truncateText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func (s *Service) encSecret(v string) (string, error) {
	if v == "" {
		return "", nil
	}
	key := sha256.Sum256([]byte(s.Cfg.JWT.Secret))
	out, err := crypto.Encrypt([]byte(v), key[:])
	if err != nil {
		return "", fmt.Errorf("加密凭据失败: %w", err)
	}
	return out, nil
}

func (s *Service) envRoleIDs(p *principal.Info) []string {
	out := make([]string, 0, len(p.RoleIDs))
	for _, rid := range p.RoleIDs {
		out = append(out, fmt.Sprintf("%d", rid))
	}
	return out
}

// dockerEnvLevel 计算当前用户对环境的访问级别；0 表示无权限。
func (s *Service) dockerEnvLevel(ctx context.Context, p *principal.Info, envID string) (int, error) {
	if p.IsSuper {
		return DockerAccessManage, nil
	}
	levels, err := s.DockerStore.AccessibleEnvironmentLevels(ctx, fmt.Sprintf("%d", p.UserID), s.envRoleIDs(p))
	if err != nil {
		return 0, errs.ErrDB.WithErr(err)
	}
	return levels[envID], nil
}

// requireDockerEnv 校验环境访问级别并返回环境。
func (s *Service) requireDockerEnv(ctx context.Context, p *principal.Info, envID string, need int) (*dockerstore.Environment, error) {
	if err := s.dockerReady(); err != nil {
		return nil, err
	}
	env, err := s.DockerStore.Environment(ctx, envID)
	if err != nil {
		return nil, errs.ErrDB.WithErr(err)
	}
	if env == nil {
		return nil, errs.ErrNotFound
	}
	level, err := s.dockerEnvLevel(ctx, p, envID)
	if err != nil {
		return nil, err
	}
	if level < need {
		if level == 0 {
			return nil, errs.New(errs.ErrForbidden.Code, "没有该 Docker 环境的访问权限")
		}
		return nil, errs.New(errs.ErrForbidden.Code, "当前环境权限不足（需要更高级别的资源授权）")
	}
	return env, nil
}

func (s *Service) toEnvView(env *dockerstore.Environment, level int, info *docker.Info) DockerEnvironmentView {
	v := DockerEnvironmentView{
		ID: env.ID, Name: env.Name, Description: env.Description, Type: env.Type,
		Host: env.Host, SocketPath: env.SocketPath, SSHUser: env.SSHUser,
		IsDefault: env.IsDefault, Status: env.Status, StatusMessage: env.StatusMessage,
		AccessLevel: level, HasTLS: env.HasTLS(), HasSSHPassword: env.SSHPassword != "", HasSSHKey: env.SSHKey != "",
		CreatedAt: env.CreatedAt, UpdatedAt: env.UpdatedAt,
	}
	if info != nil {
		v.SwarmActive = info.SwarmActive
		v.SwarmRole = info.SwarmRole
	}
	return v
}

// ListDockerEnvironments 当前用户可见的 Docker 环境（并行探测连接状态）。
func (s *Service) ListDockerEnvironments(ctx context.Context, p *principal.Info) ([]DockerEnvironmentView, error) {
	if err := s.dockerReady(); err != nil {
		return nil, err
	}
	all, err := s.DockerStore.Environments(ctx)
	if err != nil {
		return nil, errs.ErrDB.WithErr(err)
	}
	levels := map[string]int{}
	if p.IsSuper {
		for i := range all {
			levels[all[i].ID] = DockerAccessManage
		}
	} else {
		levels, err = s.DockerStore.AccessibleEnvironmentLevels(ctx, fmt.Sprintf("%d", p.UserID), s.envRoleIDs(p))
		if err != nil {
			return nil, errs.ErrDB.WithErr(err)
		}
	}

	visible := make([]*dockerstore.Environment, 0, len(all))
	infos := make([]*docker.Info, 0, len(all))
	for i := range all {
		if levels[all[i].ID] == 0 {
			continue
		}
		visible = append(visible, &all[i])
		infos = append(infos, nil)
	}

	// 并行探测（最多 4 个并发），单次 3 秒超时；探测失败不影响列表返回。
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	var mu sync.Mutex
	for i, env := range visible {
		wg.Add(1)
		go func(idx int, e *dockerstore.Environment) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			info, err := s.Docker.Test(probeCtx, e)
			if err != nil {
				msg := truncateText(err.Error(), 500)
				_ = s.DockerStore.SetEnvironmentStatus(probeCtx, e.ID, "offline", msg)
				mu.Lock()
				if idx < len(visible) {
					visible[idx].Status = "offline"
					visible[idx].StatusMessage = msg
				}
				mu.Unlock()
				return
			}
			_ = s.DockerStore.SetEnvironmentStatus(probeCtx, e.ID, "online", "")
			mu.Lock()
			infos[idx] = info
			visible[idx].Status = "online"
			visible[idx].StatusMessage = ""
			mu.Unlock()
		}(i, env)
	}
	wg.Wait()

	out := make([]DockerEnvironmentView, 0, len(visible))
	for i, env := range visible {
		out = append(out, s.toEnvView(env, levels[env.ID], infos[i]))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].IsDefault != out[j].IsDefault {
			return out[i].IsDefault
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// GetDockerEnvironment 环境详情。
func (s *Service) GetDockerEnvironment(ctx context.Context, p *principal.Info, envID string) (*DockerEnvironmentView, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessRead)
	if err != nil {
		return nil, err
	}
	level, _ := s.dockerEnvLevel(ctx, p, envID)
	var info *docker.Info
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if got, err := s.Docker.Test(probeCtx, env); err == nil {
		info = got
	}
	v := s.toEnvView(env, level, info)
	return &v, nil
}

// CreateDockerEnvironment 新增环境（创建者自动获得管理权限）。
func (s *Service) CreateDockerEnvironment(ctx context.Context, p *principal.Info, req *DockerEnvironmentReq) (string, error) {
	if err := s.dockerReady(); err != nil {
		return "", err
	}
	req.Name = strings.TrimSpace(req.Name)
	if err := validateDockerEnvReq(req, nil); err != nil {
		return "", err
	}
	all, err := s.DockerStore.Environments(ctx)
	if err != nil {
		return "", errs.ErrDB.WithErr(err)
	}
	for _, e := range all {
		if strings.EqualFold(e.Name, req.Name) {
			return "", errs.ErrDuplicate
		}
	}
	env := &dockerstore.Environment{
		ID: fmt.Sprintf("%d", id.Next()), Name: req.Name, Description: req.Description,
		Type: req.Type, Host: strings.TrimSpace(req.Host), SocketPath: strings.TrimSpace(req.SocketPath),
		SSHUser: strings.TrimSpace(req.SSHUser), CreatedBy: fmt.Sprintf("%d", p.UserID),
		IsDefault: req.IsDefault || len(all) == 0,
	}
	if env.TLSCa, err = s.encSecret(req.TLSCa); err != nil {
		return "", errs.ErrInternal.WithErr(err)
	}
	if env.TLSCert, err = s.encSecret(req.TLSCert); err != nil {
		return "", errs.ErrInternal.WithErr(err)
	}
	if env.TLSKey, err = s.encSecret(req.TLSKey); err != nil {
		return "", errs.ErrInternal.WithErr(err)
	}
	if env.SSHPassword, err = s.encSecret(req.SSHPassword); err != nil {
		return "", errs.ErrInternal.WithErr(err)
	}
	if env.SSHKey, err = s.encSecret(req.SSHKey); err != nil {
		return "", errs.ErrInternal.WithErr(err)
	}
	if err := s.DockerStore.CreateEnvironment(ctx, env); err != nil {
		return "", errs.ErrDB.WithErr(err)
	}
	// 非超级管理员：自动把自己授权为 manage，避免创建后看不到环境。
	if !p.IsSuper {
		_ = s.DockerStore.SetGrants(ctx, env.ID, []dockerstore.Grant{{
			SubjectType: "user", SubjectID: fmt.Sprintf("%d", p.UserID), AccessLevel: DockerAccessManage,
		}}, fmt.Sprintf("%d", p.UserID))
	}
	return env.ID, nil
}

// UpdateDockerEnvironment 修改环境。
func (s *Service) UpdateDockerEnvironment(ctx context.Context, p *principal.Info, envID string, req *DockerEnvironmentReq) error {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessManage)
	if err != nil {
		return err
	}
	req.Name = strings.TrimSpace(req.Name)
	if err := validateDockerEnvReq(req, env); err != nil {
		return err
	}
	all, err := s.DockerStore.Environments(ctx)
	if err != nil {
		return errs.ErrDB.WithErr(err)
	}
	for _, e := range all {
		if e.ID != envID && strings.EqualFold(e.Name, req.Name) {
			return errs.ErrDuplicate
		}
	}
	env.Name = req.Name
	env.Description = req.Description
	env.Type = req.Type
	env.Host = strings.TrimSpace(req.Host)
	env.SocketPath = strings.TrimSpace(req.SocketPath)
	env.SSHUser = strings.TrimSpace(req.SSHUser)
	env.IsDefault = req.IsDefault || env.IsDefault
	if req.ClearCredentials {
		env.TLSCa, env.TLSCert, env.TLSKey = "", "", ""
		env.SSHPassword, env.SSHKey = "", ""
	} else {
		// 空字符串表示"保持原值"，避免前端编辑环境时把凭据清空。
		if req.TLSCa != "" {
			if env.TLSCa, err = s.encSecret(req.TLSCa); err != nil {
				return errs.ErrInternal.WithErr(err)
			}
		}
		if req.TLSCert != "" {
			if env.TLSCert, err = s.encSecret(req.TLSCert); err != nil {
				return errs.ErrInternal.WithErr(err)
			}
		}
		if req.TLSKey != "" {
			if env.TLSKey, err = s.encSecret(req.TLSKey); err != nil {
				return errs.ErrInternal.WithErr(err)
			}
		}
		if req.SSHPassword != "" {
			if env.SSHPassword, err = s.encSecret(req.SSHPassword); err != nil {
				return errs.ErrInternal.WithErr(err)
			}
		}
		if req.SSHKey != "" {
			if env.SSHKey, err = s.encSecret(req.SSHKey); err != nil {
				return errs.ErrInternal.WithErr(err)
			}
		}
	}
	if err := s.DockerStore.UpdateEnvironment(ctx, env); err != nil {
		return errs.ErrDB.WithErr(err)
	}
	s.Docker.Invalidate(envID)
	return nil
}

func validateDockerEnvReq(req *DockerEnvironmentReq, existing *dockerstore.Environment) error {
	switch req.Type {
	case "local":
	case "tcp":
		if strings.TrimSpace(req.Host) == "" {
			return errs.Invalid("TCP 模式必须填写主机地址，例如 192.168.1.10:2375")
		}
	case "tls":
		if strings.TrimSpace(req.Host) == "" {
			return errs.Invalid("TLS 模式必须填写主机地址，例如 192.168.1.10:2376")
		}
		hasCert := req.TLSCert != "" || (existing != nil && existing.TLSCert != "")
		hasKey := req.TLSKey != "" || (existing != nil && existing.TLSKey != "")
		if req.ClearCredentials || !hasCert || !hasKey {
			return errs.Invalid("TLS 模式必须提供客户端证书与私钥（编辑时留空表示保持原值）")
		}
	case "ssh":
		if strings.TrimSpace(req.Host) == "" {
			return errs.Invalid("SSH 模式必须填写主机地址，例如 192.168.1.10:22")
		}
		if strings.TrimSpace(req.SSHUser) == "" {
			return errs.Invalid("SSH 模式必须填写登录用户")
		}
		hasPassword := req.SSHPassword != "" || (existing != nil && existing.SSHPassword != "")
		hasKey := req.SSHKey != "" || (existing != nil && existing.SSHKey != "")
		if !req.ClearCredentials && !hasPassword && !hasKey {
			return errs.Invalid("SSH 模式必须提供密码或私钥（编辑时留空表示保持原值）")
		}
	default:
		return errs.Invalid("不支持的连接类型，可选 local / tcp / tls / ssh")
	}
	if req.Name == "" {
		return errs.Invalid("环境名称不能为空")
	}
	return nil
}

// DeleteDockerEnvironment 删除环境（级联删除 Stack 与授权）。
func (s *Service) DeleteDockerEnvironment(ctx context.Context, p *principal.Info, envID string) error {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessManage)
	if err != nil {
		return err
	}
	if err := s.DockerStore.DeleteEnvironment(ctx, envID); err != nil {
		return errs.ErrDB.WithErr(err)
	}
	s.Docker.Invalidate(envID)
	// 删除的是默认环境时，把剩余第一个环境设为默认。
	if env.IsDefault {
		rest, err := s.DockerStore.Environments(ctx)
		if err == nil && len(rest) > 0 {
			_ = s.DockerStore.SetDefaultEnvironment(ctx, rest[0].ID)
		}
	}
	return nil
}

// TestDockerEnvironment 测试连接并刷新状态。
func (s *Service) TestDockerEnvironment(ctx context.Context, p *principal.Info, envID string) (*docker.Info, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessRead)
	if err != nil {
		return nil, err
	}
	testCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	info, err := s.Docker.Test(testCtx, env)
	if err != nil {
		msg := truncateText(err.Error(), 500)
		_ = s.DockerStore.SetEnvironmentStatus(testCtx, envID, "offline", msg)
		return nil, errs.New(50005, "Docker 环境连接失败："+msg)
	}
	_ = s.DockerStore.SetEnvironmentStatus(testCtx, envID, "online", "")
	return info, nil
}

// DockerGrantOptions 可选的角色/用户（资源授权页面使用）。
func (s *Service) DockerGrantOptions(ctx context.Context, p *principal.Info) ([]DockerGrantOption, error) {
	if err := s.dockerReady(); err != nil {
		return nil, err
	}
	var roles []entity.Role
	if err := s.Repo.DB.WithContext(ctx).Model(&entity.Role{}).
		Where("tenant_id = ? AND status = 1 AND deleted = 0", p.TenantID).
		Order("sort ASC").Find(&roles).Error; err != nil {
		return nil, errs.ErrDB.WithErr(err)
	}
	var users []entity.User
	if err := s.Repo.DB.WithContext(ctx).Model(&entity.User{}).
		Where("tenant_id = ? AND status = 1 AND deleted = 0", p.TenantID).
		Order("id ASC").Limit(500).Find(&users).Error; err != nil {
		return nil, errs.ErrDB.WithErr(err)
	}
	out := make([]DockerGrantOption, 0, len(roles)+len(users))
	for _, r := range roles {
		out = append(out, DockerGrantOption{SubjectType: "role", SubjectID: fmt.Sprintf("%d", r.ID), Name: r.Name, Detail: r.Code})
	}
	for _, u := range users {
		detail := u.Username
		if u.Nickname != "" {
			detail += "（" + u.Nickname + "）"
		}
		out = append(out, DockerGrantOption{SubjectType: "user", SubjectID: fmt.Sprintf("%d", u.ID), Name: u.Username, Detail: detail})
	}
	return out, nil
}

// DockerGrantsOfEnvironment 环境授权列表。
func (s *Service) DockerGrantsOfEnvironment(ctx context.Context, p *principal.Info, envID string) ([]DockerGrantView, error) {
	if _, err := s.requireDockerEnv(ctx, p, envID, DockerAccessManage); err != nil {
		return nil, err
	}
	grants, err := s.DockerStore.GrantsOfEnvironment(ctx, envID)
	if err != nil {
		return nil, errs.ErrDB.WithErr(err)
	}
	names := map[string]string{}
	var roles []entity.Role
	if err := s.Repo.DB.WithContext(ctx).Model(&entity.Role{}).Where("deleted = 0").Find(&roles).Error; err == nil {
		for _, r := range roles {
			names["role:"+fmt.Sprintf("%d", r.ID)] = r.Name
		}
	}
	var users []entity.User
	if err := s.Repo.DB.WithContext(ctx).Model(&entity.User{}).Where("deleted = 0").Find(&users).Error; err == nil {
		for _, u := range users {
			names["user:"+fmt.Sprintf("%d", u.ID)] = u.Username
		}
	}
	out := make([]DockerGrantView, 0, len(grants))
	for _, g := range grants {
		out = append(out, DockerGrantView{
			ID: g.ID, SubjectType: g.SubjectType, SubjectID: g.SubjectID,
			Name: names[g.SubjectType+":"+g.SubjectID], AccessLevel: g.AccessLevel, CreatedAt: g.CreatedAt,
		})
	}
	return out, nil
}

// SetDockerGrants 覆盖式保存环境授权。
func (s *Service) SetDockerGrants(ctx context.Context, p *principal.Info, envID string, req *DockerGrantReq) error {
	if _, err := s.requireDockerEnv(ctx, p, envID, DockerAccessManage); err != nil {
		return err
	}
	// 校验被授权主体真实存在（角色/用户来自主库）。
	roleIDs := map[string]bool{}
	userIDs := map[string]bool{}
	var roles []entity.Role
	if err := s.Repo.DB.WithContext(ctx).Model(&entity.Role{}).Where("tenant_id = ? AND deleted = 0", p.TenantID).Find(&roles).Error; err == nil {
		for _, r := range roles {
			roleIDs[fmt.Sprintf("%d", r.ID)] = true
		}
	}
	var users []entity.User
	if err := s.Repo.DB.WithContext(ctx).Model(&entity.User{}).Where("tenant_id = ? AND deleted = 0", p.TenantID).Find(&users).Error; err == nil {
		for _, u := range users {
			userIDs[fmt.Sprintf("%d", u.ID)] = true
		}
	}
	seen := map[string]bool{}
	grants := make([]dockerstore.Grant, 0, len(req.Grants))
	for _, g := range req.Grants {
		key := g.SubjectType + ":" + g.SubjectID
		if seen[key] {
			continue
		}
		seen[key] = true
		if g.SubjectType == "role" && !roleIDs[g.SubjectID] {
			return errs.Invalid("授权角色不存在或不属于当前租户")
		}
		if g.SubjectType == "user" && !userIDs[g.SubjectID] {
			return errs.Invalid("授权用户不存在或不属于当前租户")
		}
		grants = append(grants, dockerstore.Grant{SubjectType: g.SubjectType, SubjectID: g.SubjectID, AccessLevel: g.AccessLevel})
	}
	if err := s.DockerStore.SetGrants(ctx, envID, grants, fmt.Sprintf("%d", p.UserID)); err != nil {
		return errs.ErrDB.WithErr(err)
	}
	return nil
}

// DockerOverview 环境概览（含 Stack 数）。
func (s *Service) DockerOverview(ctx context.Context, p *principal.Info, envID string) (*DockerOverviewView, error) {
	env, err := s.requireDockerEnv(ctx, p, envID, DockerAccessRead)
	if err != nil {
		return nil, err
	}
	ov, err := s.Docker.Overview(ctx, env)
	if err != nil {
		return nil, dockerErr(err)
	}
	stacks, err := s.DockerStore.Stacks(ctx, envID)
	if err != nil {
		return nil, errs.ErrDB.WithErr(err)
	}
	view := &DockerOverviewView{Overview: *ov}
	view.StackList = make([]DockerStackView, 0, len(stacks))
	rtMap, _ := s.Docker.StackRuntimeMap(ctx, env)
	for i := range stacks {
		st := &stacks[i]
		rt := rtMap[st.Type+":"+st.Name]
		if rt.Status == "" {
			rt.Status = "inactive"
		}
		view.StackList = append(view.StackList, s.toStackView(st, env.Name, DockerAccessRead, rt, nil))
	}
	view.StackList = view.StackList[:min(len(view.StackList), 20)]
	// 概览卡片上的 Stack 数量来自 docker.Overview，这里用数据库数量覆盖（含尚未部署的 Stack）。
	view.Overview.Stacks = len(stacks)
	return view, nil
}

// min 兼容旧版本（Go 1.21+ 内置 min 对 int 可用，这里保留显式函数避免歧义）。
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (s *Service) toStackView(st *dockerstore.Stack, envName string, level int, rt docker.StackRuntime, rtErr error) DockerStackView {
	v := DockerStackView{
		ID: st.ID, EnvironmentID: st.EnvironmentID, EnvironmentName: envName,
		Name: st.Name, Type: st.Type, Status: st.Status, StatusMessage: st.StatusMessage,
		Version: st.Version, LastDeployedAt: st.LastDeployedAt,
		CreatedBy: st.CreatedBy, UpdatedBy: st.UpdatedBy, CreatedAt: st.CreatedAt, UpdatedAt: st.UpdatedAt,
		AccessLevel: level, Runtime: rt,
	}
	if rtErr != nil {
		v.RuntimeError = truncateText(rtErr.Error(), 300)
	} else if rt.Status == "" {
		v.Runtime.Status = "inactive"
	}
	// 运行态优先：数据库状态为 active 但运行时已不存在时，显示为 inactive，避免误导。
	if rtErr == nil && rt.Status == "active" {
		v.Status = "active"
	}
	if rtErr == nil && rt.Status != "active" && st.Status == "active" {
		v.Status = "inactive"
	}
	if st.LastDeployedAt == nil {
		v.PendingChanges = true
	} else if st.UpdatedAt > *st.LastDeployedAt {
		v.PendingChanges = true
	}
	return v
}

func (s *Service) stackEnvIDs(ctx context.Context, p *principal.Info) ([]string, error) {
	all, err := s.DockerStore.Environments(ctx)
	if err != nil {
		return nil, errs.ErrDB.WithErr(err)
	}
	if p.IsSuper {
		ids := make([]string, 0, len(all))
		for _, e := range all {
			ids = append(ids, e.ID)
		}
		return ids, nil
	}
	levels, err := s.DockerStore.AccessibleEnvironmentLevels(ctx, fmt.Sprintf("%d", p.UserID), s.envRoleIDs(p))
	if err != nil {
		return nil, errs.ErrDB.WithErr(err)
	}
	ids := make([]string, 0, len(levels))
	for id, lvl := range levels {
		if lvl > 0 {
			ids = append(ids, id)
		}
	}
	return ids, nil
}
