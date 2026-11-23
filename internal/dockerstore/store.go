// Package dockerstore Docker 管理模块的独立 SQLite 存储。
//
// 设计目标：
//   - Stack 的 compose 内容、.env、历史版本、环境连接信息、资源授权全部存放在单个 SQLite 文件；
//   - 只依赖 SQLite，迁移时只需拷贝 .db 文件；
//   - 纯 Go 驱动（modernc.org/sqlite），发布二进制无需 CGO。
package dockerstore

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/glebarez/go-sqlite"

	"web-basic/pkg/id"
)

//go:embed schema.sql
var schemaSQL string

// Store SQLite 数据访问入口。
type Store struct {
	db   *sql.DB
	path string
}

// Open 打开（必要时创建）SQLite 数据库并执行幂等建表。
func Open(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("docker.database 不能为空")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("解析 SQLite 路径失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return nil, fmt.Errorf("创建 SQLite 目录失败: %w", err)
	}

	db, err := sql.Open("sqlite", abs)
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite 失败: %w", err)
	}
	// 单连接：SQLite 同一时刻只允许一个写事务，单连接可避免大量 "database is locked"。
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("设置 SQLite 参数失败（%s）: %w", pragma, err)
		}
	}
	for _, stmt := range splitSQL(schemaSQL) {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			return nil, fmt.Errorf("初始化 SQLite 结构失败: %w", err)
		}
	}
	return &Store{db: db, path: abs}, nil
}

// Close 关闭数据库。
func (s *Store) Close() error { return s.db.Close() }

// Path 数据库文件绝对路径（备份接口使用）。
func (s *Store) Path() string { return s.path }

// BackupBytes 返回一致性快照：先 checkpoint 把 WAL 合并进主文件，再读取字节。
func (s *Store) BackupBytes(ctx context.Context) ([]byte, error) {
	if _, err := s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return nil, fmt.Errorf("SQLite checkpoint 失败: %w", err)
	}
	b, err := os.ReadFile(s.path)
	if err != nil {
		return nil, fmt.Errorf("读取 SQLite 文件失败: %w", err)
	}
	return b, nil
}

func newID() string { return fmt.Sprintf("%d", id.Next()) }

func now() string { return time.Now().Format(time.RFC3339) }

// ---------------- 环境 ----------------

const envColumns = `id, name, description, type, host, socket_path, tls_ca, tls_cert, tls_key,
 ssh_user, ssh_password, ssh_key, is_default, status, status_message, created_by, created_at, updated_at`

// Environments 全部环境。
func (s *Store) Environments(ctx context.Context) ([]Environment, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+envColumns+` FROM docker_environment ORDER BY is_default DESC, name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Environment, 0, 4)
	for rows.Next() {
		e, err := scanEnvironment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

// Environment 按 ID 查询环境；不存在返回 (nil, nil)。
func (s *Store) Environment(ctx context.Context, id string) (*Environment, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+envColumns+` FROM docker_environment WHERE id = ?`, id)
	e, err := scanEnvironment(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return e, err
}

// DefaultEnvironment 默认环境。
func (s *Store) DefaultEnvironment(ctx context.Context) (*Environment, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+envColumns+` FROM docker_environment ORDER BY is_default DESC, created_at ASC LIMIT 1`)
	e, err := scanEnvironment(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return e, err
}

// CountEnvironments 环境数量。
func (s *Store) CountEnvironments(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM docker_environment`).Scan(&n)
	return n, err
}

