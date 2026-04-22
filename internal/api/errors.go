package api

import (
	"fmt"

	"github.com/gin-gonic/gin"
)

// respondError writes a sanitized JSON error to the client and logs the
// underlying err via the handler's request-aware log path. publicMsg is the
// string returned to the client; err (if non-nil) is logged in full but never
// echoed, so internal details (stack traces, database quirks, filesystem
// paths) do not leak to authenticated callers.
//
// Use this for 5xx responses and any 4xx where the underlying error is not
// already phrased for end users.
func (h *Handler) respondError(c *gin.Context, status int, publicMsg string, err error) {
	if err != nil {
		h.logReq(c, "ERROR", fmt.Sprintf("[http] %s %s -> %d: %s: %v",
			c.Request.Method, c.Request.URL.Path, status, publicMsg, err))
	}
	c.JSON(status, gin.H{"error": publicMsg})
}

// respondUserError is for 4xx responses where err.Error() is already phrased
// as user guidance (validation messages, "target required", sentinel errors
// from the services layer). The full error is still logged so the operator
// can trace misuse.
func (h *Handler) respondUserError(c *gin.Context, status int, err error) {
	h.logReq(c, "DEBUG", fmt.Sprintf("[http] %s %s -> %d: %v",
		c.Request.Method, c.Request.URL.Path, status, err))
	c.JSON(status, gin.H{"error": err.Error()})
}
