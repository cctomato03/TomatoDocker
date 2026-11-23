// Package docker Docker Engine / Swarm API 封装（不含业务权限判断）。
package docker

// Info Docker 主机概要。
type Info struct {
	Name            string   `json:"name"`
	ServerVersion   string   `json:"serverVersion"`
	APIVersion      string   `json:"apiVersion"`
	OperatingSystem string   `json:"operatingSystem"`
	OSType          string   `json:"osType"`
	Architecture    string   `json:"architecture"`
	KernelVersion   string   `json:"kernelVersion"`
	NCPU            int      `json:"nCpu"`
	MemTotal        int64    `json:"memTotal"`
	Containers      int      `json:"containers"`
	Running         int      `json:"running"`
	Paused          int      `json:"paused"`
	Stopped         int      `json:"stopped"`
	Images          int      `json:"images"`
	Driver          string   `json:"driver"`
	DockerRootDir   string   `json:"dockerRootDir"`
	LoggingDriver   string   `json:"loggingDriver"`
	CgroupDriver    string   `json:"cgroupDriver"`
	SwarmActive     bool     `json:"swarmActive"`
	SwarmNodeID     string   `json:"swarmNodeId"`
	SwarmNodeAddr   string   `json:"swarmNodeAddr"`
	SwarmRole       string   `json:"swarmRole"`
	SwarmNodes      int      `json:"swarmNodes"`
	SwarmManagers   int      `json:"swarmManagers"`
	Warnings        []string `json:"warnings,omitempty"`
}

// DiskUsage 磁盘占用。
type DiskUsage struct {
	LayersSize  int64 `json:"layersSize"`
	ImagesSize  int64 `json:"imagesSize"`
	VolumesSize int64 `json:"volumesSize"`
	BuildCache  int64 `json:"buildCache"`
	Images      int   `json:"images"`
	Containers  int   `json:"containers"`
	Volumes     int   `json:"volumes"`
}

// Overview 环境概览（主机信息 + 资源计数 + 磁盘占用）。
type Overview struct {
	Info     Info      `json:"info"`
	Usage    DiskUsage `json:"usage"`
	Networks int       `json:"networks"`
	Stacks   int       `json:"stacks"`
	Services int       `json:"services"`
	Tasks    int       `json:"tasks"`
	Secrets  int       `json:"secrets"`
	Configs  int       `json:"configs"`
	Nodes    int       `json:"nodes"`
}

// Port 端口映射。
type Port struct {
	IP          string `json:"ip"`
	PrivatePort uint16 `json:"privatePort"`
	PublicPort  uint16 `json:"publicPort"`
	Type        string `json:"type"`
}

// Container 容器列表项。
type Container struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Names        []string          `json:"names"`
	Image        string            `json:"image"`
	ImageID      string            `json:"imageId"`
	Command      string            `json:"command"`
	Created      int64             `json:"created"`
	State        string            `json:"state"`
	Status       string            `json:"status"`
	Ports        []Port            `json:"ports"`
	Labels       map[string]string `json:"labels"`
	StackName    string            `json:"stackName"`
	ServiceName  string            `json:"serviceName"`
	ProjectName  string            `json:"projectName"`
	Health       string            `json:"health"`
	ComposeMode  bool              `json:"composeMode"`
	SwarmService bool              `json:"swarmService"`
}

// PortBinding 创建容器时的端口映射。
type PortBinding struct {
	HostIP        string `json:"hostIp"`
	HostPort      int    `json:"hostPort"`
	ContainerPort int    `json:"containerPort"`
	Protocol      string `json:"protocol"`
}

