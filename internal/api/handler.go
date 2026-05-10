package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"

	"shellyadmin/internal/db"
	"shellyadmin/internal/middleware"
	"shellyadmin/internal/models"
	"shellyadmin/internal/services"
)

type Handler struct {
	db      *db.DB
	cfg     Config
	service *services.AppService
	// auditSink persists a single audit row. It's pluggable so tests can
	// capture output without standing up SQLite — the production wiring in
	// NewHandler sanitizes, mirrors to slog, then writes to the DB.
	auditSink func(level, msg, requestID string)
	// auditSinkAttrs is the structured variant; takes the catalog risk
	// level so action-execution rows can be filtered without parsing the
	// message body. Defaulted from auditSink in NewHandler so tests that
	// only stub auditSink keep working.
	auditSinkAttrs func(level, msg, requestID, riskLevel string)
	// logFn is the context-aware audit helper passed to services-layer
	// callbacks. When ctx carries a request ID (set by the RequestID
	// middleware), that ID flows into the audit row and slog line.
	logFn func(ctx context.Context, level, msg string)
}

func NewHandler(database *db.DB, cfg Config) *Handler {
	handler := &Handler{
		db:  database,
		cfg: cfg,
	}
	handler.auditSink = func(level, msg, reqID string) {
		handler.auditSinkAttrs(level, msg, reqID, "")
	}
	handler.auditSinkAttrs = func(level, msg, reqID, riskLevel string) {
		sanitized := services.SanitizeLogMessage(msg)
		emitSlogWithRisk(level, sanitized, reqID, riskLevel)
		_ = handler.db.AddLogWithAttrs(level, sanitized, reqID, riskLevel)
	}
	handler.logFn = func(ctx context.Context, level, msg string) {
		handler.auditSinkAttrs(level, msg, middleware.FromContext(ctx), services.RiskFromContext(ctx))
	}
	if cfg.Service != nil {
		// Reuse the externally-supplied AppService so background workers
		// (firmware-check scheduler) and the MCP controller share state
		// with HTTP handlers. The shared service was already constructed
		// with its own logFn; we leave it untouched.
		handler.service = cfg.Service
	} else {
		handler.service = services.NewAppService(database, cfg.DataDir, handler.logFn)
	}
	return handler
}

// logReq persists an audit entry tagged with the current request's
// correlation ID. Callers that already have a gin.Context should prefer this
// over h.logFn so the audit row links back to the originating request.
func (h *Handler) logReq(c *gin.Context, level, msg string) {
	h.auditSink(level, msg, middleware.FromGinContext(c))
}

// emitSlogWithRisk mirrors audit lines to the stdlib slog logger so
// operators tailing the container log see structured JSON rather than just
// the DB-persisted audit trail. The risk_level attribute is populated on
// action-execution rows so an operator grepping the container log can
// filter on it the same way SQLite queries do. Unknown levels fall back
// to info.
func emitSlogWithRisk(level, msg, reqID, riskLevel string) {
	attrs := []any{}
	if reqID != "" {
		attrs = append(attrs, slog.String("request_id", reqID))
	}
	if riskLevel != "" {
		attrs = append(attrs, slog.String("risk_level", riskLevel))
	}
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "DEBUG":
		slog.Debug(msg, attrs...)
	case "WARN", "WARNING":
		slog.Warn(msg, attrs...)
	case "ERROR":
		slog.Error(msg, attrs...)
	default:
		slog.Info(msg, attrs...)
	}
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
		!h.verifyAdminPassword(c, req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	session := sessions.Default(c)
	session.Clear()
	session.Set("user", req.Username)
	nonce := RandomSecret()
	session.Set("nonce", nonce)
	if err := session.Save(); err != nil {
		h.logReq(c, "ERROR", fmt.Sprintf("login: session save failed: %v", err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "session persistence failed"})
		return
	}
	c.Header("X-CSRF-Token", nonce)
	c.JSON(http.StatusOK, gin.H{"ok": true, "csrf_token": nonce})
}

func (h *Handler) Logout(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	session.Options(sessions.Options{Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: h.cfg.CookieSecure})
	if err := session.Save(); err != nil {
		// Logout is best-effort: the cookie's MaxAge=-1 already clears the
		// client side, so surface the persistence error to the audit log but
		// still return ok so the user sees a successful sign-out.
		h.logReq(c, "WARN", fmt.Sprintf("logout: session save failed: %v", err))
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) GetDevices(c *gin.Context) {
	devices, err := h.service.GetDevices()
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "internal error", err)
		return
	}
	c.JSON(http.StatusOK, devices)
}

func (h *Handler) GetDeviceDetail(c *gin.Context) {
	detail, err := h.service.GetDeviceDetail(c.Param("target"))
	if err != nil {
		h.respondUserError(c, http.StatusNotFound, err)
		return
	}
	c.JSON(http.StatusOK, detail)
}

