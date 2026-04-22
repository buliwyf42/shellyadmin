package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRespondError_SanitizesClientBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var logs []string
	h := &Handler{auditSink: func(level, msg, reqID string) { logs = append(logs, level+" "+msg) }}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/devices", nil)

	h.respondError(c, http.StatusInternalServerError, "internal error",
		errors.New("database: unable to open /var/data/secret.db: permission denied"))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if body["error"] != "internal error" {
		t.Errorf("client body error = %q, want %q", body["error"], "internal error")
	}
	if strings.Contains(rec.Body.String(), "secret.db") {
		t.Errorf("response body must not leak internal details, got %s", rec.Body.String())
	}

	if len(logs) != 1 {
		t.Fatalf("logs = %d, want 1", len(logs))
	}
	if !strings.Contains(logs[0], "secret.db") {
		t.Errorf("full error must be logged, got %q", logs[0])
	}
	if !strings.HasPrefix(logs[0], "ERROR ") {
		t.Errorf("respondError should log at ERROR, got %q", logs[0])
	}
}

func TestRespondUserError_EchoesMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var logs []string
	h := &Handler{auditSink: func(level, msg, reqID string) { logs = append(logs, level+" "+msg) }}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/settings", nil)

	h.respondUserError(c, http.StatusBadRequest, errors.New("scan_timeout must be positive"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if body["error"] != "scan_timeout must be positive" {
		t.Errorf("client body error = %q, want validation message echoed", body["error"])
	}

	if len(logs) != 1 || !strings.HasPrefix(logs[0], "DEBUG ") {
		t.Errorf("respondUserError should log at DEBUG, got %v", logs)
	}
}
