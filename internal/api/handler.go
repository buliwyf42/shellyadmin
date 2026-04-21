package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"

	"shellyadmin/internal/db"
	"shellyadmin/internal/models"
	"shellyadmin/internal/services"
)

type Handler struct {
	db      *db.DB
	cfg     Config
	service *services.AppService
}

func NewHandler(database *db.DB, cfg Config) *Handler {
	handler := &Handler{
		db:  database,
		cfg: cfg,
	}
	handler.service = services.NewAppService(database, cfg.DataDir, handler.logFn)
	return handler
}

func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(c, &req, 4*1024); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if subtle.ConstantTimeCompare([]byte(req.Username), []byte(h.cfg.User)) != 1 ||
		subtle.ConstantTimeCompare([]byte(req.Password), []byte(h.cfg.Pass)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	session := sessions.Default(c)
	session.Clear()
	session.Set("user", req.Username)
	nonce := RandomSecret()
	session.Set("nonce", nonce)
	_ = session.Save()
	c.Header("X-CSRF-Token", nonce)
	c.JSON(http.StatusOK, gin.H{"ok": true, "csrf_token": nonce})
}

func (h *Handler) Logout(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	session.Options(sessions.Options{Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: h.cfg.CookieSecure})
	_ = session.Save()
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) GetDevices(c *gin.Context) {
	devices, err := h.service.GetDevices()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, devices)
}

func (h *Handler) GetDeviceDetail(c *gin.Context) {
	detail, err := h.service.GetDeviceDetail(c.Param("target"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, detail)
}

func (h *Handler) ListDeviceActions(c *gin.Context) {
	actions, err := h.service.ListDeviceActions(c.Param("target"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"actions": actions})
}

func (h *Handler) ExecuteDeviceAction(c *gin.Context) {
	var req services.DeviceActionRequest
	if err := decodeJSON(c, &req, 4*1024); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	result, err := h.service.ExecuteDeviceAction(c.Request.Context(), c.Param("target"), c.Param("action"), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) CSRFToken(c *gin.Context) {
	session := sessions.Default(c)
	nonce, _ := session.Get("nonce").(string)
	if nonce == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing session nonce"})
		return
	}
	c.Header("X-CSRF-Token", nonce)
	c.JSON(http.StatusOK, gin.H{"csrf_token": nonce})
}

func (h *Handler) Version(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"backend_version": h.cfg.BackendVersion,
		"commit":          h.cfg.BackendCommit,
	})
}

func (h *Handler) RefreshDevices(c *gin.Context) {
	devices, err := h.service.RefreshDevices(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, devices)
}

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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, devices)
}

func (h *Handler) ForgetDevice(c *gin.Context) {
	var req struct {
		Target string `json:"target"`
	}
	if err := decodeJSON(c, &req, 4*1024); err != nil || req.Target == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target required"})
		return
	}
	if err := h.service.ForgetDevice(req.Target); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) BulkAction(c *gin.Context) {
	var req services.BulkActionRequest
	if err := decodeJSON(c, &req, 16*1024); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if req.DryRun {
		preview, err := h.service.PreviewBulkAction(req)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"dry_run": true, "preview": preview})
		return
	}
	results, err := h.service.BulkAction(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"dry_run": false, "results": results})
}

func (h *Handler) ScanStart(c *gin.Context) {
	if err := h.service.StartScan(); err != nil {
		status := http.StatusBadRequest
		if err.Error() == "scan already running" {
			status = http.StatusOK
		}
		c.JSON(status, gin.H{"status": "started", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "started"})
}

func (h *Handler) ScanStatus(c *gin.Context) {
	status, err := h.service.ScanStatus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, status)
}

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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "count": count})
}

func (h *Handler) FirmwareCheck(c *gin.Context) {
	var req struct {
		Stage string `json:"stage"`
	}
	if err := decodeJSON(c, &req, 4*1024); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	total, err := h.service.StartFirmwareCheck(req.Stage)
	if err != nil && err.Error() != "firmware check already running" {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "started", "total": total})
}

func (h *Handler) FirmwareStatus(c *gin.Context) {
	status, err := h.service.FirmwareStatus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, status)
}

func (h *Handler) FirmwareUpdate(c *gin.Context) {
	var req struct {
		MACs  []string `json:"macs"`
		Stage string   `json:"stage"`
	}
	if err := decodeJSON(c, &req, 16*1024); err != nil || len(req.MACs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "macs required"})
		return
	}
	results, err := h.service.FirmwareUpdate(c.Request.Context(), req.MACs, req.Stage)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, results)
}

func (h *Handler) Provision(c *gin.Context) {
	var req struct {
		IPs           []string               `json:"ips"`
		Template      map[string]interface{} `json:"template"`
		CredentialRef string                 `json:"credential_ref"`
	}
	if err := decodeJSON(c, &req, services.MaxJSONBytes); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	results, err := h.service.Provision(c.Request.Context(), req.IPs, req.Template, req.CredentialRef)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, results)
}

