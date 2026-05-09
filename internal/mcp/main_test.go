package mcp

import (
	"os"
	"testing"

	"shellyadmin/internal/core/secretbox"
)

// TestMain installs a secretbox key for the whole package so any test
// that round-trips a credential through the DB layer doesn't fail with
// "key not initialized". Mirrors internal/services/main_test.go.
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
