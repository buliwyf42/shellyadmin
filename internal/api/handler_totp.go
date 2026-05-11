package api

import (
	"encoding/gob"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"

	"shellyadmin/internal/services"
)

// Session keys for the in-flight enrollment material. The plaintext
// secret + the JSON-encoded backup-hash list live here between Begin
// and Complete; they're cleared on commit, abandon, or logout (the
// session itself is cleared on logout, which transitively drops both).
const (
	totpPendingSecretKey = "totp_pending_secret"
	totpPendingHashesKey = "totp_pending_hashes"
)

func init() {
	// gin-contrib/sessions persists values via gob. Strings already
	// register themselves; this is here so a future shape change to a
	// struct surface won't silently fail at session.Save().
	gob.Register("")
}

// TOTPStatusResponse is the GET /api/totp/status body.
// Mirrors services.TOTPStatus but keeps the wire format owned by the
// api package (the service-layer struct already carries JSON tags so
// the type alias is enough — exported here for openapi documentation).
type TOTPStatusResponse = services.TOTPStatus

// TOTPEnrollResponse is the POST /api/totp/enroll body. The plaintext
// secret + backup codes are surfaced exactly once; the SPA is expected
// to render them into the QR canvas + downloadable text block and then
// drop them on a successful VerifyEnroll.
type TOTPEnrollResponse struct {
	Secret       string   `json:"secret"`
	OTPAuthURI   string   `json:"otpauth_uri"`
	BackupCodes  []string `json:"backup_codes"`
	QRCodeFormat string   `json:"qrcode_format"`
}

// GetTOTPStatus returns the operator's enrollment summary. The Settings
// UI calls this on every render of the 2FA card to decide between the
// "Enroll" and "Disable" buttons.
func (h *Handler) GetTOTPStatus(c *gin.Context) {
	username, ok := h.sessionUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	status, err := h.service.TOTPStatusFor(username)
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "internal error", err)
		return
	}
	c.JSON(http.StatusOK, status)
}

// EnrollTOTP mints a fresh secret + backup codes for the operator,
// stashes the pending material in the session cookie (so the secret
// never lands in the DB until verified), and returns the QR-friendly
// payload. Calling Enroll a second time before VerifyEnroll discards
// the previous pending secret — the operator can only have ONE in-
// flight enrollment at a time.
func (h *Handler) EnrollTOTP(c *gin.Context) {
	username, ok := h.sessionUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	// Reject when the operator already has an active row — the only path
	// to a new secret is Disable first. Prevents a stolen-cookie attacker
	// from rotating the secret out from under the legitimate operator.
	current, err := h.service.TOTPStatusFor(username)
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "internal error", err)
		return
	}
	if current.Enrolled {
		c.JSON(http.StatusConflict, gin.H{"error": "already enrolled"})
		return
	}
	material, err := h.service.BeginTOTPEnrollment(username)
	if err != nil {
		h.respondError(c, http.StatusInternalServerError, "enrollment generation failed", err)
		return
	}
	session := sessions.Default(c)
	session.Set(totpPendingSecretKey, material.Secret)
	session.Set(totpPendingHashesKey, string(material.BackupHashesJSON))
	if err := session.Save(); err != nil {
		h.respondError(c, http.StatusInternalServerError, "session persistence failed", err)
		return
	}
	c.JSON(http.StatusOK, TOTPEnrollResponse{
		Secret:       material.Secret,
		OTPAuthURI:   material.OTPAuthURI,
		BackupCodes:  material.BackupCodes,
		QRCodeFormat: "otpauth",
	})
}

// VerifyEnrollTOTP commits the in-flight enrollment after the operator
// supplies a TOTP code from their authenticator app. Reads the pending
// secret + hashes out of the session, runs the verify-and-commit, and
// clears the pending fields on success.
func (h *Handler) VerifyEnrollTOTP(c *gin.Context) {
	username, ok := h.sessionUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(c, &req, 4*1024); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	session := sessions.Default(c)
	secret, _ := session.Get(totpPendingSecretKey).(string)
	hashesJSON, _ := session.Get(totpPendingHashesKey).(string)
	if secret == "" || hashesJSON == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no pending enrollment"})
		return
	}
	if err := h.service.CompleteTOTPEnrollment(username, secret, []byte(hashesJSON), req.Code); err != nil {
		if errors.Is(err, services.ErrTOTPInvalidCode) {
			h.logReq(c, "WARN", "totp enroll: invalid code")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid code"})
			return
		}
		h.respondError(c, http.StatusInternalServerError, "enrollment commit failed", err)
		return
	}
	// Wipe pending fields so a leftover session doesn't keep a sealed
	// secret around after commit. session.Save propagates the deletion.
	session.Delete(totpPendingSecretKey)
	session.Delete(totpPendingHashesKey)
	if err := session.Save(); err != nil {
		h.logReq(c, "WARN", fmt.Sprintf("totp enroll: session save after commit: %v", err))
	}
	h.logReq(c, "INFO", "totp enrollment completed")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DisableTOTP revokes the operator's enrollment. Requires a fresh TOTP
// or unused backup code so a stolen session cookie cannot quietly
// turn 2FA off (the password is already on the attacker's side; the
// second-factor check is the only remaining gate).
func (h *Handler) DisableTOTP(c *gin.Context) {
	username, ok := h.sessionUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(c, &req, 4*1024); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if err := h.service.DisableTOTP(username, req.Code); err != nil {
		switch {
		case errors.Is(err, services.ErrTOTPNotEnrolled):
			c.JSON(http.StatusNotFound, gin.H{"error": "not enrolled"})
		case errors.Is(err, services.ErrTOTPInvalidCode):
			h.logReq(c, "WARN", "totp disable: invalid code")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid code"})
		default:
			h.respondError(c, http.StatusInternalServerError, "disable failed", err)
		}
		return
	}
	h.logReq(c, "INFO", "totp disabled")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// sessionUser pulls the operator identity off the session cookie. The
// auth middleware already rejected unauthenticated requests, so the
// "user" key should always be set — the fallback guard is defense-in-
// depth against a future middleware re-shuffle.
func (h *Handler) sessionUser(c *gin.Context) (string, bool) {
	username, ok := sessions.Default(c).Get("user").(string)
	if !ok || username == "" {
		return "", false
	}
	return username, true
}
