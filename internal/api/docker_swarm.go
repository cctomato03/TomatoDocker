package api

import (
	"github.com/gin-gonic/gin"

	"web-basic/internal/docker"
	"web-basic/internal/principal"
	"web-basic/pkg/response"
)

// DockerSwarmState 集群状态。
func (h *Handler) DockerSwarmState(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	state, err := h.Svc.DockerSwarmState(c.Request.Context(), principal.Must(c), envID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, state)
}

// DockerSwarmInit 初始化 Swarm。
func (h *Handler) DockerSwarmInit(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	var req struct {
		AdvertiseAddr string `json:"advertiseAddr"`
		ListenAddr    string `json:"listenAddr"`
	}
	if c.Request.ContentLength > 0 {
		if !bindJSON(c, &req) {
			return
		}
	}
	nodeID, err := h.Svc.DockerSwarmInit(c.Request.Context(), principal.Must(c), envID, req.AdvertiseAddr, req.ListenAddr)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, gin.H{"nodeId": nodeID})
}

// DockerSwarmLeave 退出 Swarm。
func (h *Handler) DockerSwarmLeave(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	force := c.DefaultQuery("force", "false") == "true"
	if err := h.Svc.DockerSwarmLeave(c.Request.Context(), principal.Must(c), envID, force); err != nil {
		response.Fail(c, err)
		return
	}
	response.Ok(c)
}

// DockerNodes 节点列表。
func (h *Handler) DockerNodes(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	list, err := h.Svc.DockerNodes(c.Request.Context(), principal.Must(c), envID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, list)
}

// DockerUpdateNode 修改节点。
func (h *Handler) DockerUpdateNode(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	nodeID, ok := pathString(c, "nodeId")
	if !ok {
		return
	}
	var req struct {
		Role         string            `json:"role"`
		Availability string            `json:"availability"`
		Labels       map[string]string `json:"labels"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if err := h.Svc.DockerUpdateNode(c.Request.Context(), principal.Must(c), envID, nodeID, req.Role, req.Availability, req.Labels); err != nil {
		response.Fail(c, err)
		return
	}
	response.Ok(c)
}

// DockerRemoveNode 移除节点。
func (h *Handler) DockerRemoveNode(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	nodeID, ok := pathString(c, "nodeId")
	if !ok {
		return
	}
	force := c.DefaultQuery("force", "false") == "true"
	if err := h.Svc.DockerRemoveNode(c.Request.Context(), principal.Must(c), envID, nodeID, force); err != nil {
		response.Fail(c, err)
		return
	}
	response.Ok(c)
}

// DockerServices 服务列表。
func (h *Handler) DockerServices(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	list, err := h.Svc.DockerServices(c.Request.Context(), principal.Must(c), envID, c.Query("keyword"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, list)
}

// DockerServiceDetail 服务详情。
func (h *Handler) DockerServiceDetail(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	sid, ok := pathString(c, "serviceId")
	if !ok {
		return
	}
	out, err := h.Svc.DockerServiceDetail(c.Request.Context(), principal.Must(c), envID, sid)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, out)
}

// DockerCreateService 创建服务。
func (h *Handler) DockerCreateService(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	var req docker.CreateServiceRequest
	if !bindJSON(c, &req) {
		return
	}
	sid, err := h.Svc.DockerCreateService(c.Request.Context(), principal.Must(c), envID, &req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, gin.H{"id": sid})
}

// DockerUpdateService 修改服务。
func (h *Handler) DockerUpdateService(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	sid, ok := pathString(c, "serviceId")
	if !ok {
		return
	}
	var req docker.UpdateServiceRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.Svc.DockerUpdateService(c.Request.Context(), principal.Must(c), envID, sid, &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.Ok(c)
}

// DockerScaleService 调整副本数。
func (h *Handler) DockerScaleService(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	sid, ok := pathString(c, "serviceId")
	if !ok {
		return
	}
	var req struct {
		Replicas uint64 `json:"replicas"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if err := h.Svc.DockerScaleService(c.Request.Context(), principal.Must(c), envID, sid, req.Replicas); err != nil {
		response.Fail(c, err)
		return
	}
	response.Ok(c)
}

// DockerRemoveService 删除服务。
func (h *Handler) DockerRemoveService(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	sid, ok := pathString(c, "serviceId")
	if !ok {
		return
	}
	if err := h.Svc.DockerRemoveService(c.Request.Context(), principal.Must(c), envID, sid); err != nil {
		response.Fail(c, err)
		return
	}
	response.Ok(c)
}

// DockerServiceLogs 服务日志。
func (h *Handler) DockerServiceLogs(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	sid, ok := pathString(c, "serviceId")
	if !ok {
		return
	}
	logs, err := h.Svc.DockerServiceLogs(c.Request.Context(), principal.Must(c), envID, sid, parseIntQuery(c, "tail", 200))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, gin.H{"logs": logs})
}

// DockerTasks 任务列表。
func (h *Handler) DockerTasks(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	list, err := h.Svc.DockerTasks(c.Request.Context(), principal.Must(c), envID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, list)
}

// DockerSecrets 密钥列表。
func (h *Handler) DockerSecrets(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	list, err := h.Svc.DockerSecrets(c.Request.Context(), principal.Must(c), envID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, list)
}

// DockerCreateSecret 创建密钥。
func (h *Handler) DockerCreateSecret(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	var req docker.CreateSecretRequest
	if !bindJSON(c, &req) {
		return
	}
	sid, err := h.Svc.DockerCreateSecret(c.Request.Context(), principal.Must(c), envID, &req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, gin.H{"id": sid})
}

// DockerRemoveSecret 删除密钥。
func (h *Handler) DockerRemoveSecret(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	sid, ok := pathString(c, "secretId")
	if !ok {
		return
	}
	if err := h.Svc.DockerRemoveSecret(c.Request.Context(), principal.Must(c), envID, sid); err != nil {
		response.Fail(c, err)
		return
	}
	response.Ok(c)
}

// DockerConfigs 配置列表。
func (h *Handler) DockerConfigs(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	list, err := h.Svc.DockerConfigs(c.Request.Context(), principal.Must(c), envID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, list)
}

// DockerCreateConfig 创建配置。
func (h *Handler) DockerCreateConfig(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	var req docker.CreateSecretRequest
	if !bindJSON(c, &req) {
		return
	}
	cid, err := h.Svc.DockerCreateConfig(c.Request.Context(), principal.Must(c), envID, &req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, gin.H{"id": cid})
}

// DockerRemoveConfig 删除配置。
func (h *Handler) DockerRemoveConfig(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	cid, ok := pathString(c, "configId")
	if !ok {
		return
	}
	if err := h.Svc.DockerRemoveConfig(c.Request.Context(), principal.Must(c), envID, cid); err != nil {
		response.Fail(c, err)
		return
	}
	response.Ok(c)
}
