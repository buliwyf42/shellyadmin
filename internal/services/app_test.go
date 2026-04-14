package services

import (
	"testing"
	"time"

	"shellyadmin/internal/models"
)

func TestRefreshProbeTimeoutUsesMinimum(t *testing.T) {
	got := refreshProbeTimeout(models.AppSettings{RefreshTimeout: 5})
	if got != 5*time.Second {
		t.Fatalf("refreshProbeTimeout() = %v, want %v", got, 5*time.Second)
	}
}

func TestRefreshProbeTimeoutHonorsHigherSetting(t *testing.T) {
	got := refreshProbeTimeout(models.AppSettings{RefreshTimeout: 7.5})
	if got != 7500*time.Millisecond {
		t.Fatalf("refreshProbeTimeout() = %v, want %v", got, 7500*time.Millisecond)
	}
}
