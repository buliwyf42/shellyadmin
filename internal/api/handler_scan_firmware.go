package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ScanStart triggers a discovery scan over the configured subnets +
// optional mDNS. The job runs in the background; callers poll
// ScanStatus to see progress and pending entries.
func (h *Handler) ScanStart(c *gin.Context) {
	if err := h.service.StartScan(); err != nil {
		status := http.StatusBadRequest
		if err.Error() == "scan already running" {
			status = http.StatusOK
		}
		h.logReq(c, "DEBUG", fmt.Sprintf("[http] %s %s -> %d: %v",
			c.Request.Method, c.Request.URL.Path, status, err))
		c.JSON(status, gin.H{"status": "started", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "started"})
}

// ScanStatus returns the current scan job state plus the pending list
// (devices found but not yet confirmed-into the inventory).
func (h *Handler) ScanStatus(c *gin.Context) {
	status, err := h.service.ScanStatus()
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "internal error", err)
		return
	}
	c.JSON(http.StatusOK, status)
}

// ScanConfirm promotes pending scan results into the persistent
// inventory. Empty MACs list confirms every pending entry.
func (h *Handler) ScanConfirm(c *gin.Context) {
	var req struct {
		MACs []string `json:"macs"`
	}
	if err := decodeJSON(c, &req, 16*1024); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	count, err := h.service.ConfirmScan(req.MACs)
	if err != nil {
		h.respondUserError(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "count": count})
}

// FirmwareCheck triggers a per-device `Shelly.CheckForUpdate` and
// records the result on the device row. The request body is accepted
// for backwards-compat but ignored — both channels are queried in one
// call.
func (h *Handler) FirmwareCheck(c *gin.Context) {
	if c.Request.ContentLength > 0 {
		var ignored map[string]any
		_ = decodeJSON(c, &ignored, 4*1024)
	}
	total, err := h.service.StartFirmwareCheck()
	if err != nil && err.Error() != "firmware check already running" {
		h.respondUserError(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "started", "total": total})
}

// FirmwareStatus reports the current/queued firmware-check job state
// plus the per-device update availability map. Supports level / search
// / risk filters and pagination via query params (see openapi.go).
func (h *Handler) FirmwareStatus(c *gin.Context) {
	status, err := h.service.FirmwareStatus()
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "internal error", err)
		return
	}
	c.JSON(http.StatusOK, status)
}

// FirmwareUpdate triggers a `Shelly.Update` install across the listed
// MACs for the chosen channel (stable/beta). The job runs in the
// background; callers poll FirmwareInstallStatus for per-device results.
func (h *Handler) FirmwareUpdate(c *gin.Context) {
	var req struct {
		MACs  []string `json:"macs"`
		Stage string   `json:"stage"`
	}
	if err := decodeJSON(c, &req, 16*1024); err != nil || len(req.MACs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "macs required"})
		return
	}
	jobID, total, err := h.service.StartFirmwareInstall(req.MACs, req.Stage)
	if err != nil {
		if err.Error() == "firmware install already running" {
			c.JSON(http.StatusOK, gin.H{"status": "running", "job_id": jobID, "total": total})
			return
		}
		h.respondUserError(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "started", "job_id": jobID, "total": total})
}

// FirmwareInstallStatus reports the per-device install-job state.
func (h *Handler) FirmwareInstallStatus(c *gin.Context) {
	status, err := h.service.FirmwareInstallStatus()
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "internal error", err)
		return
	}
	c.JSON(http.StatusOK, status)
}
