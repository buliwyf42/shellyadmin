package services

import (
	"context"
	"strings"
	"testing"
)

// --- UploadUserCA input validation ---
//
// These tests exercise only the service-layer guards that return before any
// HTTP traffic would happen. Actual RPC transport is covered by provisioner
// tests (internal/core/provisioner/user_ca_test.go) and hardware verification
// is described in the follow-up plan.

const validPEMForTests = "-----BEGIN CERTIFICATE-----\nabcd\n-----END CERTIFICATE-----\n"

func TestUploadUserCARejectsEmptyIPs(t *testing.T) {
	_, service := testService(t)
	_, err := service.UploadUserCA(context.Background(), nil, "", validPEMForTests)
	if err == nil || !strings.Contains(err.Error(), "ips required") {
		t.Fatalf("expected 'ips required' error, got %v", err)
	}
}

func TestUploadUserCARejectsTooManyIPs(t *testing.T) {
	_, service := testService(t)
	ips := make([]string, maxProvisionIPs+1)
	for i := range ips {
		ips[i] = "192.168.1.1"
	}
	_, err := service.UploadUserCA(context.Background(), ips, "", validPEMForTests)
	if err == nil || !strings.Contains(err.Error(), "too many devices") {
		t.Fatalf("expected 'too many devices' error, got %v", err)
	}
}

func TestUploadUserCARejectsUnknownKind(t *testing.T) {
	_, service := testService(t)
	_, err := service.UploadUserCA(context.Background(), []string{"192.168.1.10"}, "bogus_kind", validPEMForTests)
	if err == nil || !strings.Contains(err.Error(), "unknown certificate kind") {
		t.Fatalf("expected unknown-kind error, got %v", err)
	}
}

func TestUploadUserCARejectsEmptyPEM(t *testing.T) {
	_, service := testService(t)
	_, err := service.UploadUserCA(context.Background(), []string{"192.168.1.10"}, "", "   \n")
	if err == nil || !strings.Contains(err.Error(), "pem is required") {
		t.Fatalf("expected 'pem is required' error, got %v", err)
	}
}

func TestUploadUserCARejectsPEMWithoutHeader(t *testing.T) {
	_, service := testService(t)
	_, err := service.UploadUserCA(context.Background(), []string{"192.168.1.10"}, "", "garbage data no header")
	if err == nil || !strings.Contains(err.Error(), "pem must contain a PEM header") {
		t.Fatalf("expected missing-header error, got %v", err)
	}
}

func TestUploadUserCARejectsOversizedPEM(t *testing.T) {
	_, service := testService(t)
	pem := "-----BEGIN CERTIFICATE-----\n" + strings.Repeat("A", MaxUserCABytes) + "\n-----END CERTIFICATE-----\n"
	_, err := service.UploadUserCA(context.Background(), []string{"192.168.1.10"}, "", pem)
	if err == nil || !strings.Contains(err.Error(), "pem exceeds") {
		t.Fatalf("expected 'pem exceeds' error, got %v", err)
	}
}

func TestUploadUserCARejectsInvalidIP(t *testing.T) {
	_, service := testService(t)
	_, err := service.UploadUserCA(context.Background(), []string{"not-an-ip"}, "", validPEMForTests)
	if err == nil || !strings.Contains(err.Error(), "invalid ip") {
		t.Fatalf("expected 'invalid ip' error, got %v", err)
	}
}

func TestUploadUserCARejectsNonLocalIP(t *testing.T) {
	_, service := testService(t)
	_, err := service.UploadUserCA(context.Background(), []string{"8.8.8.8"}, "", validPEMForTests)
	if err == nil || !strings.Contains(err.Error(), "not in an allowed local range") {
		t.Fatalf("expected non-local range error, got %v", err)
	}
}

// TestUploadUserCASkipsDeviceBusyWithFirmware verifies the concurrency guard:
// if a firmware update holds a reservation on the target IP's MAC, the
// upload returns a "skipped" entry instead of racing.
func TestUploadUserCASkipsDeviceBusyWithFirmware(t *testing.T) {
	_, service := testService(t)
	// Claim the target as if firmware were running on it. reserveFirmwareTargets
	// accepts raw keys — Provision/UploadUserCA use "mac:<MAC>" for known
	// devices; for an unknown IP like this we use "ip:<addr>".
	busyKey := "ip:192.168.1.200"
	if allowed, _ := service.reserveFirmwareTargets([]string{busyKey}); len(allowed) != 1 {
		t.Fatalf("reserveFirmwareTargets failed to claim the test key")
	}
	defer service.releaseFirmwareTargets([]string{busyKey})

	results, err := service.UploadUserCA(context.Background(), []string{"192.168.1.200"}, "", validPEMForTests)
	if err != nil {
		t.Fatalf("UploadUserCA error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results len = %d, want 1", len(results))
	}
	if results[0].Status != "skipped" {
		t.Fatalf("results[0].Status = %q, want skipped", results[0].Status)
	}
	if !strings.Contains(results[0].Detail, "firmware") {
		t.Fatalf("results[0].Detail = %q, want it to mention firmware", results[0].Detail)
	}
}
