package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"web-basic/internal/docker"
	"web-basic/internal/dockerstore"
	"web-basic/internal/principal"
	"web-basic/pkg/errs"
	"web-basic/pkg/id"
)

// ListDockerStacks 当前用户可见的 Stack（附运行态）。
func (s *Service) ListDockerStacks(ctx context.Context, p *principal.Info, envID string) ([]DockerStackView, error) {
	if err := s.dockerReady(); err != nil {
		return nil, err
	}
	envs, err := s.DockerStore.Environments(ctx)
	if err != nil {
		return nil, errs.ErrDB.WithErr(err)
	}
	envByID := make(map[string]*dockerstore.Environment, len(envs))
	for i := range envs {
		envByID[envs[i].ID] = &envs[i]
	}
	levels := map[string]int{}
	if p.IsSuper {
		for _, e := range envs {
			levels[e.ID] = DockerAccessManage
		}
	} else {
		levels, err = s.DockerStore.AccessibleEnvironmentLevels(ctx, fmt.Sprintf("%d", p.UserID), s.envRoleIDs(p))
		if err != nil {
			return nil, errs.ErrDB.WithErr(err)
		}
	}
	envIDs := make([]string, 0, len(levels))
	if envID != "" {
		if levels[envID] == 0 {
			return nil, errs.New(errs.ErrForbidden.Code, "没有该 Docker 环境的访问权限")
		}
		envIDs = append(envIDs, envID)
	} else {
		for id, lvl := range levels {
			if lvl > 0 {
				envIDs = append(envIDs, id)
			}
		}
	}
	sort.Strings(envIDs)

	var stacks []dockerstore.Stack
	for _, eid := range envIDs {
		list, err := s.DockerStore.Stacks(ctx, eid)
		if err != nil {
			return nil, errs.ErrDB.WithErr(err)
		}
		stacks = append(stacks, list...)
	}

	// 每个环境一次批量拉取容器/服务，聚合出所有 Stack 的运行态（避免 N+1）。
	runtimeByEnv := make(map[string]map[string]docker.StackRuntime, len(envIDs))
	runtimeErr := make(map[string]error, len(envIDs))
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, eid := range envIDs {
		env := envByID[eid]
		if env == nil {
			continue
		}
		wg.Add(1)
		go func(e *dockerstore.Environment) {
			defer wg.Done()
			probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			defer cancel()
			rt, err := s.Docker.StackRuntimeMap(probeCtx, e)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				runtimeErr[e.ID] = err
				return
			}
			runtimeByEnv[e.ID] = rt
		}(env)
	}
	wg.Wait()

	out := make([]DockerStackView, 0, len(stacks))
	for i := range stacks {
		st := &stacks[i]
		env := envByID[st.EnvironmentID]
		envName := ""
		if env != nil {
			envName = env.Name
		}
		var rt docker.StackRuntime
		var rerr error
		if m, ok := runtimeByEnv[st.EnvironmentID]; ok {
			rt = m[st.Type+":"+st.Name]
			if rt.Status == "" {
				rt.Status = "inactive"
			}
		} else {
			rerr = runtimeErr[st.EnvironmentID]
		}
		out = append(out, s.toStackView(st, envName, levels[st.EnvironmentID], rt, rerr))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out, nil
}

// GetDockerStack Stack 详情（含历史版本与操作记录）。
func (s *Service) GetDockerStack(ctx context.Context, p *principal.Info, stackID string) (*DockerStackDetail, error) {
	if err := s.dockerReady(); err != nil {
		return nil, err
	}
	st, err := s.DockerStore.Stack(ctx, stackID)
	if err != nil {
		return nil, errs.ErrDB.WithErr(err)
	}
	if st == nil {
		return nil, errs.ErrNotFound
	}
	env, err := s.requireDockerEnv(ctx, p, st.EnvironmentID, DockerAccessRead)
	if err != nil {
		return nil, err
	}
	level, _ := s.dockerEnvLevel(ctx, p, st.EnvironmentID)
	rt, rerr := s.Docker.StackRuntime(ctx, env, st.Type, st.Name)
	if rt.Status == "" {
		rt.Status = "inactive"
	}
	revisions, err := s.DockerStore.Revisions(ctx, stackID)
	if err != nil {
		return nil, errs.ErrDB.WithErr(err)
	}
	activities, err := s.DockerStore.Activities(ctx, stackID, 50)
	if err != nil {
		return nil, errs.ErrDB.WithErr(err)
	}
	return &DockerStackDetail{
		DockerStackView: s.toStackView(st, env.Name, level, rt, rerr),
		ComposeContent:  st.ComposeContent,
		EnvContent:      st.EnvContent,
		Revisions:       revisions,
		Activities:      activities,
	}, nil
}