func (h *Handler) ListDeviceActions(c *gin.Context) {
	actions, err := h.service.ListDeviceActions(c.Param("target"))
	if err != nil {
		h.respondUserError(c, http.StatusNotFound, err)
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
		h.respondUserError(c, http.StatusBadRequest, err)
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
		h.respondError(c, http.StatusInternalServerError, "internal error", err)
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
		h.respondUserError(c, http.StatusBadRequest, err)
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
		h.respondError(c, http.StatusInternalServerError, "internal error", err)
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

func (h *Handler) ScanStatus(c *gin.Context) {
	status, err := h.service.ScanStatus()
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "internal error", err)
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
		h.respondUserError(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "count": count})
}

func (h *Handler) FirmwareCheck(c *gin.Context) {
	// Body is accepted for backwards compat but ignored — Shelly.CheckForUpdate
	// returns both stable and beta in a single call, so the channel selector
	// is purely a frontend display filter.
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

func (h *Handler) FirmwareStatus(c *gin.Context) {
	status, err := h.service.FirmwareStatus()
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "internal error", err)
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

func (h *Handler) FirmwareInstallStatus(c *gin.Context) {
	status, err := h.service.FirmwareInstallStatus()
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "internal error", err)
		return
	}
	c.JSON(http.StatusOK, status)
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
		h.respondUserError(c, http.StatusBadRequest, err)
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
		h.respondUserError(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, results)
}

func (h *Handler) GetSettings(c *gin.Context) {
	settings, err := h.service.GetSettings()
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "internal error", err)
		return
	}
	// Redact the MCP token before it crosses the wire — service layer
	// returns it decrypted for internal callers, but the SPA must never
	// see plaintext. The "<set>" placeholder lets the UI show "configured"
	// state and round-trip the field on save without exposing the secret.
	if settings.MCPToken != "" {
		settings.MCPToken = services.MCPTokenRedacted
	}
	// Tell the UI when the env var is overriding the persisted settings,
	// so the MCP fields render read-only with an override notice. Plus
	// surface live listener status for the running/stopped badge.
	settings.MCPManagedByEnv = h.service.MCPManagedByEnv()
	settings.MCPRunning = h.service.MCPRunning()
	c.JSON(http.StatusOK, settings)
}

func (h *Handler) SaveSettings(c *gin.Context) {
	var settings models.AppSettings
	if err := decodeJSON(c, &settings, 64*1024); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid settings"})
		return
	}
	if err := h.service.SaveSettings(settings); err != nil {
		h.respondUserError(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) ListTemplates(c *gin.Context) {
	names, err := h.service.ListTemplates()
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "internal error", err)
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
		h.respondUserError(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) DeleteTemplate(c *gin.Context) {
	if err := h.service.DeleteTemplate(c.Param("name")); err != nil {
		h.respondError(c, http.StatusInternalServerError, "internal error", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) GetLogs(c *gin.Context) {
	entries, err := h.service.GetLogsFiltered(c.Query("level"), c.Query("search"), c.Query("risk"))
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "internal error", err)
		return
	}
	c.JSON(http.StatusOK, entries)
}

func (h *Handler) DeleteLogs(c *gin.Context) {
	count, err := h.service.ClearLogs()
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "internal error", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "deleted": count})
}

func (h *Handler) ListCredentials(c *gin.Context) {
	credentials, err := h.service.ListCredentials()
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "internal error", err)
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
		h.respondUserError(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) DeleteCredential(c *gin.Context) {
	if err := h.service.DeleteCredential(c.Param("name")); err != nil {
		h.respondUserError(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) ListCredentialGroups(c *gin.Context) {
	groups, err := h.service.ListCredentialGroups()
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "internal error", err)
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
		h.respondUserError(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) DeleteCredentialGroup(c *gin.Context) {
	if err := h.service.DeleteCredentialGroup(c.Param("name")); err != nil {
		h.respondUserError(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) GetCredentialGroupAssignments(c *gin.Context) {
	assignments, err := h.service.ListCredentialGroupAssignments()
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "internal error", err)
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
		h.respondUserError(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

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

func (h *Handler) ExportLogs(c *gin.Context) {
	body, filename, contentType, err := h.service.ExportLogsFiltered(c.Query("level"), c.Query("search"), c.Query("risk"), c.Query("format"))
	if err != nil {
		h.respondUserError(c, http.StatusBadRequest, err)
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
		h.respondError(c, http.StatusInternalServerError, "internal error", err)
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
		h.respondUserError(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, report)
}

func (h *Handler) OpenAPIV1(c *gin.Context) {
	c.JSON(http.StatusOK, openAPIV1Spec())
}

// verifyAdminPassword checks the supplied plaintext against the configured
// argon2id PHC hash from cfg.PassHash. Returns false on empty plaintext so
// blank submissions can't squeak through.
func (h *Handler) verifyAdminPassword(c *gin.Context, plain string) bool {
	if plain == "" {
		return false
	}
	ok, err := services.VerifyPassword(plain, h.cfg.PassHash)
	if err != nil {
		h.logReq(c, "ERROR", fmt.Sprintf("password hash verify failed: %v", err))
		return false
	}
	return ok
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
