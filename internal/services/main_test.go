package services

import (
	"os"
	"testing"

	"shellyadmin/internal/core/secretbox"
)

// TestMain installs a fixed secretbox key once for the whole package so tests
// that persist credentials through DB.SaveCredential* don't fail with
// "key not initialized". Production wiring lives in cmd/shellyctl/main.go.
func TestMain(m *testing.M) {
	key, err := secretbox.GenerateKey()
	if err != nil {
		panic(err)
	}
	if err := secretbox.SetKey(key); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