func (h *Handler) UploadUserCA(c *gin.Context) {
	var req struct {
		IPs  []string `json:"ips"`
		Kind string   `json:"kind"`
		PEM  string   `json:"pem"`
	}
	// PEM cap (MaxUserCABytes) plus headroom for the IP list and JSON envelope.
	if err := decodeJSON(c, &req, services.MaxUserCABytes+32*1024); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	results, err := h.service.UploadUserCA(c.Request.Context(), req.IPs, req.Kind, req.PEM)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, results)
}

func (h *Handler) GetSettings(c *gin.Context) {
	settings, err := h.service.GetSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, settings)
}

func (h *Handler) SaveSettings(c *gin.Context) {
	var settings models.AppSettings
	if err := decodeJSON(c, &settings, 64*1024); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid settings"})
		return
	}
	if err := h.service.SaveSettings(settings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) ListTemplates(c *gin.Context) {
	names, err := h.service.ListTemplates()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, names)
}

func (h *Handler) GetTemplate(c *gin.Context) {
	record, err := h.service.GetTemplate(c.Param("name"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "template not found"})
		return
	}
	c.JSON(http.StatusOK, record)
}

func (h *Handler) SaveTemplate(c *gin.Context) {
	var req struct {
		Content       string `json:"content"`
		CredentialRef string `json:"credential_ref"`
	}
	if err := decodeJSON(c, &req, services.MaxTemplateBytes+1024); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content required"})
		return
	}
	if err := h.service.SaveTemplate(c.Param("name"), req.Content, req.CredentialRef); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) DeleteTemplate(c *gin.Context) {
	if err := h.service.DeleteTemplate(c.Param("name")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) GetLogs(c *gin.Context) {
	entries, err := h.service.GetLogs(c.Query("level"), c.Query("search"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, entries)
}

func (h *Handler) DeleteLogs(c *gin.Context) {
	count, err := h.service.ClearLogs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "deleted": count})
}

func (h *Handler) ListCredentials(c *gin.Context) {
	credentials, err := h.service.ListCredentials()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, credentials)
}

func (h *Handler) SaveCredential(c *gin.Context) {
	var req models.Credential
	if err := decodeJSON(c, &req, 64*1024); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid credential"})
		return
	}
	if err := h.service.SaveCredential(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) DeleteCredential(c *gin.Context) {
	if err := h.service.DeleteCredential(c.Param("name")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) ListCredentialGroups(c *gin.Context) {
	groups, err := h.service.ListCredentialGroups()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, groups)
}

func (h *Handler) SaveCredentialGroup(c *gin.Context) {
	var req models.CredentialGroup
	if err := decodeJSON(c, &req, 32*1024); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group"})
		return
	}
	if err := h.service.SaveCredentialGroup(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) DeleteCredentialGroup(c *gin.Context) {
	if err := h.service.DeleteCredentialGroup(c.Param("name")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) GetCredentialGroupAssignments(c *gin.Context) {
	assignments, err := h.service.ListCredentialGroupAssignments()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"assignments": assignments})
}

func (h *Handler) SaveCredentialGroupAssignments(c *gin.Context) {
	var req struct {
		MACs      []string `json:"macs"`
		GroupName string   `json:"group_name"`
	}
	if err := decodeJSON(c, &req, 128*1024); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid assignment request"})
		return
	}
	if err := h.service.SaveCredentialGroupAssignments(req.MACs, req.GroupName); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) ExportDevice(c *gin.Context) {
	target := c.Param("target")
	body, err := h.service.ExportDevice(target)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
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

func (h *Handler) ExportLogs(c *gin.Context) {
	body, filename, contentType, err := h.service.ExportLogs(c.Query("level"), c.Query("search"), c.Query("format"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Data(http.StatusOK, contentType, body)
}

func (h *Handler) ExportBackup(c *gin.Context) {
	includeSecrets := c.Query("include_secrets") == "true"
	if includeSecrets && c.Query("confirm") != "export-plaintext-secrets" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "plaintext secret export requires confirm=export-plaintext-secrets"})
		return
	}
	body, err := h.service.ExportBackup(includeSecrets)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, body)
}

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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, report)
}

func (h *Handler) OpenAPIV1(c *gin.Context) {
	c.JSON(http.StatusOK, openAPIV1Spec())
}

func (h *Handler) logFn(level, msg string) {
	_ = h.db.AddLog(level, services.SanitizeLogMessage(msg))
}

func RandomSecret() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(buf)
}

func decodeJSON(c *gin.Context, dst any, maxBytes int64) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errors.New("unexpected trailing content")
		}
		return err
	}
	return nil
}
