package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

func RequireCSRF() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		nonce, _ := session.Get("nonce").(string)
		if nonce != "" {
			c.Header("X-CSRF-Token", nonce)
		}

		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}

		if nonce == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing session nonce"})
			return
		}
		token := c.GetHeader("X-CSRF-Token")
		if subtle.ConstantTimeCompare([]byte(token), []byte(nonce)) != 1 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "invalid csrf token"})
			return
		}
		c.Next()
	}
}
