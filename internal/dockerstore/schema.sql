-- Docker 管理模块 SQLite 结构（独立数据库，便于整库备份/迁移）
-- 约定：所有时间使用 RFC3339 文本；敏感字段（TLS 私钥、SSH 密码）由上层加密后写入。

CREATE TABLE IF NOT EXISTS docker_environment (
  id             TEXT PRIMARY KEY,
  name           TEXT NOT NULL,
  description    TEXT NOT NULL DEFAULT '',
  type           TEXT NOT NULL,                 -- local | tcp | tls | ssh
  host           TEXT NOT NULL DEFAULT '',
  socket_path    TEXT NOT NULL DEFAULT '',
  tls_ca         TEXT NOT NULL DEFAULT '',
  tls_cert       TEXT NOT NULL DEFAULT '',
  tls_key        TEXT NOT NULL DEFAULT '',
  ssh_user       TEXT NOT NULL DEFAULT '',
  ssh_password   TEXT NOT NULL DEFAULT '',
  ssh_key        TEXT NOT NULL DEFAULT '',
  is_default     INTEGER NOT NULL DEFAULT 0,
  status         TEXT NOT NULL DEFAULT 'unknown', -- online | offline | unknown
  status_message TEXT NOT NULL DEFAULT '',
  created_by     TEXT NOT NULL DEFAULT '',
  created_at     TEXT NOT NULL,
  updated_at     TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_docker_env_name ON docker_environment(name);

CREATE TABLE IF NOT EXISTS docker_environment_grant (
  id             TEXT PRIMARY KEY,
  environment_id TEXT NOT NULL,
  subject_type   TEXT NOT NULL,                 -- role | user
  subject_id     TEXT NOT NULL,
  access_level   INTEGER NOT NULL DEFAULT 1,    -- 1 只读 2 操作 3 管理
  created_by     TEXT NOT NULL DEFAULT '',
  created_at     TEXT NOT NULL,
  UNIQUE(environment_id, subject_type, subject_id),
  FOREIGN KEY (environment_id) REFERENCES docker_environment(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_docker_grant_subject ON docker_environment_grant(subject_type, subject_id);

CREATE TABLE IF NOT EXISTS docker_stack (
  id               TEXT PRIMARY KEY,
  environment_id   TEXT NOT NULL,
  name             TEXT NOT NULL,
  type             TEXT NOT NULL DEFAULT 'compose', -- compose | swarm
  compose_content  TEXT NOT NULL DEFAULT '',
  env_content      TEXT NOT NULL DEFAULT '',
  status           TEXT NOT NULL DEFAULT 'inactive', -- active | inactive | error | deploying
  status_message   TEXT NOT NULL DEFAULT '',
  version          INTEGER NOT NULL DEFAULT 1,
  last_deployed_at TEXT,
  created_by       TEXT NOT NULL DEFAULT '',
  updated_by       TEXT NOT NULL DEFAULT '',
  created_at       TEXT NOT NULL,
  updated_at       TEXT NOT NULL,
  UNIQUE(environment_id, name),
  FOREIGN KEY (environment_id) REFERENCES docker_environment(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_docker_stack_env ON docker_stack(environment_id);

CREATE TABLE IF NOT EXISTS docker_stack_revision (
  id              TEXT PRIMARY KEY,
  stack_id        TEXT NOT NULL,
  version         INTEGER NOT NULL,
  compose_content TEXT NOT NULL DEFAULT '',
  env_content     TEXT NOT NULL DEFAULT '',
  message         TEXT NOT NULL DEFAULT '',
  created_by      TEXT NOT NULL DEFAULT '',
  created_at      TEXT NOT NULL,
  UNIQUE(stack_id, version),
  FOREIGN KEY (stack_id) REFERENCES docker_stack(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_docker_revision_stack ON docker_stack_revision(stack_id, version DESC);

CREATE TABLE IF NOT EXISTS docker_stack_activity (
  id         TEXT PRIMARY KEY,
  stack_id   TEXT NOT NULL,
  action     TEXT NOT NULL,
  status     TEXT NOT NULL,                   -- success | failed
  message    TEXT NOT NULL DEFAULT '',
  operator   TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  FOREIGN KEY (stack_id) REFERENCES docker_stack(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_docker_activity_stack ON docker_stack_activity(stack_id, created_at DESC);

PRAGMA user_version = 1;