// VolumeBinding 创建容器时的挂载。
type VolumeBinding struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"readOnly"`
}

// CreateContainerRequest 创建容器入参（Web 表单）。
type CreateContainerRequest struct {
	Name          string            `json:"name"`
	Image         string            `json:"image" binding:"required"`
	Command       string            `json:"command"`
	Entrypoint    string            `json:"entrypoint"`
	Env           []string          `json:"env"`
	Ports         []PortBinding     `json:"ports"`
	Volumes       []VolumeBinding   `json:"volumes"`
	NetworkID     string            `json:"networkId"`
	RestartPolicy string            `json:"restartPolicy"`
	AutoStart     bool              `json:"autoStart"`
	Privileged    bool              `json:"privileged"`
	Tty           bool              `json:"tty"`
	Labels        map[string]string `json:"labels"`
}

// Image 镜像列表项。
type Image struct {
	ID          string            `json:"id"`
	ShortID     string            `json:"shortId"`
	RepoTags    []string          `json:"repoTags"`
	RepoDigests []string          `json:"repoDigests"`
	Created     int64             `json:"created"`
	Size        int64             `json:"size"`
	SharedSize  int64             `json:"sharedSize"`
	Containers  int64             `json:"containers"`
	Labels      map[string]string `json:"labels"`
}

// Network 网络列表项。
type Network struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Driver     string            `json:"driver"`
	Scope      string            `json:"scope"`
	Internal   bool              `json:"internal"`
	Attachable bool              `json:"attachable"`
	Ingress    bool              `json:"ingress"`
	EnableIPv6 bool              `json:"enableIpv6"`
	Subnets    []string          `json:"subnets"`
	Containers int               `json:"containers"`
	Labels     map[string]string `json:"labels"`
	Created    string            `json:"created"`
	StackName  string            `json:"stackName"`
}

// CreateNetworkRequest 创建网络入参。
type CreateNetworkRequest struct {
	Name       string            `json:"name" binding:"required"`
	Driver     string            `json:"driver"`
	Subnet     string            `json:"subnet"`
	Gateway    string            `json:"gateway"`
	Internal   bool              `json:"internal"`
	Attachable bool              `json:"attachable"`
	EnableIPv6 bool              `json:"enableIpv6"`
	Labels     map[string]string `json:"labels"`
}

// Volume 数据卷列表项。
type Volume struct {
	Name       string            `json:"name"`
	Driver     string            `json:"driver"`
	Mountpoint string            `json:"mountpoint"`
	Scope      string            `json:"scope"`
	CreatedAt  string            `json:"createdAt"`
	Labels     map[string]string `json:"labels"`
	StackName  string            `json:"stackName"`
	InUse      bool              `json:"inUse"`
}

// CreateVolumeRequest 创建数据卷入参。
type CreateVolumeRequest struct {
	Name       string            `json:"name" binding:"required"`
	Driver     string            `json:"driver"`
	DriverOpts map[string]string `json:"driverOpts"`
	Labels     map[string]string `json:"labels"`
}

// Node Swarm 节点。
type Node struct {
	ID            string            `json:"id"`
	Hostname      string            `json:"hostname"`
	Role          string            `json:"role"`
	Availability  string            `json:"availability"`
	Status        string            `json:"status"`
	Addr          string            `json:"addr"`
	Version       string            `json:"version"`
	PlatformOS    string            `json:"platformOs"`
	PlatformArch  string            `json:"platformArch"`
	NCPU          int64             `json:"nCpu"`
	MemTotal      int64             `json:"memTotal"`
	Leader        bool              `json:"leader"`
	ManagerStatus string            `json:"managerStatus"`
	Labels        map[string]string `json:"labels"`
	EngineLabels  map[string]string `json:"engineLabels"`
	CreatedAt     string            `json:"createdAt"`
	UpdatedAt     string            `json:"updatedAt"`
}

// ServicePort Swarm 服务发布端口。
type ServicePort struct {
	Name          string `json:"name"`
	Protocol      string `json:"protocol"`
	TargetPort    uint32 `json:"targetPort"`
	PublishedPort uint32 `json:"publishedPort"`
	PublishMode   string `json:"publishMode"`
}

// Service Swarm 服务。
type Service struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Image        string            `json:"image"`
	Mode         string            `json:"mode"`
	Replicas     uint64            `json:"replicas"`
	Running      uint64            `json:"running"`
	Ports        []ServicePort     `json:"ports"`
	StackName    string            `json:"stackName"`
	Labels       map[string]string `json:"labels"`
	Command      string            `json:"command"`
	CreatedAt    string            `json:"createdAt"`
	UpdatedAt    string            `json:"updatedAt"`
	UpdateStatus string            `json:"updateStatus"`
}

// CreateServiceRequest 创建 Swarm 服务入参。
type CreateServiceRequest struct {
	Name          string            `json:"name" binding:"required"`
	Image         string            `json:"image" binding:"required"`
	Replicas      uint64            `json:"replicas"`
	Command       string            `json:"command"`
	Env           []string          `json:"env"`
	Ports         []PortBinding     `json:"ports"`
	NetworkID     string            `json:"networkId"`
	Labels        map[string]string `json:"labels"`
	Constraints   []string          `json:"constraints"`
	RestartPolicy string            `json:"restartPolicy"`
}

// UpdateServiceRequest 修改服务（镜像/副本数/环境变量/端口/约束）。
type UpdateServiceRequest struct {
	Image       string            `json:"image"`
	Replicas    *uint64           `json:"replicas"`
	Env         []string          `json:"env"`
	Ports       []PortBinding     `json:"ports"`
	Command     string            `json:"command"`
	Constraints []string          `json:"constraints"`
	NetworkID   string            `json:"networkId"`
	Labels      map[string]string `json:"labels"`
	Force       bool              `json:"force"`
}

// Task Swarm 任务。
type Task struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ServiceID    string `json:"serviceId"`
	ServiceName  string `json:"serviceName"`
	NodeID       string `json:"nodeId"`
	NodeName     string `json:"nodeName"`
	Slot         int    `json:"slot"`
	State        string `json:"state"`
	DesiredState string `json:"desiredState"`
	Message      string `json:"message"`
	Err          string `json:"err"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

