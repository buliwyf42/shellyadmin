package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"shellyadmin/internal/services"
)

// GetLogs returns the latest 500 audit rows filtered by level / search /
// risk. See db.GetLogsFiltered for the exact filter semantics.
func (h *Handler) GetLogs(c *gin.Context) {
	entries, err := h.service.GetLogsFiltered(c.Query("level"), c.Query("search"), c.Query("risk"))
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "internal error", err)
		return
	}
	c.JSON(http.StatusOK, entries)
}

// DeleteLogs is the manual clear-audit-log path used by the Logs page.
// Returns the number of rows removed. NOT used by the retention pruner
// — that path goes through services.PruneAuditLogOlderThan with a
// controlled bypass of the append-only trigger.
func (h *Handler) DeleteLogs(c *gin.Context) {
	count, err := h.service.ClearLogs()
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "internal error", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "deleted": count})
}

// ExportLogs streams the filtered audit log as CSV (default) or NDJSON,
// with a Content-Disposition attachment header so browsers prompt for
// a download.
func (h *Handler) ExportLogs(c *gin.Context) {
	body, filename, contentType, err := h.service.ExportLogsFiltered(c.Query("level"), c.Query("search"), c.Query("risk"), c.Query("format"))
	if err != nil {
		h.respondUserError(c, http.StatusBadRequest, err)
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Data(http.StatusOK, contentType, body)
}

// ExportDevice streams a single device's full state (device row +
// snapshot + compliance verdict) as a JSON attachment.
func (h *Handler) ExportDevice(c *gin.Context) {
	target := c.Param("target")
	body, err := h.service.ExportDevice(target)
	if err != nil {
		h.respondUserError(c, http.StatusNotFound, err)
		return
	}
	identifier := body.Device.MAC
	if identifier == "" {
		identifier = body.Device.IP
	}
	if identifier == "" {
		identifier = "device"
	}
	identifier = strings.ReplaceAll(identifier, ":", "")
	filename := fmt.Sprintf("shellyadmin-device-%s-%s.json", identifier, time.Now().UTC().Format("20060102T150405Z"))
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.JSON(http.StatusOK, body)
}

// ExportBackup writes a full backup (settings + templates + credentials
// + assignments + devices). include_secrets=true plus
// confirm=export-plaintext-secrets is required to include the
// decrypted credential password / HA1 fields.
func (h *Handler) ExportBackup(c *gin.Context) {
	includeSecrets := c.Query("include_secrets") == "true"
	if includeSecrets && c.Query("confirm") != "export-plaintext-secrets" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "plaintext secret export requires confirm=export-plaintext-secrets"})
		return
	}
	body, err := h.service.ExportBackup(includeSecrets)
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "internal error", err)
		return
	}
	c.JSON(http.StatusOK, body)
}

// ImportBackup restores a previously-exported backup. apply=false
// returns the import report (what *would* change); apply=true commits
// the changes inside a single transaction.
func (h *Handler) ImportBackup(c *gin.Context) {
	var req struct {
		Apply bool                  `json:"apply"`
		Data  services.BackupExport `json:"data"`
	}
	if err := decodeJSON(c, &req, services.MaxJSONBytes); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	report, err := h.service.ImportBackup(req.Data, req.Apply)
	if err != nil {
		h.respondUserError(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, report)
}
