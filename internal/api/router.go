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
	// TrustedProxies is a comma-separated list of CIDR ranges whose
	// X-Forwarded-For header will be trusted by gin.ClientIP(). Empty (the
	// default) means no proxies are trusted — ClientIP returns the direct
	// peer. Without this, an attacker on the LAN can spoof client_ip in
	// audit rows by setting X-Forwarded-For on the request.
	TrustedProxies string
}

func NewRouter(database *db.DB, cfg Config) *gin.Engine {
	// gin.New() + explicit middlewares replaces gin.Default(), which would
	// install gin.Logger() (writes to stdout in its own format, no slog
	// integration, no query-string sanitization). middleware.RequestID must
	// run first so subsequent middlewares (including StructuredLogger) see
	// the request ID via context.
	r := gin.New()
	// TrustedProxies: empty list disables X-Forwarded-For trust entirely
	// (gin's default trusts ALL proxies which is wrong for LAN deploys).
	// Errors here only happen on malformed CIDRs; log + zero them out so
	// startup doesn't fail on a typo, but the operator sees the warning.
	proxies := []string{}
	if cfg.TrustedProxies != "" {
		for _, p := range strings.Split(cfg.TrustedProxies, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				proxies = append(proxies, p)
			}
		}
	}
	if err := r.SetTrustedProxies(proxies); err != nil {
		// gin returns an error on malformed entries — fall back to
		// "trust nothing" rather than panic at startup.
		_ = r.SetTrustedProxies(nil)
	}
	r.Use(gin.Recovery())
	r.Use(middleware.RequestID())
	r.Use(middleware.StructuredLogger())
	r.Use(middleware.SecurityHeaders())
	store := cookie.NewStore([]byte(cfg.Secret))
	store.Options(sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7,
		HttpOnly: true,
		Secure:   cfg.CookieSecure,
		// SameSite=Strict (was Lax) prevents the session cookie from being
		// sent on cross-site top-level navigations entirely. Combined with
		// the CSRF middleware this raises the bar for cross-site request
		// forgery — a victim opening an attacker's `<form action=
		// "https://shellyadmin/api/...">` no longer leaks the cookie. The
		// trade-off is that links into ShellyAdmin from external sites
		// (e.g. a bookmark sent in chat) require the user to follow the
		// link and re-authenticate on the same tab session. Acceptable for
		// a Single-Operator admin tool.
		SameSite: http.SameSiteStrictMode,
	})
	r.Use(sessions.Sessions("shellyadmin", store))

	h := NewHandler(database, cfg)

	// S5 — RequireAuth now consults the server-side session store via
	// AppService.SessionValidator(). Cookie alone is no longer
	// sufficient; the session id baked into the cookie must point to
	// an un-revoked, un-expired row in `sessions`. Tests that build a
	// router without an AppService (Service == nil) fall back to
	// cookie-only auth via a nil validator, keeping the regression
	// surface manageable.
	//
	// T3 — RequireAuth also accepts PAT bearer tokens. The patValidator
	// is the *AppService itself (it satisfies middleware.PATValidator
	// via the LookupPAT method); nil for tests that don't wire the
	// service.
	var validator middleware.SessionValidator
	var patValidator middleware.PATValidator
	if h.service != nil {
		validator = h.service.SessionValidator()
		patValidator = h.service
	}
	authMW := middleware.RequireAuthWithPAT(validator, patValidator)

	r.GET("/login", func(c *gin.Context) {
		session := sessions.Default(c)
		if session.Get("user") != nil {
			c.Redirect(http.StatusFound, "/")
			return
		}
		serveSPAIndex(c, cfg)
	})
	r.POST("/login", middleware.LoginRateLimit(), h.Login)
	r.POST("/logout", authMW, middleware.RequireCSRF(), h.Logout)

	auth := r.Group("/")
	auth.Use(authMW)
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
	// NoMethod: gin's default returns 405 with an empty body and the gin
	// HTML 405 page. Return JSON for /api/* so clients can parse it the
	// same way they parse other API errors.
	r.HandleMethodNotAllowed = true
	r.NoMethod(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.AbortWithStatusJSON(http.StatusMethodNotAllowed, gin.H{"error": "method not allowed"})
			return
		}
		c.AbortWithStatus(http.StatusMethodNotAllowed)
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
