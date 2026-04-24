package services

import (
	"net/netip"
	"testing"
	"time"

	"shellyadmin/internal/models"
)

func boolPtr(v bool) *bool { return &v }

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

func TestValidateSettingsAcceptsExtendedComplianceOptions(t *testing.T) {
	port := 0
	err := ValidateSettings(models.AppSettings{
		Subnets:         []string{"192.168.1.0/30"},
		EnableMDNS:      false,
		ScanTimeout:     2,
		RefreshTimeout:  5,
		ScanConcurrency: 64,
		Compliance: models.ComplianceRules{
			WSTLSMode:     "user",
			WSSSLCa:       "ca.pem",
			BLERPCEnabled: boolPtr(true),
			BLEObserver:   boolPtr(false),
			RPCUDPPort:    &port,
		},
	})
	if err != nil {
		t.Fatalf("ValidateSettings() error = %v", err)
	}
}

func TestSaveCredentialGroupUsesAdminCompatibilityUsername(t *testing.T) {
	database, service := testService(t)

	err := service.SaveCredentialGroup(models.CredentialGroup{
		Name:     "site-a",
		Password: "secret-pass",
		Tags:     []string{"demo"},
	})
	if err != nil {
		t.Fatalf("SaveCredentialGroup() error = %v", err)
	}

	groups, err := service.ListCredentialGroups()
	if err != nil {
		t.Fatalf("ListCredentialGroups() error = %v", err)
	}
	if len(groups) != 1 || groups[0].Name != "site-a" {
		t.Fatalf("unexpected groups = %#v", groups)
	}

	credentials, err := database.ListCredentials()
	if err != nil {
		t.Fatalf("ListCredentials() error = %v", err)
	}
	if len(credentials) != 1 {
		t.Fatalf("credentials count = %d, want 1", len(credentials))
	}
	if credentials[0].Username != "admin" {
		t.Fatalf("credential username = %q, want admin", credentials[0].Username)
	}
}

func TestIsProvisionTargetAllowed(t *testing.T) {
	cases := []struct {
		addr    string
		allowed bool
	}{
		// Allowed: RFC1918 private
		{"192.168.1.1", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		// Allowed: link-local
		{"169.254.1.1", true},
		// Allowed: IPv6 ULA
		{"fd00::1", true},
		// Allowed: IPv6 link-local
		{"fe80::1", true},
		// Blocked: loopback
		{"127.0.0.1", false},
		{"::1", false},
		// Blocked: unspecified
		{"0.0.0.0", false},
		// Blocked: multicast
		{"224.0.0.1", false},
		{"ff02::1", false},
		// Blocked: public internet
		{"8.8.8.8", false},
		{"1.1.1.1", false},
	}
	for _, tc := range cases {
		addr := netip.MustParseAddr(tc.addr)
		got := isProvisionTargetAllowed(addr)
		if got != tc.allowed {
			t.Errorf("isProvisionTargetAllowed(%q) = %v, want %v", tc.addr, got, tc.allowed)
		}
	}
}
