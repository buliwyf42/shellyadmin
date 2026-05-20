package api

import (
	"net/http"
	"testing"
)

// TestChangeCredentialsRejectsWrongCurrentPassword ensures the current-password
// gate actually fires before any write.
func TestChangeCredentialsRejectsWrongCurrentPassword(t *testing.T) {
	r, cookie, csrf := tokensTestRouter(t)
	rec := doRequest(t, r, "POST", "/api/account/credentials", map[string]any{
		"current_password": "wrong-password",
		"username":         "",
		"new_password":     "brand-new-password",
	}, cookie, csrf, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body=%s", rec.Code, rec.Body.String())
	}
}

// TestChangeCredentialsRotatesPassword is the happy path: the new password
// works and the old one stops working.
func TestChangeCredentialsRotatesPassword(t *testing.T) {
	r, cookie, csrf := tokensTestRouter(t)
	rec := doRequest(t, r, "POST", "/api/account/credentials", map[string]any{
		"current_password": "correct-horse",
		"username":         "",
		"new_password":     "brand-new-password",
	}, cookie, csrf, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("change status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	if rec := postLogin(t, r, "admin", "brand-new-password"); rec.Code != http.StatusOK {
		t.Fatalf("login with new password = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if rec := postLogin(t, r, "admin", "correct-horse"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("login with old password = %d, want 401", rec.Code)
	}
}

// TestChangeCredentialsRenamesUser confirms a username change takes effect.
func TestChangeCredentialsRenamesUser(t *testing.T) {
	r, cookie, csrf := tokensTestRouter(t)
	rec := doRequest(t, r, "POST", "/api/account/credentials", map[string]any{
		"current_password": "correct-horse",
		"username":         "newname",
		"new_password":     "brand-new-password",
	}, cookie, csrf, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("change status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	if rec := postLogin(t, r, "newname", "brand-new-password"); rec.Code != http.StatusOK {
		t.Fatalf("login with new username = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if rec := postLogin(t, r, "admin", "brand-new-password"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("login with old username = %d, want 401", rec.Code)
	}
}

// TestChangeCredentialsRejectsShortPassword guards the usability floor.
func TestChangeCredentialsRejectsShortPassword(t *testing.T) {
	r, cookie, csrf := tokensTestRouter(t)
	rec := doRequest(t, r, "POST", "/api/account/credentials", map[string]any{
		"current_password": "correct-horse",
		"username":         "",
		"new_password":     "short",
	}, cookie, csrf, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}