// CreateDockerStack 新建 Stack（写入 SQLite，不自动部署）。
func (s *Service) CreateDockerStack(ctx context.Context, p *principal.Info, req *DockerStackReq) (string, error) {
	if err := s.dockerReady(); err != nil {
		return "", err
	}
	env, err := s.requireDockerEnv(ctx, p, req.EnvironmentID, DockerAccessOperate)
	if err != nil {
		return "", err
	}
	if !dockerNameRe.MatchString(req.Name) {
		return "", errs.Invalid("Stack 名称只能使用小写字母、数字、- 和 _，并以字母或数字开头（最长 64 位）")
	}
	if req.Type == "swarm" {
		if err := s.ensureSwarm(ctx, env); err != nil {
			return "", err
		}
	}
	if _, err := docker.LoadCompose(ctx, req.Name, req.ComposeContent, req.EnvContent); err != nil {
		return "", errs.InvalidErr(err)
	}
	if old, err := s.DockerStore.StackByName(ctx, req.EnvironmentID, req.Name); err != nil {
		return "", errs.ErrDB.WithErr(err)
	} else if old != nil {
		return "", errs.ErrDuplicate
	}
	st := &dockerstore.Stack{
		ID: fmt.Sprintf("%d", id.Next()), EnvironmentID: req.EnvironmentID, Name: req.Name,
		Type: req.Type, ComposeContent: req.ComposeContent, EnvContent: req.EnvContent,
		Status: "inactive", StatusMessage: "", Version: 1,
		CreatedBy: p.Username, UpdatedBy: p.Username,
	}
	if err := s.DockerStore.CreateStack(ctx, st, p.Username, "创建 Stack"); err != nil {
		return "", errs.ErrDB.WithErr(err)
	}
	return st.ID, nil
}

// UpdateDockerStack 保存 compose/.env 内容（产生新历史版本，不自动部署）。
func (s *Service) UpdateDockerStack(ctx context.Context, p *principal.Info, stackID string, req *DockerStackUpdateReq) error {
	if err := s.dockerReady(); err != nil {
		return err
	}
	st, err := s.DockerStore.Stack(ctx, stackID)
	if err != nil {
		return errs.ErrDB.WithErr(err)
	}
	if st == nil {
		return errs.ErrNotFound
	}
	env, err := s.requireDockerEnv(ctx, p, st.EnvironmentID, DockerAccessOperate)
	if err != nil {
		return err
	}
	if req.Type == "swarm" {
		if err := s.ensureSwarm(ctx, env); err != nil {
			return err
		}
	}
	if _, err := docker.LoadCompose(ctx, st.Name, req.ComposeContent, req.EnvContent); err != nil {
		return errs.InvalidErr(err)
	}
	st.Type = req.Type
	st.ComposeContent = req.ComposeContent
	st.EnvContent = req.EnvContent
	st.Version++
	st.UpdatedBy = p.Username
	if err := s.DockerStore.SaveStackContent(ctx, st, p.Username, fmt.Sprintf("保存第 %d 版", st.Version)); err != nil {
		return errs.ErrDB.WithErr(err)
	}
	return nil
}

func (s *Service) ensureSwarm(ctx context.Context, env *dockerstore.Environment) error {
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	state, err := s.Docker.SwarmState(probeCtx, env)
	if err != nil {
		return dockerErr(err)
	}
	if !state.Active {
		return errs.Invalid("当前环境未启用 Swarm，请先在「Swarm 集群」页面初始化或加入集群")
	}
	if !state.ControlAvailable {
		return errs.Invalid("当前环境不是 Swarm 管理节点，无法部署 Stack")
	}
	return nil
}

// DeployDockerStack 部署/重新部署 Stack。
func (s *Service) DeployDockerStack(ctx context.Context, p *principal.Info, stackID string, opts docker.DeployOptions) error {
	if err := s.dockerReady(); err != nil {
		return err
	}
	st, err := s.DockerStore.Stack(ctx, stackID)
	if err != nil {
		return errs.ErrDB.WithErr(err)
	}
	if st == nil {
		return errs.ErrNotFound
	}
	env, err := s.requireDockerEnv(ctx, p, st.EnvironmentID, DockerAccessOperate)
	if err != nil {
		return err
	}
	if st.Type == "swarm" {
		if err := s.ensureSwarm(ctx, env); err != nil {
			return err
		}
	}
	_ = s.DockerStore.SetStackStatus(ctx, stackID, "deploying", "正在部署...", nil)

	deployCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Minute)
	defer cancel()
	if st.Type == "swarm" {
		err = s.Docker.DeploySwarm(deployCtx, env, st, opts)
	} else {
		err = s.Docker.DeployCompose(deployCtx, env, st, opts)
	}
	nowStr := time.Now().Format(time.RFC3339)
	if err != nil {
		msg := truncateText(err.Error(), 1000)
		_ = s.DockerStore.SetStackStatus(ctx, stackID, "error", msg, nil)
		_ = s.DockerStore.AddActivity(ctx, stackID, "deploy", "failed", msg, p.Username)
		return dockerErr(err)
	}
	_ = s.DockerStore.SetStackStatus(ctx, stackID, "active", "", &nowStr)
	_ = s.DockerStore.AddActivity(ctx, stackID, "deploy", "success", "部署成功", p.Username)
	return nil
}

