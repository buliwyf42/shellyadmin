package scanner

import (
	"net/netip"
	"strings"
	"testing"
)

func TestExpandCIDR_AcceptsPrivateRanges(t *testing.T) {
	cases := []string{
		"192.168.1.0/24",
		"10.0.0.0/24",
		"172.16.0.0/24",
		"169.254.0.0/24",
	}
	for _, cidr := range cases {
		ips, err := ExpandCIDR(cidr)
		if err != nil {
			t.Errorf("ExpandCIDR(%s) unexpected error: %v", cidr, err)
			continue
		}
		if len(ips) == 0 {
			t.Errorf("ExpandCIDR(%s) returned no hosts", cidr)
		}
	}
}

func TestExpandCIDR_RejectsPublicRanges(t *testing.T) {
	cases := []string{
		"0.0.0.0/0",
		"1.1.1.0/24",
		"8.8.8.0/24",
	}
	for _, cidr := range cases {
		if _, err := ExpandCIDR(cidr); err == nil {
			t.Errorf("ExpandCIDR(%s) accepted a public CIDR; expected rejection", cidr)
		}
	}
}

func TestExpandCIDR_RejectsOversizedSubnet(t *testing.T) {
	// /16 in private space is allowed-by-range but exceeds the 1024-host cap.
	if _, err := ExpandCIDR("192.168.0.0/16"); err == nil {
		t.Errorf("ExpandCIDR(/16) accepted; expected host-count rejection")
	} else if !strings.Contains(err.Error(), "more than") {
		t.Errorf("unexpected error shape for /16: %v", err)
	}
}

func TestExpandCIDR_RejectsSpecialPurposeRanges(t *testing.T) {
	cases := []string{
		"127.0.0.0/24", // loopback
		"224.0.0.0/24", // multicast
	}
	for _, cidr := range cases {
		if _, err := ExpandCIDR(cidr); err == nil {
			t.Errorf("ExpandCIDR(%s) accepted; expected rejection", cidr)
		}
	}
}

func TestIsAllowedScanNetwork(t *testing.T) {
	cases := []struct {
		addr string
		want bool
	}{
		{"192.168.1.0", true},
		{"10.0.0.0", true},
		{"172.16.0.0", true},
		{"169.254.0.0", true},
		{"127.0.0.0", false},
		{"224.0.0.0", false},
		{"0.0.0.0", false},
		{"1.1.1.1", false},
		{"8.8.8.8", false},
	}
	for _, c := range cases {
		addr, err := netip.ParseAddr(c.addr)
		if err != nil {
			t.Fatalf("parse %s: %v", c.addr, err)
		}
		if got := IsAllowedScanNetwork(addr); got != c.want {
			t.Errorf("IsAllowedScanNetwork(%s) = %v, want %v", c.addr, got, c.want)
		}
	}
}