// Secret Swarm 密钥（Data 永远不返回）。
type Secret struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Labels    map[string]string `json:"labels"`
	Driver    string            `json:"driver"`
	CreatedAt string            `json:"createdAt"`
	UpdatedAt string            `json:"updatedAt"`
	StackName string            `json:"stackName"`
}

// ConfigObj Swarm 配置（Data 永远不返回）。
type ConfigObj struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Labels    map[string]string `json:"labels"`
	CreatedAt string            `json:"createdAt"`
	UpdatedAt string            `json:"updatedAt"`
	StackName string            `json:"stackName"`
}

// CreateSecretRequest 创建密钥/配置入参。
type CreateSecretRequest struct {
	Name   string            `json:"name" binding:"required"`
	Data   string            `json:"data" binding:"required"`
	Labels map[string]string `json:"labels"`
}

// SwarmState 集群状态。
type SwarmState struct {
	Active           bool              `json:"active"`
	NodeID           string            `json:"nodeId"`
	NodeAddr         string            `json:"nodeAddr"`
	LocalNodeState   string            `json:"localNodeState"`
	ControlAvailable bool              `json:"controlAvailable"`
	Error            string            `json:"error"`
	ClusterID        string            `json:"clusterId"`
	Name             string            `json:"name"`
	Labels           map[string]string `json:"labels"`
	Nodes            int               `json:"nodes"`
	Managers         int               `json:"managers"`
	CreatedAt        string            `json:"createdAt"`
	UpdatedAt        string            `json:"updatedAt"`
}

// ContainerStats 容器资源快照。
type ContainerStats struct {
	CPUPercent float64 `json:"cpuPercent"`
	MemUsage   int64   `json:"memUsage"`
	MemLimit   int64   `json:"memLimit"`
	MemPercent float64 `json:"memPercent"`
	NetRx      int64   `json:"netRx"`
	NetTx      int64   `json:"netTx"`
	BlockRead  int64   `json:"blockRead"`
	BlockWrite int64   `json:"blockWrite"`
	Pids       int64   `json:"pids"`
}

// StackRuntime Stack 运行态摘要（来自 Docker 标签聚合，不落库）。
type StackRuntime struct {
	Status            string            `json:"status"`
	ContainersTotal   int               `json:"containersTotal"`
	ContainersRunning int               `json:"containersRunning"`
	ServicesTotal     int               `json:"servicesTotal"`
	ServicesRunning   int               `json:"servicesRunning"`
	Resources         []RuntimeResource `json:"resources"`
}

// RuntimeResource 运行态资源（容器或服务）。
type RuntimeResource struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Kind   string `json:"kind"` // container | service
	State  string `json:"state"`
	Image  string `json:"image"`
	Detail string `json:"detail"`
}
