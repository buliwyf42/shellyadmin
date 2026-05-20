package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"shellyadmin/internal/middleware"
)

type apiRouteDoc struct {
	Method      string
	Path        string
	Summary     string
	Description string
	Auth        bool
	// Scope declares the per-route PAT scope required by T3. Empty
	// string means "no scope required" — i.e. cookie-only routes that
	// reject PAT-authed callers entirely (managed by the handler
	// itself via requirePATCallerCookieOnly). When non-empty, the
	// route is wrapped in middleware.RequireScope(Scope) at register
	// time so a PAT without that scope (or `admin`) gets a 403.
	Scope    string
	Register func(routes gin.IRoutes, h *Handler)
}

func documentedAPIRoutes() []apiRouteDoc {
	return []apiRouteDoc{
		{
			Method:  http.MethodGet,
			Path:    "/health",
			Summary: "Health check",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/health", h.Health)
			},
		},
		{
			Method:      http.MethodGet,
			Path:        "/ready",
			Summary:     "Readiness probe",
			Description: "Operator-facing health detail (DB reachability, MCP listener state). Returns 503 when degraded. Distinct from /health which stays 200/OK for container liveness probes.",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/ready", h.Ready)
			},
		},
		{
			Method:      http.MethodGet,
			Path:        "/api/setup/status",
			Summary:     "Report first-run setup state",
			Description: "Public, pre-auth probe returning {configured: bool}. The SPA calls this on load to choose between the first-run setup screen and the login screen. Never returns credential material.",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/setup/status", h.SetupStatus)
			},
		},
		{
			Method:      http.MethodPost,
			Path:        "/api/setup",
			Summary:     "Complete first-run setup",
			Description: "Public, rate-limited, one-shot. With no operator login configured, stores the supplied username + password (argon2id-hashed) as the operator account. Returns 409 once an account exists.",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/setup", middleware.LoginRateLimit(), h.Setup)
			},
		},
		{
			Method:      http.MethodPost,
			Path:        "/api/login",
			Summary:     "Start authenticated session",
			Description: "Create a session cookie and return the CSRF token for subsequent authenticated requests.",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/login", middleware.LoginRateLimit(), h.Login)
			},
		},
		{
			Method:      http.MethodPost,
			Path:        "/api/logout",
			Summary:     "End authenticated session",
			Description: "Invalidate the active session cookie.",
			Auth:        true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/logout", h.Logout)
			},
		},
		{
			Method:      http.MethodGet,
			Path:        "/api/csrf-token",
			Summary:     "Fetch CSRF token",
			Description: "Return the CSRF token bound to the authenticated session.",
			Auth:        true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/csrf-token", h.CSRFToken)
			},
		},
		{
			Method:      http.MethodPost,
			Path:        "/api/account/credentials",
			Summary:     "Change operator username/password",
			Description: "Cookie-only. Verifies the current password, then updates the operator login and revokes all existing sessions so a stolen cookie cannot outlive the rotation. PAT-authed callers are rejected with 403.",
			Auth:        true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/account/credentials", h.ChangeCredentials)
			},
		},
		{
			Method:  http.MethodGet,
			Path:    "/api/version",
			Summary: "Read runtime version",
			Auth:    true,
			Scope:   "devices:read",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/version", h.Version)
			},
		},
		{
			Method:  http.MethodGet,
			Path:    "/api/devices",
			Summary: "List devices",
			Auth:    true,
			Scope:   "devices:read",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/devices", h.GetDevices)
			},
		},
		{
			Method:  http.MethodGet,
			Path:    "/api/devices/{target}",
			Summary: "Get device detail",
			Auth:    true,
			Scope:   "devices:read",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/devices/:target", h.GetDeviceDetail)
			},
		},
		{
			Method:  http.MethodGet,
			Path:    "/api/devices/{target}/actions",
			Summary: "List supported single-device actions",
			Auth:    true,
			Scope:   "devices:read",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/devices/:target/actions", h.ListDeviceActions)
			},
		},
		{
			Method:      http.MethodGet,
			Path:        "/api/devices/{target}/export",
			Summary:     "Export device snapshot as JSON",
			Description: "Return the device's record, parsed config/status, and discovered capabilities as a downloadable JSON snapshot.",
			Auth:        true,
			Scope:       "devices:read",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/devices/:target/export", h.ExportDevice)
			},
		},
		{
			Method:  http.MethodPost,
			Path:    "/api/devices/{target}/actions/{action}",
			Summary: "Execute a supported single-device action",
			Auth:    true,
			Scope:   "devices:write",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/devices/:target/actions/:action", h.ExecuteDeviceAction)
			},
		},
		{
			Method:  http.MethodPost,
			Path:    "/api/devices/refresh",
			Summary: "Refresh all known devices",
			Auth:    true,
			Scope:   "devices:write",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/devices/refresh", h.RefreshDevices)
			},
		},
		{
			Method:  http.MethodPost,
			Path:    "/api/devices/refresh-one",
			Summary: "Refresh one device by MAC or IP",
			Auth:    true,
			Scope:   "devices:write",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/devices/refresh-one", h.RefreshDevice)
			},
		},
		{
			Method:  http.MethodPost,
			Path:    "/api/devices/forget",
			Summary: "Forget one device",
			Auth:    true,
			Scope:   "devices:write",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/devices/forget", h.ForgetDevice)
			},
		},
		{
			Method:      http.MethodPost,
			Path:        "/api/bulk",
			Summary:     "Preview or apply a documented bulk settings action",
			Description: "Send `dry_run=true` to receive preview targets and warnings without changing devices.",
			Auth:        true,
			Scope:       "devices:write",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/bulk", h.BulkAction)
			},
		},
		{
			Method:  http.MethodPost,
			Path:    "/api/scan/start",
			Summary: "Start a discovery scan",
			Auth:    true,
			Scope:   "devices:write",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/scan/start", h.ScanStart)
			},
		},
		{
			Method:  http.MethodGet,
			Path:    "/api/scan/status",
			Summary: "Get current or latest scan status",
			Auth:    true,
			Scope:   "devices:read",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/scan/status", h.ScanStatus)
			},
		},
		{
			Method:  http.MethodPost,
			Path:    "/api/scan/confirm",
			Summary: "Confirm discovered devices into inventory",
			Auth:    true,
			Scope:   "devices:write",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/scan/confirm", h.ScanConfirm)
			},
		},
		{
			Method:  http.MethodPost,
			Path:    "/api/firmware/check",
			Summary: "Start a fleet firmware check",
			Auth:    true,
			Scope:   "firmware:read",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/firmware/check", h.FirmwareCheck)
			},
		},
		{
			Method:  http.MethodGet,
			Path:    "/api/firmware/status",
			Summary: "Read fleet firmware check status",
			Auth:    true,
			Scope:   "firmware:read",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/firmware/status", h.FirmwareStatus)
			},
		},
		{
			Method:  http.MethodPost,
			Path:    "/api/firmware/update",
			Summary: "Trigger firmware updates for selected devices",
			Auth:    true,
			Scope:   "firmware:write",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/firmware/update", h.FirmwareUpdate)
			},
		},
		{
			Method:  http.MethodGet,
			Path:    "/api/firmware/install/status",
			Summary: "Read fleet firmware install status",
			Auth:    true,
			Scope:   "firmware:read",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/firmware/install/status", h.FirmwareInstallStatus)
			},
		},
		{
			Method:  http.MethodPost,
			Path:    "/api/provision",
			Summary: "Provision selected IP targets",
			Auth:    true,
			Scope:   "provision",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/provision", h.Provision)
			},
		},
		{
			Method:      http.MethodPost,
			Path:        "/api/provision/user-ca",
			Summary:     "Upload a user CA PEM to selected devices",
			Description: "Chunked Shelly certificate upload. Accepts an optional kind field (\"user_ca\"|\"tls_client_cert\"|\"tls_client_key\"; default \"user_ca\") that selects Shelly.PutUserCA, Shelly.PutTLSClientCert, or Shelly.PutTLSClientKey. Sends the PEM in ~1KB chunks and commits so MQTT/WS configs referencing the corresponding file take effect.",
			Auth:        true,
			Scope:       "provision",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/provision/user-ca", h.UploadUserCA)
			},
		},
		{
			Method:  http.MethodGet,
			Path:    "/api/settings",
			Summary: "Read application settings",
			Auth:    true,
			Scope:   "settings:read",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/settings", h.GetSettings)
			},
		},
		{
			Method:  http.MethodPost,
			Path:    "/api/settings",
			Summary: "Save application settings",
			Auth:    true,
			Scope:   "settings:write",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/settings", h.SaveSettings)
			},
		},
		{
			Method:      http.MethodGet,
			Path:        "/api/totp/status",
			Summary:     "Read operator TOTP enrollment status",
			Description: "Returns whether the authenticated operator has an active TOTP row, plus the remaining backup-code count. Used by the Settings 2FA card to switch between the Enroll and Disable controls.",
			Auth:        true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/totp/status", h.GetTOTPStatus)
			},
		},
		{
			Method:      http.MethodPost,
			Path:        "/api/totp/enroll",
			Summary:     "Begin TOTP enrollment",
			Description: "Mints a fresh TOTP secret + 10 single-use backup codes for the authenticated operator and stashes the pending material in the session cookie. The secret + recovery codes are surfaced exactly once in the response body. Call /api/totp/verify-enroll with a code from the operator's authenticator app to commit. Returns 409 when the operator already has an active enrollment.",
			Auth:        true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/totp/enroll", h.EnrollTOTP)
			},
		},
		{
			Method:      http.MethodPost,
			Path:        "/api/totp/verify-enroll",
			Summary:     "Commit pending TOTP enrollment",
			Description: "Verifies the operator-supplied TOTP code against the in-flight secret stashed by /api/totp/enroll, secretbox-seals the secret + hashed backup codes, and persists the row. The pending session fields are cleared on success.",
			Auth:        true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/totp/verify-enroll", h.VerifyEnrollTOTP)
			},
		},
		{
			Method:      http.MethodPost,
			Path:        "/api/totp/disable",
			Summary:     "Disable TOTP for the authenticated operator",
			Description: "Requires a fresh TOTP code or unused backup code so a stolen session cookie cannot quietly turn 2FA off. Deletes the row on success.",
			Auth:        true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/totp/disable", h.DisableTOTP)
			},
		},
		{
			Method:      http.MethodGet,
			Path:        "/api/tokens",
			Summary:     "List Personal Access Tokens",
			Description: "Returns metadata (no plaintext secrets) for every PAT owned by the authenticated operator, plus the available_scopes catalog. Cookie-only — PAT-authed callers receive 403 to keep the operator's machine-credential inventory out of reach of a leaked single token.",
			Auth:        true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/tokens", h.ListTokens)
			},
		},
		{
			Method:      http.MethodPost,
			Path:        "/api/tokens",
			Summary:     "Mint a Personal Access Token",
			Description: "Creates a new bearer-token credential with the supplied scopes. The plaintext token is surfaced exactly once in the response body — the SPA copies it to clipboard then drops the field from memory. Cookie-only — PAT-authed callers cannot mint new tokens (privilege-escalation guard).",
			Auth:        true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/tokens", h.CreateToken)
			},
		},
		{
			Method:      http.MethodDelete,
			Path:        "/api/tokens/{id}",
			Summary:     "Revoke a Personal Access Token",
			Description: "Marks the row revoked. Subsequent requests carrying the token receive 401. Idempotent. Cookie-only.",
			Auth:        true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.DELETE("/api/tokens/:id", h.RevokeToken)
			},
		},
		{
			Method:  http.MethodGet,
			Path:    "/api/templates",
			Summary: "List provisioning templates",
			Auth:    true,
			Scope:   "provision",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/templates", h.ListTemplates)
			},
		},
		{
			Method:  http.MethodGet,
			Path:    "/api/templates/{name}",
			Summary: "Read one provisioning template",
			Auth:    true,
			Scope:   "provision",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/templates/:name", h.GetTemplate)
			},
		},
		{
			Method:  http.MethodPost,
			Path:    "/api/templates/{name}",
			Summary: "Create or update one provisioning template",
			Auth:    true,
			Scope:   "provision",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/templates/:name", h.SaveTemplate)
			},
		},
		{
			Method:  http.MethodDelete,
			Path:    "/api/templates/{name}",
			Summary: "Delete one provisioning template",
			Auth:    true,
			Scope:   "provision",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.DELETE("/api/templates/:name", h.DeleteTemplate)
			},
		},
		{
			Method:  http.MethodGet,
			Path:    "/api/credentials",
			Summary: "List credentials",
			Auth:    true,
			Scope:   "provision",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/credentials", h.ListCredentials)
			},
		},
		{
			Method:  http.MethodPost,
			Path:    "/api/credentials",
			Summary: "Create or update one credential",
			Auth:    true,
			Scope:   "provision",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/credentials", h.SaveCredential)
			},
		},
		{
			Method:  http.MethodDelete,
			Path:    "/api/credentials/{name}",
			Summary: "Delete one credential",
			Auth:    true,
			Scope:   "provision",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.DELETE("/api/credentials/:name", h.DeleteCredential)
			},
		},
		{
			Method:  http.MethodGet,
			Path:    "/api/credential-groups",
			Summary: "List credential groups",
			Auth:    true,
			Scope:   "provision",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/credential-groups", h.ListCredentialGroups)
			},
		},
		{
			Method:  http.MethodPost,
			Path:    "/api/credential-groups",
			Summary: "Create or update one credential group",
			Auth:    true,
			Scope:   "provision",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/credential-groups", h.SaveCredentialGroup)
			},
		},
		{
			Method:  http.MethodDelete,
			Path:    "/api/credential-groups/{name}",
			Summary: "Delete one credential group",
			Auth:    true,
			Scope:   "provision",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.DELETE("/api/credential-groups/:name", h.DeleteCredentialGroup)
			},
		},
		{
			Method:  http.MethodGet,
			Path:    "/api/credential-groups/assignments",
			Summary: "List device-to-group assignments",
			Auth:    true,
			Scope:   "provision",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/credential-groups/assignments", h.GetCredentialGroupAssignments)
			},
		},
		{
			Method:  http.MethodPost,
			Path:    "/api/credential-groups/assignments",
			Summary: "Save device-to-group assignments",
			Auth:    true,
			Scope:   "provision",
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/credential-groups/assignments", h.SaveCredentialGroupAssignments)
			},
		},
		{
			Method:  http.MethodGet,
			Path:    "/api/logs",
			Summary: "Read audit logs",
			Auth:    true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/logs", h.GetLogs)
			},
		},
		{
			Method:      http.MethodGet,
			Path:        "/api/logs/export",
			Summary:     "Export audit logs as CSV or NDJSON",
			Description: "Same `level` + `search` filter as `/api/logs`. Pass `format=csv` (default) or `format=ndjson`. Caps at 100000 rows.",
			Auth:        true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/logs/export", h.ExportLogs)
			},
		},
		{
			Method:  http.MethodDelete,
			Path:    "/api/logs",
			Summary: "Delete audit logs",
			Auth:    true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.DELETE("/api/logs", h.DeleteLogs)
			},
		},
		{
			Method:  http.MethodGet,
			Path:    "/api/openapi/v1.json",
			Summary: "Fetch the documented API contract",
			Auth:    true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/openapi/v1.json", h.OpenAPIV1)
			},
		},
		{
			Method:  http.MethodGet,
			Path:    "/api/backup/export",
			Summary: "Export backup payload",
			Auth:    true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/backup/export", h.ExportBackup)
			},
		},
		{
			Method:  http.MethodPost,
			Path:    "/api/backup/import",
			Summary: "Dry-run or apply backup import",
			Auth:    true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/backup/import", h.ImportBackup)
			},
		},
	}
}

