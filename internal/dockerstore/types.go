package dockerstore

// Environment Docker 环境（连接端点）。
//
// 敏感字段在落库前由上层加密：TLSCa/TLSCert/TLSKey/SSHPassword/SSHKey。
// 接口出参绝不要直接序列化本结构，应使用 service 层组装的安全 DTO。
type Environment struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	Type          string `json:"type"` // local | tcp | tls | ssh
	Host          string `json:"host"`
	SocketPath    string `json:"socketPath"`
	TLSCa         string `json:"-"`
	TLSCert       string `json:"-"`
	TLSKey        string `json:"-"`
	SSHUser       string `json:"sshUser"`
	SSHPassword   string `json:"-"`
	SSHKey        string `json:"-"`
	IsDefault     bool   `json:"isDefault"`
	Status        string `json:"status"`
	StatusMessage string `json:"statusMessage"`
	CreatedBy     string `json:"createdBy"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

// HasTLS 是否配置了 TLS 客户端证书。
func (e *Environment) HasTLS() bool { return e.TLSCa != "" || e.TLSCert != "" || e.TLSKey != "" }

// HasSSHCredential 是否配置了 SSH 密码或私钥。
func (e *Environment) HasSSHCredential() bool { return e.SSHPassword != "" || e.SSHKey != "" }

// Grant 环境资源授权：把环境按「只读 / 操作 / 管理」级别授权给角色或用户。
type Grant struct {
	ID            string `json:"id"`
	EnvironmentID string `json:"environmentId"`
	SubjectType   string `json:"subjectType"` // role | user
	SubjectID     string `json:"subjectId"`
	AccessLevel   int    `json:"accessLevel"` // 1 read 2 operate 3 manage
	CreatedBy     string `json:"createdBy"`
	CreatedAt     string `json:"createdAt"`
}

// Stack Stack 定义（compose 文件 + .env 永久保存在 SQLite）。
type Stack struct {
	ID             string  `json:"id"`
	EnvironmentID  string  `json:"environmentId"`
	Name           string  `json:"name"`
	Type           string  `json:"type"` // compose | swarm
	ComposeContent string  `json:"composeContent"`
	EnvContent     string  `json:"envContent"`
	Status         string  `json:"status"` // active | inactive | error | deploying
	StatusMessage  string  `json:"statusMessage"`
	Version        int     `json:"version"`
	LastDeployedAt *string `json:"lastDeployedAt"`
	CreatedBy      string  `json:"createdBy"`
	UpdatedBy      string  `json:"updatedBy"`
	CreatedAt      string  `json:"createdAt"`
	UpdatedAt      string  `json:"updatedAt"`
}

// Revision Stack 历史版本（每次保存或回滚产生一条，支持一键回滚）。
type Revision struct {
	ID             string `json:"id"`
	StackID        string `json:"stackId"`
	Version        int    `json:"version"`
	ComposeContent string `json:"composeContent"`
	EnvContent     string `json:"envContent"`
	Message        string `json:"message"`
	CreatedBy      string `json:"createdBy"`
	CreatedAt      string `json:"createdAt"`
}

// Activity Stack 操作记录（部署/停止/回滚，含失败原因）。
type Activity struct {
	ID        string `json:"id"`
	StackID   string `json:"stackId"`
	Action    string `json:"action"`
	Status    string `json:"status"` // success | failed
	Message   string `json:"message"`
	Operator  string `json:"operator"`
	CreatedAt string `json:"createdAt"`
}
