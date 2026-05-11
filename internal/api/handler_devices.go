package api

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"shellyadmin/internal/models"
	"shellyadmin/internal/services"
)

// GetDevices returns the fleet inventory as the slim DeviceListView shape
// (M8 — docs/plans/phase-4b-refactor-block.md Block 4b.2). Compliance
// verdicts + per-component counts are stamped on the underlying
// models.Device by AppService.GetDevices() before the projection runs.
func (h *Handler) GetDevices(c *gin.Context) {
	devices, err := h.service.GetDevices()
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "internal error", err)
		return
	}
	c.JSON(http.StatusOK, models.ToListViews(devices))
}

// GetDeviceDetail returns the full per-device detail (raw config +
// raw status snapshots plus the compliance result). Target accepts
// MAC, IP, or device name.
func (h *Handler) GetDeviceDetail(c *gin.Context) {
	detail, err := h.service.GetDeviceDetail(c.Param("target"))
	if err != nil {
		h.respondUserError(c, http.StatusNotFound, err)
		return
	}
	c.JSON(http.StatusOK, detail)
}

// ListDeviceActions returns the catalog of `Shelly.ListMethods`-derived
// per-device actions that the operator can trigger from the Device
// Detail page.
func (h *Handler) ListDeviceActions(c *gin.Context) {
	actions, err := h.service.ListDeviceActions(c.Param("target"))
	if err != nil {
		h.respondUserError(c, http.StatusNotFound, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"actions": actions})
}

// ExecuteDeviceAction runs a named action against a specific device.
// The action surface and risk catalog live in
// services.ExecuteDeviceAction; the handler is a thin pass-through.
func (h *Handler) ExecuteDeviceAction(c *gin.Context) {
	var req services.DeviceActionRequest
	if err := decodeJSON(c, &req, 4*1024); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	result, err := h.service.ExecuteDeviceAction(c.Request.Context(), c.Param("target"), c.Param("action"), req)
	if err != nil {
		h.respondUserError(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// RefreshDevices kicks off a refresh of every known device. Returns the
// updated inventory (DeviceListView shape) once the job completes.
func (h *Handler) RefreshDevices(c *gin.Context) {
	devices, err := h.service.RefreshDevices(c.Request.Context())
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "internal error", err)
		return
	}
	c.JSON(http.StatusOK, models.ToListViews(devices))
}

// RefreshDevice refreshes a single device. Target accepts MAC, IP, or
// device name. Returns the post-refresh inventory in DeviceListView shape.
func (h *Handler) RefreshDevice(c *gin.Context) {
	var req struct {
		Target string `json:"target"`
	}
	if err := decodeJSON(c, &req, 4*1024); err != nil || req.Target == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target required"})
		return
	}
	devices, err := h.service.RefreshDevice(c.Request.Context(), req.Target)
	if err != nil {
		h.respondUserError(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, models.ToListViews(devices))
}

// ForgetDevice removes a device row from the inventory entirely. The
// device is not contacted; only the local record is dropped.
func (h *Handler) ForgetDevice(c *gin.Context) {
	var req struct {
		Target string `json:"target"`
	}
	if err := decodeJSON(c, &req, 4*1024); err != nil || req.Target == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target required"})
		return
	}
	if err := h.service.ForgetDevice(req.Target); err != nil {
		h.respondError(c, http.StatusInternalServerError, "internal error", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// BulkAction is the multi-device action surface used by the Devices
// page. dry_run=true returns the preview (eligible / skipped / risk);
// dry_run=false executes against every eligible target.
func (h *Handler) BulkAction(c *gin.Context) {
	var req services.BulkActionRequest
	if err := decodeJSON(c, &req, 16*1024); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if req.DryRun {
		preview, err := h.service.PreviewBulkAction(req)
		if err != nil {
			h.respondUserError(c, http.StatusBadRequest, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"dry_run": true, "preview": preview})
		return
	}
	results, err := h.service.BulkAction(c.Request.Context(), req)
	if err != nil {
		h.respondUserError(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"dry_run": false, "results": results})
}