func registerDocumentedAPIRoutes(publicRoutes gin.IRoutes, authRoutes *gin.RouterGroup, h *Handler) {
	for _, route := range documentedAPIRoutes() {
		if !route.Auth {
			route.Register(publicRoutes, h)
			continue
		}
		// Per-route scope gate (T3). Routes without an explicit Scope
		// default to "admin" — meaning a PAT-authed caller without the
		// admin scope (e.g. devices:read PAT calling /api/logs) gets a
		// 403. Cookie-authed callers pass through RequireScope
		// unconditionally; the gate only fires on the PAT path.
		scope := route.Scope
		if scope == "" {
			scope = middleware.ScopeAdmin
		}
		// Sub-group isolates the middleware to this route — calling
		// .Use on the shared authRoutes itself would attach the scope
		// guard to every subsequent route.
		target := authRoutes.Group("")
		target.Use(middleware.RequireScope(scope))
		route.Register(target, h)
	}
}

func openAPIV1Spec() gin.H {
	paths := gin.H{}
	for _, route := range documentedAPIRoutes() {
		pathItem, ok := paths[route.Path].(gin.H)
		if !ok {
			pathItem = gin.H{}
		}
		operation := gin.H{
			"summary": route.Summary,
		}
		if route.Description != "" {
			operation["description"] = route.Description
		}
		pathItem[strings.ToLower(route.Method)] = operation
		paths[route.Path] = pathItem
	}
	return gin.H{
		"openapi": "3.1.0",
		"info": gin.H{
			"title":       "ShellyAdmin API",
			"version":     "v1",
			"description": "Documented integration surface for trusted-LAN ShellyAdmin deployments.",
		},
		"servers": []gin.H{
			{"url": "/"},
		},
		"paths": paths,
	}
}
