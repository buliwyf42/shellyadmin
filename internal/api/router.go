package api

import (
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
	"shellyadmin/internal/services"
)

type Config struct {
	User string
	// PassHash is the argon2id PHC string for the admin password.
	// Generate with `shellyctl hash-password`.
	PassHash       string
	Secret         string
	CookieSecure   bool
	DataDir        string
	BackendVersion string
	BackendCommit  string
	StaticFS       fs.FS
	HasStatic      bool
	DevStatic      string
	// Service is the shared AppService instance that backs every handler.
	// When non-nil, NewHandler skips its own services.NewAppService() call
	// and reuses this one instead — required so background workers, the
	// MCP controller, and HTTP handlers all see the same in-memory state.
	// When nil, NewHandler still constructs its own (back-compat for tests
	// that don't need to share state with main.go).
	Service *services.AppService
}

func NewRouter(database *db.DB, cfg Config) *gin.Engine {
	r := gin.Default()
	r.Use(middleware.RequestID())
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

	r.GET("/login", func(c *gin.Context) {
		session := sessions.Default(c)
		if session.Get("user") != nil {
			c.Redirect(http.StatusFound, "/")
			return
		}
		serveSPAIndex(c, cfg)
	})
	r.POST("/login", middleware.LoginRateLimit(), h.Login)
	r.POST("/logout", middleware.RequireAuth(), middleware.RequireCSRF(), h.Logout)

	auth := r.Group("/")
	auth.Use(middleware.RequireAuth())
	auth.Use(middleware.APIRateLimit())
	auth.Use(middleware.RequireCSRF())
	registerDocumentedAPIRoutes(r, auth, h)
	registerAppRoutes(auth, cfg)

	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		if isStaticAssetPath(c.Request.URL.Path) {
			serveStaticOr404(c, cfg)
			return
		}
		session := sessions.Default(c)
		if session.Get("user") == nil {
			c.Redirect(http.StatusFound, "/login")
			return
		}
		serveSPAIndex(c, cfg)
	})
	return r
}

func registerAppRoutes(auth *gin.RouterGroup, cfg Config) {
	for _, path := range []string{"/", "/scan", "/firmware", "/provision", "/groups", "/compliance", "/logs", "/settings", "/about", "/docs", "/devices/:target"} {
		auth.GET(path, func(c *gin.Context) {
			serveSPAIndex(c, cfg)
		})
	}
}

func isStaticAssetPath(requestPath string) bool {
	if strings.HasPrefix(requestPath, "/assets/") {
		return true
	}
	cleanPath := path.Clean(requestPath)
	return path.Ext(cleanPath) != ""
}

func staticSubFS(cfg Config) (fs.FS, error) {
	if !cfg.HasStatic || cfg.StaticFS == nil {
		return nil, fs.ErrNotExist
	}
	return fs.Sub(cfg.StaticFS, "dist")
}

func cleanStaticPath(requestPath string) string {
	cleanPath := strings.TrimPrefix(path.Clean(requestPath), "/")
	if cleanPath == "" || cleanPath == "." {
		return "index.html"
	}
	return cleanPath
}

func serveStaticOr404(c *gin.Context, cfg Config) {
	sub, err := staticSubFS(cfg)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	requestPath := cleanStaticPath(c.Request.URL.Path)
	info, statErr := fs.Stat(sub, requestPath)
	if statErr != nil || info.IsDir() {
		c.Status(http.StatusNotFound)
		return
	}
	http.FileServer(http.FS(sub)).ServeHTTP(c.Writer, c.Request)
}

func serveSPAIndex(c *gin.Context, cfg Config) {
	sub, err := staticSubFS(cfg)
	if err == nil {
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
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(`<!doctype html><html><body><div id="app">ShellyAdmin frontend not built yet.</div></body></html>`))
}