// CreateEnvironment 新增环境。
func (s *Store) CreateEnvironment(ctx context.Context, e *Environment) error {
	if e.ID == "" {
		e.ID = newID()
	}
	e.CreatedAt = now()
	e.UpdatedAt = e.CreatedAt
	if e.Status == "" {
		e.Status = "unknown"
	}
	if e.IsDefault {
		if err := s.clearDefault(ctx); err != nil {
			return err
		}
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO docker_environment (`+envColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		e.ID, e.Name, e.Description, e.Type, e.Host, e.SocketPath, e.TLSCa, e.TLSCert, e.TLSKey,
		e.SSHUser, e.SSHPassword, e.SSHKey, boolInt(e.IsDefault), e.Status, e.StatusMessage,
		e.CreatedBy, e.CreatedAt, e.UpdatedAt)
	return err
}

// UpdateEnvironment 覆盖环境连接配置（保留 ID/创建信息）。
func (s *Store) UpdateEnvironment(ctx context.Context, e *Environment) error {
	e.UpdatedAt = now()
	if e.IsDefault {
		if err := s.clearDefault(ctx); err != nil {
			return err
		}
	}
	res, err := s.db.ExecContext(ctx, `UPDATE docker_environment SET name=?, description=?, type=?, host=?, socket_path=?,
		tls_ca=?, tls_cert=?, tls_key=?, ssh_user=?, ssh_password=?, ssh_key=?, is_default=?, updated_at=? WHERE id=?`,
		e.Name, e.Description, e.Type, e.Host, e.SocketPath, e.TLSCa, e.TLSCert, e.TLSKey,
		e.SSHUser, e.SSHPassword, e.SSHKey, boolInt(e.IsDefault), e.UpdatedAt, e.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteEnvironment 删除环境（级联删除授权、Stack、版本与操作记录）。
func (s *Store) DeleteEnvironment(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM docker_environment WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SetDefaultEnvironment 把指定环境设为默认（其余取消默认）。
func (s *Store) SetDefaultEnvironment(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE docker_environment SET is_default=0`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE docker_environment SET is_default=1, updated_at=? WHERE id=?`, now(), id); err != nil {
		return err
	}
	return tx.Commit()
}

// SetEnvironmentStatus 记录连接探测结果。
func (s *Store) SetEnvironmentStatus(ctx context.Context, id, status, message string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE docker_environment SET status=?, status_message=?, updated_at=? WHERE id=?`,
		status, message, now(), id)
	return err
}

func (s *Store) clearDefault(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE docker_environment SET is_default=0`)
	return err
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

type scanner interface {
	Scan(dest ...any) error
}

func scanEnvironment(sc scanner) (*Environment, error) {
	var e Environment
	var isDefault int
	err := sc.Scan(&e.ID, &e.Name, &e.Description, &e.Type, &e.Host, &e.SocketPath,
		&e.TLSCa, &e.TLSCert, &e.TLSKey, &e.SSHUser, &e.SSHPassword, &e.SSHKey,
		&isDefault, &e.Status, &e.StatusMessage, &e.CreatedBy, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, err
	}
	e.IsDefault = isDefault == 1
	return &e, nil
}

// ---------------- 资源授权 ----------------

// AccessibleEnvironmentLevels 当前用户可见环境及最高访问级别。
func (s *Store) AccessibleEnvironmentLevels(ctx context.Context, userID string, roleIDs []string) (map[string]int, error) {
	out := make(map[string]int)
	args := []any{"user", userID}
	query := `SELECT environment_id, MAX(access_level) FROM docker_environment_grant
		WHERE (subject_type = ? AND subject_id = ?)`
	if len(roleIDs) > 0 {
		query += ` OR (subject_type = 'role' AND subject_id IN (` + placeholders(len(roleIDs)) + `))`
		for _, rid := range roleIDs {
			args = append(args, rid)
		}
	}
	query += ` GROUP BY environment_id`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var envID string
		var level int
		if err := rows.Scan(&envID, &level); err != nil {
			return nil, err
		}
		out[envID] = level
	}
	return out, rows.Err()
}

// GrantsOfEnvironment 环境授权列表。
func (s *Store) GrantsOfEnvironment(ctx context.Context, envID string) ([]Grant, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, environment_id, subject_type, subject_id, access_level, created_by, created_at
		FROM docker_environment_grant WHERE environment_id = ? ORDER BY subject_type, created_at`, envID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Grant, 0, 4)
	for rows.Next() {
		var g Grant
		if err := rows.Scan(&g.ID, &g.EnvironmentID, &g.SubjectType, &g.SubjectID, &g.AccessLevel, &g.CreatedBy, &g.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// SetGrants 覆盖式保存某环境的授权（一个事务内完成）。
func (s *Store) SetGrants(ctx context.Context, envID string, grants []Grant, operator string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM docker_environment_grant WHERE environment_id = ?`, envID); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO docker_environment_grant
		(id, environment_id, subject_type, subject_id, access_level, created_by, created_at)
		VALUES (?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	ts := now()
	for _, g := range grants {
		if g.AccessLevel < 1 || g.AccessLevel > 3 {
			continue
		}
		if g.SubjectType != "role" && g.SubjectType != "user" {
			continue
		}
		if _, err := stmt.ExecContext(ctx, newID(), envID, g.SubjectType, g.SubjectID, g.AccessLevel, operator, ts); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---------------- Stack ----------------

const stackColumns = `id, environment_id, name, type, compose_content, env_content, status, status_message,
 version, last_deployed_at, created_by, updated_by, created_at, updated_at`

// Stacks 某环境下的 Stack；envID 为空返回全部（超级管理员导出用）。
func (s *Store) Stacks(ctx context.Context, envID string) ([]Stack, error) {
	query := `SELECT ` + stackColumns + ` FROM docker_stack`
	args := []any{}
	if envID != "" {
		query += ` WHERE environment_id = ?`
		args = append(args, envID)
	}
	query += ` ORDER BY updated_at DESC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Stack, 0, 8)
	for rows.Next() {
		st, err := scanStack(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *st)
	}
	return out, rows.Err()
}

// Stack 按 ID 查询。
func (s *Store) Stack(ctx context.Context, id string) (*Stack, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+stackColumns+` FROM docker_stack WHERE id = ?`, id)
	st, err := scanStack(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return st, err
}

// StackByName 按环境 + 名称查询（部署前查重）。
func (s *Store) StackByName(ctx context.Context, envID, name string) (*Stack, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+stackColumns+` FROM docker_stack WHERE environment_id = ? AND name = ?`, envID, name)
	st, err := scanStack(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return st, err
}

func scanStack(sc scanner) (*Stack, error) {
	var st Stack
	var lastDeployed sql.NullString
	err := sc.Scan(&st.ID, &st.EnvironmentID, &st.Name, &st.Type, &st.ComposeContent, &st.EnvContent,
		&st.Status, &st.StatusMessage, &st.Version, &lastDeployed, &st.CreatedBy, &st.UpdatedBy, &st.CreatedAt, &st.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if lastDeployed.Valid {
		v := lastDeployed.String
		st.LastDeployedAt = &v
	}
	return &st, nil
}

// CreateStack 新增 Stack 并写入首版历史（事务）。
func (s *Store) CreateStack(ctx context.Context, st *Stack, operator, message string) error {
	if st.ID == "" {
		st.ID = newID()
	}
	ts := now()
	st.CreatedAt, st.UpdatedAt = ts, ts
	if st.Version <= 0 {
		st.Version = 1
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO docker_stack (`+stackColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		st.ID, st.EnvironmentID, st.Name, st.Type, st.ComposeContent, st.EnvContent, st.Status, st.StatusMessage,
		st.Version, st.LastDeployedAt, st.CreatedBy, st.UpdatedBy, st.CreatedAt, st.UpdatedAt)
	if err != nil {
		return err
	}
	rev := &Revision{
		ID: newID(), StackID: st.ID, Version: st.Version, ComposeContent: st.ComposeContent,
		EnvContent: st.EnvContent, Message: message, CreatedBy: operator, CreatedAt: ts,
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO docker_stack_revision
		(id, stack_id, version, compose_content, env_content, message, created_by, created_at)
		VALUES (?,?,?,?,?,?,?,?)`,
		rev.ID, rev.StackID, rev.Version, rev.ComposeContent, rev.EnvContent, rev.Message, rev.CreatedBy, rev.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

// SaveStackContent 保存新的 compose/.env 内容并写入历史版本（事务）。
func (s *Store) SaveStackContent(ctx context.Context, st *Stack, operator, message string) error {
	st.UpdatedAt = now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE docker_stack SET type=?, compose_content=?, env_content=?, version=?, updated_by=?, updated_at=? WHERE id=?`,
		st.Type, st.ComposeContent, st.EnvContent, st.Version, st.UpdatedBy, st.UpdatedAt, st.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	rev := &Revision{
		ID: newID(), StackID: st.ID, Version: st.Version, ComposeContent: st.ComposeContent,
		EnvContent: st.EnvContent, Message: message, CreatedBy: operator, CreatedAt: st.UpdatedAt,
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO docker_stack_revision
		(id, stack_id, version, compose_content, env_content, message, created_by, created_at)
		VALUES (?,?,?,?,?,?,?,?)`,
		rev.ID, rev.StackID, rev.Version, rev.ComposeContent, rev.EnvContent, rev.Message, rev.CreatedBy, rev.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

// SetStackStatus 更新部署状态（不产生新版本）。
func (s *Store) SetStackStatus(ctx context.Context, id, status, message string, deployedAt *string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE docker_stack SET status=?, status_message=?, last_deployed_at=COALESCE(?, last_deployed_at), updated_at=? WHERE id=?`,
		status, message, deployedAt, now(), id)
	return err
}

// DeleteStack 删除 Stack（级联删除版本与操作记录）。
func (s *Store) DeleteStack(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM docker_stack WHERE id = ?`, id)
	return err
}

// Revisions Stack 历史版本（新→旧）。
func (s *Store) Revisions(ctx context.Context, stackID string) ([]Revision, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, stack_id, version, compose_content, env_content, message, created_by, created_at
		FROM docker_stack_revision WHERE stack_id = ? ORDER BY version DESC`, stackID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Revision, 0, 8)
	for rows.Next() {
		var r Revision
		if err := rows.Scan(&r.ID, &r.StackID, &r.Version, &r.ComposeContent, &r.EnvContent, &r.Message, &r.CreatedBy, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Revision 指定版本。
func (s *Store) Revision(ctx context.Context, stackID string, version int) (*Revision, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, stack_id, version, compose_content, env_content, message, created_by, created_at
		FROM docker_stack_revision WHERE stack_id = ? AND version = ?`, stackID, version)
	var r Revision
	err := row.Scan(&r.ID, &r.StackID, &r.Version, &r.ComposeContent, &r.EnvContent, &r.Message, &r.CreatedBy, &r.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// Activities Stack 操作记录（最新 100 条）。
func (s *Store) Activities(ctx context.Context, stackID string, limit int) ([]Activity, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, stack_id, action, status, message, operator, created_at
		FROM docker_stack_activity WHERE stack_id = ? ORDER BY created_at DESC LIMIT ?`, stackID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Activity, 0, limit)
	for rows.Next() {
		var a Activity
		if err := rows.Scan(&a.ID, &a.StackID, &a.Action, &a.Status, &a.Message, &a.Operator, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AddActivity 记录一次部署/停止/回滚操作。
func (s *Store) AddActivity(ctx context.Context, stackID, action, status, message, operator string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO docker_stack_activity (id, stack_id, action, status, message, operator, created_at)
		VALUES (?,?,?,?,?,?,?)`, newID(), stackID, action, status, message, operator, now())
	return err
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// splitSQL 按分号拆分建表脚本（注释行忽略）。
func splitSQL(script string) []string {
	parts := strings.Split(script, ";")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		onlyComment := true
		for _, line := range strings.Split(p, "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "--") {
				onlyComment = false
				break
			}
		}
		if !onlyComment {
			out = append(out, p)
		}
	}
	return out
}
