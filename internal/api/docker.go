package api

import (
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"web-basic/internal/docker"
	"web-basic/internal/principal"
	"web-basic/internal/service"
	"web-basic/pkg/errs"
	"web-basic/pkg/response"
)

func pathString(c *gin.Context, name string) (string, bool) {
	v := strings.TrimSpace(c.Param(name))
	if v == "" {
		response.Fail(c, errs.Invalid("缺少路径参数 "+name))
		return "", false
	}
	return v, true
}

/* ------------------------------ 环境 ------------------------------ */

// DockerEnvironments 环境列表。
func (h *Handler) DockerEnvironments(c *gin.Context) {
	list, err := h.Svc.ListDockerEnvironments(c.Request.Context(), principal.Must(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, list)
}

// DockerEnvironment 环境详情。
func (h *Handler) DockerEnvironment(c *gin.Context) {
	id, ok := pathString(c, "id")
	if !ok {
		return
	}
	env, err := h.Svc.GetDockerEnvironment(c.Request.Context(), principal.Must(c), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, env)
}

// DockerCreateEnvironment 新增环境。
func (h *Handler) DockerCreateEnvironment(c *gin.Context) {
	var req service.DockerEnvironmentReq
	if !bindJSON(c, &req) {
		return
	}
	id, err := h.Svc.CreateDockerEnvironment(c.Request.Context(), principal.Must(c), &req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, gin.H{"id": id})
}

// DockerUpdateEnvironment 修改环境。
func (h *Handler) DockerUpdateEnvironment(c *gin.Context) {
	id, ok := pathString(c, "id")
	if !ok {
		return
	}
	var req service.DockerEnvironmentReq
	if !bindJSON(c, &req) {
		return
	}
	if err := h.Svc.UpdateDockerEnvironment(c.Request.Context(), principal.Must(c), id, &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.Ok(c)
}

// DockerDeleteEnvironment 删除环境。
func (h *Handler) DockerDeleteEnvironment(c *gin.Context) {
	id, ok := pathString(c, "id")
	if !ok {
		return
	}
	if err := h.Svc.DeleteDockerEnvironment(c.Request.Context(), principal.Must(c), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.Ok(c)
}

// DockerTestEnvironment 测试环境连接。
func (h *Handler) DockerTestEnvironment(c *gin.Context) {
	id, ok := pathString(c, "id")
	if !ok {
		return
	}
	info, err := h.Svc.TestDockerEnvironment(c.Request.Context(), principal.Must(c), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, info)
}

/* ------------------------------ 资源授权 ------------------------------ */

// DockerGrantOptions 授权可选主体。
func (h *Handler) DockerGrantOptions(c *gin.Context) {
	list, err := h.Svc.DockerGrantOptions(c.Request.Context(), principal.Must(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, list)
}

// DockerGrants 环境授权列表。
func (h *Handler) DockerGrants(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	list, err := h.Svc.DockerGrantsOfEnvironment(c.Request.Context(), principal.Must(c), envID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, list)
}

// DockerSetGrants 保存环境授权。
func (h *Handler) DockerSetGrants(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	var req service.DockerGrantReq
	if !bindJSON(c, &req) {
		return
	}
	if err := h.Svc.SetDockerGrants(c.Request.Context(), principal.Must(c), envID, &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.Ok(c)
}

// DockerOverview 环境概览。
func (h *Handler) DockerOverview(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	ov, err := h.Svc.DockerOverview(c.Request.Context(), principal.Must(c), envID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, ov)
}

/* ------------------------------ 容器 ------------------------------ */

// DockerContainers 容器列表。
func (h *Handler) DockerContainers(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	all := c.DefaultQuery("all", "true") == "true"
	list, err := h.Svc.DockerContainers(c.Request.Context(), principal.Must(c), envID, all, c.Query("keyword"), c.Query("state"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, list)
}

// DockerContainerInspect 容器详情。
func (h *Handler) DockerContainerInspect(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	cid, ok := pathString(c, "cid")
	if !ok {
		return
	}
	detail, err := h.Svc.DockerContainerInspect(c.Request.Context(), principal.Must(c), envID, cid)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, detail)
}

// DockerContainerAction 容器生命周期操作。
func (h *Handler) DockerContainerAction(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	cid, ok := pathString(c, "cid")
	if !ok {
		return
	}
	var req struct {
		Action string `json:"action" binding:"required"`
		Name   string `json:"name"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if err := h.Svc.DockerContainerAction(c.Request.Context(), principal.Must(c), envID, cid, req.Action, req.Name); err != nil {
		response.Fail(c, err)
		return
	}
	response.Ok(c)
}

// DockerContainerRemove 删除容器。
func (h *Handler) DockerContainerRemove(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	cid, ok := pathString(c, "cid")
	if !ok {
		return
	}
	force := c.DefaultQuery("force", "false") == "true"
	volumes := c.DefaultQuery("volumes", "false") == "true"
	if err := h.Svc.DockerContainerRemove(c.Request.Context(), principal.Must(c), envID, cid, force, volumes); err != nil {
		response.Fail(c, err)
		return
	}
	response.Ok(c)
}

// DockerCreateContainer 创建容器。
func (h *Handler) DockerCreateContainer(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	var req docker.CreateContainerRequest
	if !bindJSON(c, &req) {
		return
	}
	cid, err := h.Svc.DockerCreateContainer(c.Request.Context(), principal.Must(c), envID, &req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, gin.H{"id": cid})
}

// DockerContainerLogs 容器日志。
func (h *Handler) DockerContainerLogs(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	cid, ok := pathString(c, "cid")
	if !ok {
		return
	}
	tail := parseIntQuery(c, "tail", 200)
	timestamps := c.DefaultQuery("timestamps", "true") == "true"
	logs, err := h.Svc.DockerContainerLogs(c.Request.Context(), principal.Must(c), envID, cid, tail, timestamps)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, gin.H{"logs": logs})
}

// DockerContainerStats 容器资源统计。
func (h *Handler) DockerContainerStats(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	cid, ok := pathString(c, "cid")
	if !ok {
		return
	}
	stats, err := h.Svc.DockerContainerStats(c.Request.Context(), principal.Must(c), envID, cid)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, stats)
}

// DockerContainerExec 在容器内执行命令。
func (h *Handler) DockerContainerExec(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	cid, ok := pathString(c, "cid")
	if !ok {
		return
	}
	var req struct {
		Command string `json:"command" binding:"required"`
	}
	if !bindJSON(c, &req) {
		return
	}
	out, code, err := h.Svc.DockerContainerExec(c.Request.Context(), principal.Must(c), envID, cid, req.Command)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, gin.H{"output": out, "exitCode": code})
}

// DockerContainersPrune 清理已停止容器。
func (h *Handler) DockerContainersPrune(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	n, space, err := h.Svc.DockerContainersPrune(c.Request.Context(), principal.Must(c), envID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, gin.H{"deleted": n, "spaceReclaimed": space})
}

/* ------------------------------ 镜像 ------------------------------ */

// DockerImages 镜像列表。
func (h *Handler) DockerImages(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	all := c.DefaultQuery("all", "false") == "true"
	list, err := h.Svc.DockerImages(c.Request.Context(), principal.Must(c), envID, all, c.Query("keyword"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, list)
}

// DockerImageInspect 镜像详情。
func (h *Handler) DockerImageInspect(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	id, ok := pathString(c, "imageId")
	if !ok {
		return
	}
	out, err := h.Svc.DockerImageInspect(c.Request.Context(), principal.Must(c), envID, id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, out)
}

// DockerPullImage 拉取镜像（NDJSON 流式进度）。
func (h *Handler) DockerPullImage(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	ref := strings.TrimSpace(c.Query("ref"))
	if ref == "" {
		response.Fail(c, errs.Invalid("镜像名称不能为空"))
		return
	}
	rc, err := h.Svc.DockerPullImage(c.Request.Context(), principal.Must(c), envID, ref)
	if err != nil {
		response.Fail(c, err)
		return
	}
	defer rc.Close()
	c.Header("Content-Type", "application/x-ndjson")
	c.Header("Cache-Control", "no-cache")
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, rc)
}

// DockerImageRemove 删除镜像。
func (h *Handler) DockerImageRemove(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	id, ok := pathString(c, "imageId")
	if !ok {
		return
	}
	force := c.DefaultQuery("force", "true") == "true"
	if err := h.Svc.DockerImageRemove(c.Request.Context(), principal.Must(c), envID, id, force); err != nil {
		response.Fail(c, err)
		return
	}
	response.Ok(c)
}

// DockerImagesPrune 清理悬空镜像。
func (h *Handler) DockerImagesPrune(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	n, space, err := h.Svc.DockerImagesPrune(c.Request.Context(), principal.Must(c), envID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, gin.H{"deleted": n, "spaceReclaimed": space})
}

/* ------------------------------ 网络 ------------------------------ */

// DockerNetworks 网络列表。
func (h *Handler) DockerNetworks(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	list, err := h.Svc.DockerNetworks(c.Request.Context(), principal.Must(c), envID, c.Query("keyword"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, list)
}

// DockerNetworkInspect 网络详情。
func (h *Handler) DockerNetworkInspect(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	nid, ok := pathString(c, "networkId")
	if !ok {
		return
	}
	out, err := h.Svc.DockerNetworkInspect(c.Request.Context(), principal.Must(c), envID, nid)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, out)
}

// DockerCreateNetwork 创建网络。
func (h *Handler) DockerCreateNetwork(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	var req docker.CreateNetworkRequest
	if !bindJSON(c, &req) {
		return
	}
	id, err := h.Svc.DockerCreateNetwork(c.Request.Context(), principal.Must(c), envID, &req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, gin.H{"id": id})
}

// DockerRemoveNetwork 删除网络。
func (h *Handler) DockerRemoveNetwork(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	nid, ok := pathString(c, "networkId")
	if !ok {
		return
	}
	if err := h.Svc.DockerRemoveNetwork(c.Request.Context(), principal.Must(c), envID, nid); err != nil {
		response.Fail(c, err)
		return
	}
	response.Ok(c)
}

// DockerConnectNetwork 容器接入网络。
func (h *Handler) DockerConnectNetwork(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	nid, ok := pathString(c, "networkId")
	if !ok {
		return
	}
	var req struct {
		ContainerID string   `json:"containerId" binding:"required"`
		Aliases     []string `json:"aliases"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if err := h.Svc.DockerConnectNetwork(c.Request.Context(), principal.Must(c), envID, nid, req.ContainerID, req.Aliases); err != nil {
		response.Fail(c, err)
		return
	}
	response.Ok(c)
}

// DockerDisconnectNetwork 容器断开网络。
func (h *Handler) DockerDisconnectNetwork(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	nid, ok := pathString(c, "networkId")
	if !ok {
		return
	}
	var req struct {
		ContainerID string `json:"containerId" binding:"required"`
		Force       bool   `json:"force"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if err := h.Svc.DockerDisconnectNetwork(c.Request.Context(), principal.Must(c), envID, nid, req.ContainerID, req.Force); err != nil {
		response.Fail(c, err)
		return
	}
	response.Ok(c)
}

// DockerNetworksPrune 清理未使用网络。
func (h *Handler) DockerNetworksPrune(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	n, err := h.Svc.DockerNetworksPrune(c.Request.Context(), principal.Must(c), envID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, gin.H{"deleted": n})
}

/* ------------------------------ 数据卷 ------------------------------ */

// DockerVolumes 数据卷列表。
func (h *Handler) DockerVolumes(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	list, err := h.Svc.DockerVolumes(c.Request.Context(), principal.Must(c), envID, c.Query("keyword"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, list)
}

// DockerVolumeInspect 数据卷详情。
func (h *Handler) DockerVolumeInspect(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	name, ok := pathString(c, "name")
	if !ok {
		return
	}
	out, err := h.Svc.DockerVolumeInspect(c.Request.Context(), principal.Must(c), envID, name)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, out)
}

// DockerCreateVolume 创建数据卷。
func (h *Handler) DockerCreateVolume(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	var req docker.CreateVolumeRequest
	if !bindJSON(c, &req) {
		return
	}
	out, err := h.Svc.DockerCreateVolume(c.Request.Context(), principal.Must(c), envID, &req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, out)
}

// DockerRemoveVolume 删除数据卷。
func (h *Handler) DockerRemoveVolume(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	name, ok := pathString(c, "name")
	if !ok {
		return
	}
	force := c.DefaultQuery("force", "false") == "true"
	if err := h.Svc.DockerRemoveVolume(c.Request.Context(), principal.Must(c), envID, name, force); err != nil {
		response.Fail(c, err)
		return
	}
	response.Ok(c)
}

// DockerVolumesPrune 清理未使用数据卷。
func (h *Handler) DockerVolumesPrune(c *gin.Context) {
	envID, ok := pathString(c, "id")
	if !ok {
		return
	}
	n, space, err := h.Svc.DockerVolumesPrune(c.Request.Context(), principal.Must(c), envID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, gin.H{"deleted": n, "spaceReclaimed": space})
}

func parseIntQuery(c *gin.Context, key string, def int) int {
	v := strings.TrimSpace(c.Query(key))
	if v == "" {
		return def
	}
	n := 0
	for _, r := range v {
		if r < '0' || r > '9' {
			return def
		}
		n = n*10 + int(r-'0')
	}
	if n <= 0 {
		return def
	}
	return n
}
