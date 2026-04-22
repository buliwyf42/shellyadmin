package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestRequestIDGenerated(t *testing.T) {
	router := gin.New()
	router.Use(RequestID())
	var captured string
	router.GET("/ping", func(c *gin.Context) {
		captured = FromGinContext(c)
		if FromContext(c.Request.Context()) != captured {
			t.Fatalf("context disagrees with gin: %q vs %q", FromContext(c.Request.Context()), captured)
		}
		c.Status(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	router.ServeHTTP(rec, req)

	if captured == "" {
		t.Fatalf("expected generated request id, got empty")
	}
	if got := rec.Header().Get(HeaderRequestID); got != captured {
		t.Fatalf("response header %q does not match captured id %q", got, captured)
	}
	if len(captured) != 16 {
		t.Fatalf("expected 16-char hex id, got %q", captured)
	}
}

func TestRequestIDEchoesValidInbound(t *testing.T) {
	router := gin.New()
	router.Use(RequestID())
	var captured string
	router.GET("/ping", func(c *gin.Context) {
		captured = FromGinContext(c)
		c.Status(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set(HeaderRequestID, "abc-123_XYZ")
	router.ServeHTTP(rec, req)

	if captured != "abc-123_XYZ" {
		t.Fatalf("expected to echo inbound id, got %q", captured)
	}
}

func TestRequestIDRejectsInvalidInbound(t *testing.T) {
	router := gin.New()
	router.Use(RequestID())
	var captured string
	router.GET("/ping", func(c *gin.Context) {
		captured = FromGinContext(c)
		c.Status(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set(HeaderRequestID, "bad value with spaces")
	router.ServeHTTP(rec, req)

	if captured == "" || captured == "bad value with spaces" {
		t.Fatalf("expected regenerated id, got %q", captured)
	}
}

func TestRequestIDTruncatesLongInbound(t *testing.T) {
	router := gin.New()
	router.Use(RequestID())
	var captured string
	router.GET("/ping", func(c *gin.Context) {
		captured = FromGinContext(c)
		c.Status(http.StatusOK)
	})

	long := strings.Repeat("a", maxInboundLen+20)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set(HeaderRequestID, long)
	router.ServeHTTP(rec, req)

	if len(captured) != maxInboundLen {
		t.Fatalf("expected truncation to %d chars, got %d (%q)", maxInboundLen, len(captured), captured)
	}
}

func TestFromContextNilSafe(t *testing.T) {
	if FromContext(nil) != "" {
		t.Fatalf("FromContext(nil) should return empty")
	}
	if FromGinContext(nil) != "" {
		t.Fatalf("FromGinContext(nil) should return empty")
	}
}