// StopDockerStack 停止 Stack。
func (s *Service) StopDockerStack(ctx context.Context, p *principal.Info, stackID string) error {
	if err := s.dockerReady(); err != nil {
		return err
	}
	st, err := s.DockerStore.Stack(ctx, stackID)
	if err != nil {
		return errs.ErrDB.WithErr(err)
	}
	if st == nil {
		return errs.ErrNotFound
	}
	env, err := s.requireDockerEnv(ctx, p, st.EnvironmentID, DockerAccessOperate)
	if err != nil {
		return err
	}
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
	defer cancel()
	if st.Type == "swarm" {
		err = s.Docker.StopSwarm(stopCtx, env, st)
	} else {
		err = s.Docker.StopCompose(stopCtx, env, st)
	}
	if err != nil {
		msg := truncateText(err.Error(), 1000)
		_ = s.DockerStore.AddActivity(ctx, stackID, "stop", "failed", msg, p.Username)
		return dockerErr(err)
	}
	_ = s.DockerStore.SetStackStatus(ctx, stackID, "inactive", "已停止", nil)
	_ = s.DockerStore.AddActivity(ctx, stackID, "stop", "success", "已停止", p.Username)
	return nil
}

// DeleteDockerStack 删除 Stack（可选同时清理运行时资源）。
func (s *Service) DeleteDockerStack(ctx context.Context, p *principal.Info, stackID string, deleteRuntime, removeVolumes bool) error {
	if err := s.dockerReady(); err != nil {
		return err
	}
	st, err := s.DockerStore.Stack(ctx, stackID)
	if err != nil {
		return errs.ErrDB.WithErr(err)
	}
	if st == nil {
		return errs.ErrNotFound
	}
	env, err := s.requireDockerEnv(ctx, p, st.EnvironmentID, DockerAccessOperate)
	if err != nil {
		return err
	}
	if deleteRuntime {
		delCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
		defer cancel()
		if st.Type == "swarm" {
			err = s.Docker.RemoveSwarm(delCtx, env, st, removeVolumes)
		} else {
			err = s.Docker.RemoveCompose(delCtx, env, st, removeVolumes)
		}
		if err != nil {
			return dockerErr(err)
		}
	}
	if err := s.DockerStore.DeleteStack(ctx, stackID); err != nil {
		return errs.ErrDB.WithErr(err)
	}
	return nil
}

// RollbackDockerStack 回滚到指定历史版本（可选立即重新部署）。
func (s *Service) RollbackDockerStack(ctx context.Context, p *principal.Info, stackID string, version int, deployNow bool) error {
	if err := s.dockerReady(); err != nil {
		return err
	}
	st, err := s.DockerStore.Stack(ctx, stackID)
	if err != nil {
		return errs.ErrDB.WithErr(err)
	}
	if st == nil {
		return errs.ErrNotFound
	}
	if _, err := s.requireDockerEnv(ctx, p, st.EnvironmentID, DockerAccessOperate); err != nil {
		return err
	}
	rev, err := s.DockerStore.Revision(ctx, stackID, version)
	if err != nil {
		return errs.ErrDB.WithErr(err)
	}
	if rev == nil {
		return errs.ErrNotFound
	}
	st.ComposeContent = rev.ComposeContent
	st.EnvContent = rev.EnvContent
	st.Version++
	st.UpdatedBy = p.Username
	if err := s.DockerStore.SaveStackContent(ctx, st, p.Username, fmt.Sprintf("回滚到第 %d 版", version)); err != nil {
		return errs.ErrDB.WithErr(err)
	}
	_ = s.DockerStore.AddActivity(ctx, stackID, "rollback", "success", fmt.Sprintf("回滚到第 %d 版", version), p.Username)
	if deployNow {
		return s.DeployDockerStack(ctx, p, stackID, docker.DeployOptions{})
	}
	return nil
}

