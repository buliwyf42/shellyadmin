package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Health is the flat-200/OK container liveness probe. Stays simple so
// the orchestrator does not restart the container on a slow DB query.
// Operator-facing "is the service degraded?" detail lives in Ready.
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// Ready returns operator-oriented health detail: DB reachability,
// firmware-check scheduler heartbeat, MCP-listener status. This is
// NOT the container liveness probe — /health stays a flat 200/OK so
// k8s/Docker do not restart the container on a slow DB query. Use
// /ready in dashboards (Grafana, Uptime Kuma) when you want the
// "service degraded" signal without forcing a restart loop.
func (h *Handler) Ready(c *gin.Context) {
	resp := gin.H{"status": "ok"}
	dbStart := time.Now()
	if h.db != nil {
		if _, err := h.db.GetSettings(); err != nil {
			resp["status"] = "degraded"
			resp["db_error"] = err.Error()
		}
	}
	resp["db_ping_ms"] = time.Since(dbStart).Milliseconds()
	if h.service != nil {
		resp["mcp_running"] = h.service.MCPRunning()
		resp["mcp_managed_by_env"] = h.service.MCPManagedByEnv()
	}
	if resp["status"] == "degraded" {
		c.JSON(http.StatusServiceUnavailable, resp)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// Version reports the backend build identity the SPA shows in the About
// page footer.
func (h *Handler) Version(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"backend_version": h.cfg.BackendVersion,
		"commit":          h.cfg.BackendCommit,
	})
}

// OpenAPIV1 serves the generated OpenAPI 3.1 document.
func (h *Handler) OpenAPIV1(c *gin.Context) {
	c.JSON(http.StatusOK, openAPIV1Spec())
}
