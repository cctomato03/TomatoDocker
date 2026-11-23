package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"web-basic/internal/docker"
	"web-basic/internal/principal"
	"web-basic/internal/service"
	"web-basic/pkg/response"
)

// DockerStacks Stack 列表。
func (h *Handler) DockerStacks(c *gin.Context) {
	list, err := h.Svc.ListDockerStacks(c.Request.Context(), principal.Must(c), c.Query("envId"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, list)
}

// DockerStack Stack 详情。
func (h *Handler) DockerStack(c *gin.Context) {
	id, ok := pathString(c, "id")
	if !ok {
		return
	}
	detail, err := h.Svc.GetDockerStack(c.Request.Context(), principal.Must(c), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, detail)
}

// DockerCreateStack 新建 Stack。
func (h *Handler) DockerCreateStack(c *gin.Context) {
	var req service.DockerStackReq
	if !bindJSON(c, &req) {
		return
	}
	id, err := h.Svc.CreateDockerStack(c.Request.Context(), principal.Must(c), &req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, gin.H{"id": id})
}

// DockerUpdateStack 保存 Stack 内容。
func (h *Handler) DockerUpdateStack(c *gin.Context) {
	id, ok := pathString(c, "id")
	if !ok {
		return
	}
	var req service.DockerStackUpdateReq
	if !bindJSON(c, &req) {
		return
	}
	if err := h.Svc.UpdateDockerStack(c.Request.Context(), principal.Must(c), id, &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.Ok(c)
}

// DockerDeleteStack 删除 Stack。
func (h *Handler) DockerDeleteStack(c *gin.Context) {
	id, ok := pathString(c, "id")
	if !ok {
		return
	}
	deleteRuntime := c.DefaultQuery("runtime", "true") == "true"
	removeVolumes := c.DefaultQuery("volumes", "false") == "true"
	if err := h.Svc.DeleteDockerStack(c.Request.Context(), principal.Must(c), id, deleteRuntime, removeVolumes); err != nil {
		response.Fail(c, err)
		return
	}
	response.Ok(c)
}

// DockerDeployStack 部署 Stack。
func (h *Handler) DockerDeployStack(c *gin.Context) {
	id, ok := pathString(c, "id")
	if !ok {
		return
	}
	var opts docker.DeployOptions
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&opts); err != nil {
			response.Fail(c, response.BindErr(err))
			return
		}
	}
	if err := h.Svc.DeployDockerStack(c.Request.Context(), principal.Must(c), id, opts); err != nil {
		response.Fail(c, err)
		return
	}
	response.Ok(c)
}

// DockerStopStack 停止 Stack。
func (h *Handler) DockerStopStack(c *gin.Context) {
	id, ok := pathString(c, "id")
	if !ok {
		return
	}
	if err := h.Svc.StopDockerStack(c.Request.Context(), principal.Must(c), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.Ok(c)
}

// DockerRollbackStack 回滚 Stack。
func (h *Handler) DockerRollbackStack(c *gin.Context) {
	id, ok := pathString(c, "id")
	if !ok {
		return
	}
	var req struct {
		Version   int  `json:"version" binding:"required"`
		DeployNow bool `json:"deployNow"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if err := h.Svc.RollbackDockerStack(c.Request.Context(), principal.Must(c), id, req.Version, req.DeployNow); err != nil {
		response.Fail(c, err)
		return
	}
	response.Ok(c)
}

// DockerExportStacks 导出 Stack 定义（JSON）。
func (h *Handler) DockerExportStacks(c *gin.Context) {
	list, err := h.Svc.ExportDockerStacks(c.Request.Context(), principal.Must(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, gin.H{"stacks": list, "exportedAt": nowText()})
}

// DockerImportStacks 导入 Stack 定义。
func (h *Handler) DockerImportStacks(c *gin.Context) {
	var req service.DockerStackImportReq
	if !bindJSON(c, &req) {
		return
	}
	result, err := h.Svc.ImportDockerStacks(c.Request.Context(), principal.Must(c), &req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OkData(c, result)
}

// DockerBackupDatabase 下载 SQLite 备份。
func (h *Handler) DockerBackupDatabase(c *gin.Context) {
	b, name, err := h.Svc.BackupDockerDatabase(c.Request.Context(), principal.Must(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", `attachment; filename="`+name+`"`)
	c.Header("Content-Length", itoa(len(b)))
	c.Data(http.StatusOK, "application/octet-stream", b)
}

func nowText() string { return time.Now().Format("2006-01-02 15:04:05") }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
