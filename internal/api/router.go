package api

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"

	"shellyadmin/internal/db"
	"shellyadmin/internal/middleware"
)

type Config struct {
	User           string
	Pass           string
	Secret         string
	CookieSecure   bool
	DataDir        string
	BackendVersion string
	BackendCommit  string
	StaticFS       embed.FS
	HasStatic      bool
	DevStatic      string
}

func NewRouter(database *db.DB, cfg Config) *gin.Engine {
	r := gin.Default()
	r.Use(middleware.SecurityHeaders())
	store := cookie.NewStore([]byte(cfg.Secret))
	store.Options(sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7,
		HttpOnly: true,
		Secure:   cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	r.Use(sessions.Sessions("shellyadmin", store))

	h := NewHandler(database, cfg)

	r.GET("/health", h.Health)
	r.GET("/login", func(c *gin.Context) {
		session := sessions.Default(c)
		if session.Get("user") != nil {
			c.Redirect(http.StatusFound, "/")
			return
		}
		serveSPA(c, cfg)
	})
	r.POST("/login", middleware.LoginRateLimit(), h.Login)
	r.POST("/api/login", middleware.LoginRateLimit(), h.Login)
	r.POST("/logout", middleware.RequireAuth(), middleware.RequireCSRF(), h.Logout)
	r.POST("/api/logout", middleware.RequireAuth(), middleware.RequireCSRF(), h.Logout)

	auth := r.Group("/")
	auth.Use(middleware.RequireAuth())
	auth.Use(middleware.APIRateLimit())
	auth.Use(middleware.RequireCSRF())
	auth.GET("/api/csrf-token", h.CSRFToken)
	auth.GET("/api/version", h.Version)
	auth.GET("/api/devices", h.GetDevices)
	auth.POST("/api/devices/refresh", h.RefreshDevices)
	auth.POST("/api/devices/refresh-one", h.RefreshDevice)
	auth.POST("/api/devices/forget", h.ForgetDevice)
	auth.POST("/api/bulk", h.BulkAction)
	auth.POST("/api/scan/start", h.ScanStart)
	auth.GET("/api/scan/status", h.ScanStatus)
	auth.POST("/api/scan/confirm", h.ScanConfirm)
	auth.POST("/api/firmware/check", h.FirmwareCheck)
	auth.GET("/api/firmware/status", h.FirmwareStatus)
	auth.POST("/api/firmware/update", h.FirmwareUpdate)
	auth.POST("/api/provision", h.Provision)
	auth.GET("/api/settings", h.GetSettings)
	auth.POST("/api/settings", h.SaveSettings)
	auth.GET("/api/templates", h.ListTemplates)
	auth.GET("/api/templates/:name", h.GetTemplate)
	auth.POST("/api/templates/:name", h.SaveTemplate)
	auth.DELETE("/api/templates/:name", h.DeleteTemplate)
	auth.GET("/api/credentials", h.ListCredentials)
	auth.POST("/api/credentials", h.SaveCredential)
	auth.DELETE("/api/credentials/:name", h.DeleteCredential)
	auth.GET("/api/credential-groups", h.ListCredentialGroups)
	auth.POST("/api/credential-groups", h.SaveCredentialGroup)
	auth.DELETE("/api/credential-groups/:name", h.DeleteCredentialGroup)
	auth.GET("/api/credential-groups/assignments", h.GetCredentialGroupAssignments)
	auth.POST("/api/credential-groups/assignments", h.SaveCredentialGroupAssignments)
	auth.GET("/api/logs", h.GetLogs)
	auth.DELETE("/api/logs", h.DeleteLogs)
	auth.GET("/api/backup/export", h.ExportBackup)
	auth.POST("/api/backup/import", h.ImportBackup)
	registerAppRoutes(auth, cfg)

	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/assets/") || strings.Contains(c.Request.URL.Path, ".") {
			serveSPA(c, cfg)
			return
		}
		session := sessions.Default(c)
		if session.Get("user") == nil {
			c.Redirect(http.StatusFound, "/login")
			return
		}
		serveSPA(c, cfg)
	})
	return r
}

func registerAppRoutes(auth *gin.RouterGroup, cfg Config) {
	for _, path := range []string{"/", "/scan", "/firmware", "/provision", "/groups", "/compliance", "/logs", "/settings", "/about"} {
		auth.GET(path, func(c *gin.Context) {
			serveSPA(c, cfg)
		})
	}
}

func serveSPA(c *gin.Context, cfg Config) {
	if cfg.HasStatic {
		sub, err := fs.Sub(cfg.StaticFS, "dist")
		if err == nil {
			requestPath := strings.TrimPrefix(path.Clean(c.Request.URL.Path), "/")
			if requestPath == "" || requestPath == "." {
				requestPath = "index.html"
			}
			if info, statErr := fs.Stat(sub, requestPath); statErr == nil && !info.IsDir() {
				http.FileServer(http.FS(sub)).ServeHTTP(c.Writer, c.Request)
				return
			}
			indexFile, openErr := sub.Open("index.html")
			if openErr == nil {
				defer indexFile.Close()
				body, readErr := io.ReadAll(indexFile)
				if readErr == nil {
					c.Data(http.StatusOK, "text/html; charset=utf-8", body)
					return
				}
			}
		}
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(`<!doctype html><html><body><div id="app">ShellyAdmin frontend not built yet.</div></body></html>`))
}
