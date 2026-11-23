// Package database 数据库初始化。
package database

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	glog "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"web-basic/internal/config"
)

// OpenSQLite 初始化 SQLite 数据库（纯 Go 驱动，无 CGO 依赖）。
//
// 单连接：SQLite 同一时刻只允许一个写事务，单连接可彻底避免 "database is locked"；
// 本项目为单实例管理后台，读写吞吐远低于连接数上限，代价可接受。
func OpenSQLite(cfg config.SQLite) (*gorm.DB, error) {
	if cfg.Path == "" {
		return nil, fmt.Errorf("sqlite.path 不能为空")
	}
	abs, err := filepath.Abs(cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("解析 SQLite 路径失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return nil, fmt.Errorf("创建 SQLite 目录失败: %w", err)
	}

	level := glog.Warn
	if cfg.LogSQL {
		level = glog.Info
	}
	slow := time.Duration(cfg.SlowThresholdMS) * time.Millisecond
	if slow <= 0 {
		slow = 200 * time.Millisecond
	}

	db, err := gorm.Open(sqlite.Open(abs), &gorm.Config{
		SkipDefaultTransaction: true,
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true, // 表名不加复数：sys_user 而非 sys_users
		},
		NowFunc:         func() time.Time { return time.Now().Local() },
		TranslateError:  true,
		PrepareStmt:     false,
		CreateBatchSize: 200,
		Logger: glog.New(log.New(os.Stdout, "[GORM] ", log.LstdFlags), glog.Config{
			SlowThreshold:             slow,
			LogLevel:                  level,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite 失败: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("获取 SQLite 连接池失败: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)

	busy := cfg.BusyTimeoutMS
	if busy <= 0 {
		busy = 5000
	}
	pragmas := []string{
		fmt.Sprintf("PRAGMA busy_timeout=%d", busy),
		"PRAGMA foreign_keys=ON",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA temp_store=MEMORY",
	}
	if cfg.WAL {
		pragmas = append([]string{"PRAGMA journal_mode=WAL"}, pragmas...)
	}
	for _, p := range pragmas {
		if err := db.Exec(p).Error; err != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("设置 SQLite 参数失败（%s）: %w", p, err)
		}
	}
	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("SQLite 探活失败: %w", err)
	}
	return db, nil
}
