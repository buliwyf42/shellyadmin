package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"shellyadmin/internal/services"
	"shellyadmin/internal/services/totp"
)

// enrollTOTPForTests mints a fresh TOTP enrollment for `admin` against
// the handler's live service. Returns the secret + backup codes the
// caller will need to mint codes / hit recovery paths.
func enrollTOTPForTests(t *testing.T, h *Handler) (secret string, backupCodes []string) {
	t.Helper()
	mat, err := h.service.BeginTOTPEnrollment("admin")
	if err != nil {
		t.Fatalf("BeginTOTPEnrollment: %v", err)
	}
	code, err := totp.Generate(mat.Secret, time.Now().UTC())
	if err != nil {
		t.Fatalf("totp.Generate: %v", err)
	}
	if err := h.service.CompleteTOTPEnrollment("admin", mat.Secret, mat.BackupHashesJSON, code); err != nil {
		t.Fatalf("CompleteTOTPEnrollment: %v", err)
	}
	return mat.Secret, mat.BackupCodes
}

// postLoginWithTOTP is the variant of postLogin that includes the
// optional totp_code field. Empty values omit the field on the wire so
// the assertion exercises the "field absent" branch faithfully.
func postLoginWithTOTP(t *testing.T, r *gin.Engine, username, password, code string) *httptest.ResponseRecorder {
	t.Helper()
	payload := map[string]string{"username": username, "password": password}
	if code != "" {
		payload["totp_code"] = code
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/api/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func decodeErrorField(t *testing.T, body []byte) string {
	t.Helper()
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("decode body: %v (body=%s)", err, string(body))
	}
	s, _ := parsed["error"].(string)
	return s
}

// TestLoginTOTPRequiredWhenEnrolledAndCodeMissing locks in the SPA's
// two-step entry signal: right password + no code = 401 totp_required,
// no lockout bump.
func TestLoginTOTPRequiredWhenEnrolledAndCodeMissing(t *testing.T) {
	r, h := loginTestSetup(t)
	enrollTOTPForTests(t, h)

	rec := postLoginWithTOTP(t, r, "admin", "correct-horse", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body=%s", rec.Code, rec.Body.String())
	}
	if got := decodeErrorField(t, rec.Body.Bytes()); got != "totp_required" {
		t.Errorf("error = %q, want %q", got, "totp_required")
	}
	// Empty-code is mid-flow, not a failed login → lockout counter must
	// not move.
	if locked, _ := h.service.IsAccountLocked("admin"); locked {
		t.Errorf("empty totp_code bumped lockout counter")
	}
}

// TestLoginTOTPWrongCodeBumpsLockout locks in that brute-forcing the
// 6-digit code is bounded by the same LoginMaxFailures budget that
// gates the password — otherwise the 10^6 search space against a
// stable secret would be reachable on a stolen-password threat model.
func TestLoginTOTPWrongCodeBumpsLockout(t *testing.T) {
	r, h := loginTestSetup(t)
	enrollTOTPForTests(t, h)

	rec := postLoginWithTOTP(t, r, "admin", "correct-horse", "000000")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body=%s", rec.Code, rec.Body.String())
	}
	if got := decodeErrorField(t, rec.Body.Bytes()); got != "invalid_totp_code" {
		t.Errorf("error = %q, want %q", got, "invalid_totp_code")
	}
	// Failure was recorded — exhausting the budget eventually locks.
	for i := 0; i < services.LoginMaxFailures; i++ {
		_ = postLoginWithTOTP(t, r, "admin", "correct-horse", "000000")
	}
	locked, _ := h.service.IsAccountLocked("admin")
	if !locked {
		t.Errorf("expected lockout after LoginMaxFailures wrong totp codes")
	}
}

// TestLoginTOTPSucceedsWithFreshCode is the happy-path: right password
// + a valid TOTP code clears both gates and issues a session.
func TestLoginTOTPSucceedsWithFreshCode(t *testing.T) {
	r, h := loginTestSetup(t)
	secret, _ := enrollTOTPForTests(t, h)

	code, err := totp.Generate(secret, time.Now().UTC())
	if err != nil {
		t.Fatalf("totp.Generate: %v", err)
	}
	rec := postLoginWithTOTP(t, r, "admin", "correct-horse", code)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
}

// TestLoginTOTPAcceptsBackupCode locks in the recovery path: an unused
// backup code stands in for the TOTP code when the operator's
// authenticator app is lost.
func TestLoginTOTPAcceptsBackupCode(t *testing.T) {
	r, h := loginTestSetup(t)
	_, backupCodes := enrollTOTPForTests(t, h)

	rec := postLoginWithTOTP(t, r, "admin", "correct-horse", backupCodes[0])
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	// Same backup code can NOT be reused — the bitmask burns the bit.
	rec = postLoginWithTOTP(t, r, "admin", "correct-horse", backupCodes[0])
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("re-use status = %d, want 401, body=%s", rec.Code, rec.Body.String())
	}
	if got := decodeErrorField(t, rec.Body.Bytes()); got != "invalid_totp_code" {
		t.Errorf("re-use error = %q, want %q", got, "invalid_totp_code")
	}
}

// TestLoginPasswordOnlyStillWorksWhenNotEnrolled guards the "escape
// hatch" promise from the plan: an operator who hasn't enrolled can
// still log in with just a password. Without this branch, the very
// first login after a fresh install couldn't happen.
func TestLoginPasswordOnlyStillWorksWhenNotEnrolled(t *testing.T) {
	r, _ := loginTestSetup(t)
	rec := postLoginWithTOTP(t, r, "admin", "correct-horse", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
}

// TestLoginWrongPasswordSkipsTOTPLookup is a defense-in-depth check —
// the TOTP status query must NEVER fire for a failed password attempt,
// since that would leak "user is enrolled" to an unauthenticated
// attacker via a side-channel (e.g. an upstream DB query timing).
func TestLoginWrongPasswordSkipsTOTPLookup(t *testing.T) {
	r, h := loginTestSetup(t)
	enrollTOTPForTests(t, h)

	rec := postLoginWithTOTP(t, r, "admin", "wrong-password", "000000")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body=%s", rec.Code, rec.Body.String())
	}
	// The response body must say "invalid credentials" — NOT
	// invalid_totp_code or totp_required, which would imply the
	// handler reached the TOTP branch.
	if got := decodeErrorField(t, rec.Body.Bytes()); got != "invalid credentials" {
		t.Errorf("error = %q, want %q (TOTP branch reachable on wrong password)", got, "invalid credentials")
	}
}
