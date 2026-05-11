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
	Register    func(routes gin.IRoutes, h *Handler)
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
			Method:  http.MethodGet,
			Path:    "/api/version",
			Summary: "Read runtime version",
			Auth:    true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/version", h.Version)
			},
		},
		{
			Method:  http.MethodGet,
			Path:    "/api/devices",
			Summary: "List devices",
			Auth:    true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/devices", h.GetDevices)
			},
		},
		{
			Method:  http.MethodGet,
			Path:    "/api/devices/{target}",
			Summary: "Get device detail",
			Auth:    true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/devices/:target", h.GetDeviceDetail)
			},
		},
		{
			Method:  http.MethodGet,
			Path:    "/api/devices/{target}/actions",
			Summary: "List supported single-device actions",
			Auth:    true,
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
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/devices/:target/export", h.ExportDevice)
			},
		},
		{
			Method:  http.MethodPost,
			Path:    "/api/devices/{target}/actions/{action}",
			Summary: "Execute a supported single-device action",
			Auth:    true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/devices/:target/actions/:action", h.ExecuteDeviceAction)
			},
		},
		{
			Method:  http.MethodPost,
			Path:    "/api/devices/refresh",
			Summary: "Refresh all known devices",
			Auth:    true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/devices/refresh", h.RefreshDevices)
			},
		},
		{
			Method:  http.MethodPost,
			Path:    "/api/devices/refresh-one",
			Summary: "Refresh one device by MAC or IP",
			Auth:    true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/devices/refresh-one", h.RefreshDevice)
			},
		},
		{
			Method:  http.MethodPost,
			Path:    "/api/devices/forget",
			Summary: "Forget one device",
			Auth:    true,
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
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/bulk", h.BulkAction)
			},
		},
		{
			Method:  http.MethodPost,
			Path:    "/api/scan/start",
			Summary: "Start a discovery scan",
			Auth:    true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/scan/start", h.ScanStart)
			},
		},
		{
			Method:  http.MethodGet,
			Path:    "/api/scan/status",
			Summary: "Get current or latest scan status",
			Auth:    true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/scan/status", h.ScanStatus)
			},
		},
		{
			Method:  http.MethodPost,
			Path:    "/api/scan/confirm",
			Summary: "Confirm discovered devices into inventory",
			Auth:    true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/scan/confirm", h.ScanConfirm)
			},
		},
		{
			Method:  http.MethodPost,
			Path:    "/api/firmware/check",
			Summary: "Start a fleet firmware check",
			Auth:    true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/firmware/check", h.FirmwareCheck)
			},
		},
		{
			Method:  http.MethodGet,
			Path:    "/api/firmware/status",
			Summary: "Read fleet firmware check status",
			Auth:    true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/firmware/status", h.FirmwareStatus)
			},
		},
		{
			Method:  http.MethodPost,
			Path:    "/api/firmware/update",
			Summary: "Trigger firmware updates for selected devices",
			Auth:    true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/firmware/update", h.FirmwareUpdate)
			},
		},
		{
			Method:  http.MethodGet,
			Path:    "/api/firmware/install/status",
			Summary: "Read fleet firmware install status",
			Auth:    true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/firmware/install/status", h.FirmwareInstallStatus)
			},
		},
		{
			Method:  http.MethodPost,
			Path:    "/api/provision",
			Summary: "Provision selected IP targets",
			Auth:    true,
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
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/provision/user-ca", h.UploadUserCA)
			},
		},
		{
			Method:  http.MethodGet,
			Path:    "/api/settings",
			Summary: "Read application settings",
			Auth:    true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/settings", h.GetSettings)
			},
		},
		{
			Method:  http.MethodPost,
			Path:    "/api/settings",
			Summary: "Save application settings",
			Auth:    true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/settings", h.SaveSettings)
			},
		},
		{
			Method:  http.MethodGet,
			Path:    "/api/templates",
			Summary: "List provisioning templates",
			Auth:    true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/templates", h.ListTemplates)
			},
		},
		{
			Method:  http.MethodGet,
			Path:    "/api/templates/{name}",
			Summary: "Read one provisioning template",
			Auth:    true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/templates/:name", h.GetTemplate)
			},
		},
		{
			Method:  http.MethodPost,
			Path:    "/api/templates/{name}",
			Summary: "Create or update one provisioning template",
			Auth:    true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/templates/:name", h.SaveTemplate)
			},
		},
		{
			Method:  http.MethodDelete,
			Path:    "/api/templates/{name}",
			Summary: "Delete one provisioning template",
			Auth:    true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.DELETE("/api/templates/:name", h.DeleteTemplate)
			},
		},
		{
			Method:  http.MethodGet,
			Path:    "/api/credentials",
			Summary: "List credentials",
			Auth:    true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/credentials", h.ListCredentials)
			},
		},
		{
			Method:  http.MethodPost,
			Path:    "/api/credentials",
			Summary: "Create or update one credential",
			Auth:    true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/credentials", h.SaveCredential)
			},
		},
		{
			Method:  http.MethodDelete,
			Path:    "/api/credentials/{name}",
			Summary: "Delete one credential",
			Auth:    true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.DELETE("/api/credentials/:name", h.DeleteCredential)
			},
		},
		{
			Method:  http.MethodGet,
			Path:    "/api/credential-groups",
			Summary: "List credential groups",
			Auth:    true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/credential-groups", h.ListCredentialGroups)
			},
		},
		{
			Method:  http.MethodPost,
			Path:    "/api/credential-groups",
			Summary: "Create or update one credential group",
			Auth:    true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.POST("/api/credential-groups", h.SaveCredentialGroup)
			},
		},
		{
			Method:  http.MethodDelete,
			Path:    "/api/credential-groups/{name}",
			Summary: "Delete one credential group",
			Auth:    true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.DELETE("/api/credential-groups/:name", h.DeleteCredentialGroup)
			},
		},
		{
			Method:  http.MethodGet,
			Path:    "/api/credential-groups/assignments",
			Summary: "List device-to-group assignments",
			Auth:    true,
			Register: func(routes gin.IRoutes, h *Handler) {
				routes.GET("/api/credential-groups/assignments", h.GetCredentialGroupAssignments)
			},
		},
		{
			Method:  http.MethodPost,
			Path:    "/api/credential-groups/assignments",
			Summary: "Save device-to-group assignments",
			Auth:    true,
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

func registerDocumentedAPIRoutes(publicRoutes gin.IRoutes, authRoutes gin.IRoutes, h *Handler) {
	for _, route := range documentedAPIRoutes() {
		target := publicRoutes
		if route.Auth {
			target = authRoutes
		}
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
