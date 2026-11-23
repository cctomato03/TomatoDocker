// Package bootstrap 应用装配：配置 → 日志 → 数据库 → 缓存 → 迁移 → 种子 → 路由 → HTTP 服务。
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"

	"web-basic/internal/api"
	"web-basic/internal/config"
	"web-basic/internal/database"
	"web-basic/internal/docker"
	"web-basic/internal/dockerstore"
	"web-basic/internal/logger"
	"web-basic/internal/migrate"
	"web-basic/internal/repository"
	"web-basic/internal/router"
	"web-basic/internal/scheduler"
	"web-basic/internal/service"
	"web-basic/pkg/cache"
	"web-basic/pkg/captcha"
	"web-basic/pkg/id"
	"web-basic/pkg/jwt"
	"web-basic/pkg/storage"
)

// ensureDefaultDockerEnvironment 首次启动时登记本地 Docker，之后不再覆盖（尊重用户删除/改名）。
func ensureDefaultDockerEnvironment(ctx context.Context, cfg *config.Config, store *dockerstore.Store) error {
	n, err := store.CountEnvironments(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	socket := ""
	if runtime.GOOS == "windows" {
		socket = "npipe:////./pipe/docker_engine"
	} else {
		socket = "unix:///var/run/docker.sock"
	}
	if cfg.Docker.Socket != "" {
		socket = cfg.Docker.Socket
	}
	name := cfg.Docker.DefaultLocalName
	if name == "" {
		name = "本地 Docker"
	}
	env := &dockerstore.Environment{
		ID:         fmt.Sprintf("%d", id.Next()),
		Name:       name,
		Type:       "local",
		SocketPath: socket,
		IsDefault:  true,
		CreatedBy:  "system",
	}
	if err := store.CreateEnvironment(ctx, env); err != nil {
		return err
	}
	logger.S().Infow("已登记本地 Docker 环境", "name", name, "socket", socket)
	return nil
}

// App 运行中的应用实例。
type App struct {
	Cfg     *config.Config
	Svc     *service.Service
	Engine  *gin.Engine
	server  *http.Server
	closeFn []func()
}

// Run 启动应用，ctx 取消时优雅关停。
func Run(ctx context.Context, configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	if err := logger.Init(cfg.Log); err != nil {
		return fmt.Errorf("初始化日志失败: %w", err)
	}
	defer logger.Sync()

	logger.S().Infow("starting", "app", cfg.App.Name, "env", cfg.App.Env, "port", cfg.App.Port)

	// 雪花 ID
	if err := id.Init(1); err != nil {
		return fmt.Errorf("初始化 ID 生成器失败: %w", err)
	}

	// MySQL
	db, err := database.OpenMySQL(cfg.MySQL)
	if err != nil {
		return err
	}
	if cfg.MySQL.AutoMigrate {
		if err := migrate.Run(db); err != nil {
			return fmt.Errorf("数据库迁移失败: %w", err)
		}
		logger.S().Info("数据库迁移完成")
	}

	// Redis
	rdb, err := database.OpenRedis(cfg.Redis)
	if err != nil {
		return err
	}
	c := cache.New(rdb, cfg.Redis.Prefix)

	// 存储
	st, err := storage.New(cfg.Storage)
	if err != nil {
		return fmt.Errorf("初始化文件存储失败: %w", err)
	}

	// 基础设施服务
	jwtMgr := jwt.New(cfg.JWT.Secret, cfg.JWT.SecretPrev, cfg.JWT.AccessTTL, cfg.JWT.RefreshTTL, cfg.JWT.Issuer)
	captchaSvc := captcha.NewService(
		captcha.NewRedisStore(rdb, cfg.Redis.Prefix, 2*time.Minute),
		cfg.Auth.CaptchaEnabled,
	)

	// Docker 管理模块：环境与 Stack 使用独立 SQLite（便于整库备份/迁移）。
	var dockerStore *dockerstore.Store
	var dockerMgr *docker.Manager
	if cfg.Docker.Enabled {
		dockerStore, err = dockerstore.Open(cfg.Docker.Database)
		if err != nil {
			return fmt.Errorf("初始化 Docker SQLite 存储失败: %w", err)
		}
		defer dockerStore.Close()
		dockerMgr = docker.NewManager(dockerStore, cfg.Docker, cfg.JWT.Secret)
		defer dockerMgr.Close()
		if err := ensureDefaultDockerEnvironment(ctx, cfg, dockerStore); err != nil {
			return fmt.Errorf("初始化本地 Docker 环境失败: %w", err)
		}
	} else {
		logger.S().Warn("Docker 管理模块已关闭（docker.enabled=false）")
	}

	repo := repository.New(db, c, cfg)
	svc := service.New(&service.Deps{
		Repo:        repo,
		Cache:       c,
		Cfg:         cfg,
		JWT:         jwtMgr,
		Captcha:     captchaSvc,
		Storage:     st,
		Docker:      dockerMgr,
		DockerStore: dockerStore,
	})

	// 升级后主体缓存里可能没有新增字段（如 RoleIDs）；启动时统一失效一次，避免角色资源授权最长 30 分钟不生效。
	svc.InvalidateAll(ctx)

	// 初始化超管账号
	if err := ensureAdmin(ctx, cfg, repo, svc); err != nil {
		return fmt.Errorf("初始化管理员失败: %w", err)
	}

	h := api.New(svc)
	engine := router.New(cfg, svc, h)

	// 定时任务调度器
	svc.RegisterJobHandlers()
	sch := scheduler.New(svc)
	if cfg.Scheduler.Enabled {
		sch.Start(ctx)
		defer sch.Stop()
		h.ReloadJobs = sch.Reload
	}

	app := &App{Cfg: cfg, Svc: svc, Engine: engine}
	app.server = &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.App.Port),
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.S().Infof("HTTP 服务已启动: http://localhost:%d", cfg.App.Port)
		if err := app.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.S().Info("收到退出信号，开始优雅关停")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := app.server.Shutdown(shutdownCtx); err != nil {
		logger.S().Errorw("HTTP 服务关停异常", "err", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
	_ = rdb.Close()

	logger.S().Info("已退出")
	return nil
}