// ExportDockerStacks 导出当前用户可见的全部 Stack（JSON 供整库恢复）。
func (s *Service) ExportDockerStacks(ctx context.Context, p *principal.Info) ([]DockerStackExport, error) {
	if err := s.dockerReady(); err != nil {
		return nil, err
	}
	envs, err := s.DockerStore.Environments(ctx)
	if err != nil {
		return nil, errs.ErrDB.WithErr(err)
	}
	levels := map[string]int{}
	if p.IsSuper {
		for _, e := range envs {
			levels[e.ID] = DockerAccessManage
		}
	} else {
		levels, err = s.DockerStore.AccessibleEnvironmentLevels(ctx, fmt.Sprintf("%d", p.UserID), s.envRoleIDs(p))
		if err != nil {
			return nil, errs.ErrDB.WithErr(err)
		}
	}
	out := make([]DockerStackExport, 0, 16)
	for _, env := range envs {
		if levels[env.ID] == 0 {
			continue
		}
		stacks, err := s.DockerStore.Stacks(ctx, env.ID)
		if err != nil {
			return nil, errs.ErrDB.WithErr(err)
		}
		for _, st := range stacks {
			out = append(out, DockerStackExport{
				EnvironmentName: env.Name, Name: st.Name, Type: st.Type,
				ComposeContent: st.ComposeContent, EnvContent: st.EnvContent,
			})
		}
	}
	return out, nil
}

// ImportDockerStacks 导入 Stack（按环境名称匹配；需要目标环境 operate 权限）。
func (s *Service) ImportDockerStacks(ctx context.Context, p *principal.Info, req *DockerStackImportReq) (map[string]int, error) {
	if err := s.dockerReady(); err != nil {
		return nil, err
	}
	envs, err := s.DockerStore.Environments(ctx)
	if err != nil {
		return nil, errs.ErrDB.WithErr(err)
	}
	envByName := make(map[string]*dockerstore.Environment, len(envs))
	for i := range envs {
		envByName[strings.ToLower(envs[i].Name)] = &envs[i]
	}
	created, updated, skipped := 0, 0, 0
	for _, item := range req.Stacks {
		item.Name = strings.TrimSpace(item.Name)
		env := envByName[strings.ToLower(strings.TrimSpace(item.EnvironmentName))]
		if env == nil {
			skipped++
			continue
		}
		if _, err := s.requireDockerEnv(ctx, p, env.ID, DockerAccessOperate); err != nil {
			return nil, err
		}
		if !dockerNameRe.MatchString(item.Name) {
			return nil, errs.Invalid("导入的 Stack 名称不合法：" + item.Name)
		}
		if item.Type != "compose" && item.Type != "swarm" {
			return nil, errs.Invalid("导入的 Stack 类型不合法：" + item.Name)
		}
		if _, err := docker.LoadCompose(ctx, item.Name, item.ComposeContent, item.EnvContent); err != nil {
			return nil, errs.InvalidErr(fmt.Errorf("Stack「%s」: %w", item.Name, err))
		}
		old, err := s.DockerStore.StackByName(ctx, env.ID, item.Name)
		if err != nil {
			return nil, errs.ErrDB.WithErr(err)
		}
		if old != nil {
			if !req.Overwrite {
				skipped++
				continue
			}
			old.Type = item.Type
			old.ComposeContent = item.ComposeContent
			old.EnvContent = item.EnvContent
			old.Version++
			old.UpdatedBy = p.Username
			if err := s.DockerStore.SaveStackContent(ctx, old, p.Username, "从备份导入"); err != nil {
				return nil, errs.ErrDB.WithErr(err)
			}
			updated++
			continue
		}
		st := &dockerstore.Stack{
			ID: fmt.Sprintf("%d", id.Next()), EnvironmentID: env.ID, Name: item.Name, Type: item.Type,
			ComposeContent: item.ComposeContent, EnvContent: item.EnvContent, Status: "inactive",
			Version: 1, CreatedBy: p.Username, UpdatedBy: p.Username,
		}
		if err := s.DockerStore.CreateStack(ctx, st, p.Username, "从备份导入"); err != nil {
			return nil, errs.ErrDB.WithErr(err)
		}
		created++
	}
	return map[string]int{"created": created, "updated": updated, "skipped": skipped}, nil
}

// BackupDockerDatabase 下载 SQLite 备份（仅超级管理员）。
func (s *Service) BackupDockerDatabase(ctx context.Context, p *principal.Info) ([]byte, string, error) {
	if err := s.dockerReady(); err != nil {
		return nil, "", err
	}
	if !p.IsSuper {
		return nil, "", errs.New(errs.ErrForbidden.Code, "只有超级管理员可以下载 Docker 数据库备份")
	}
	b, err := s.DockerStore.BackupBytes(ctx)
	if err != nil {
		return nil, "", errs.ErrInternal.WithErr(err)
	}
	name := fmt.Sprintf("tomato-docker-%s.db", time.Now().Format("20060102-150405"))
	return b, name, nil
}
