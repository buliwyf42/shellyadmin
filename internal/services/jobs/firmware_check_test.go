package jobs

import (
	"testing"

	"shellyadmin/internal/core/firmware"
	"shellyadmin/internal/models"
)

// A failed check used to be written as if it had succeeded: empty availability
// fields plus a fresh CheckedAt, and nothing at all about reachability. That is
// how a device with a stale IP kept reporting online:true with an empty
// last_refresh_error while every job against it failed.
func TestApplyCheckResultKeepsCacheAndMarksMissWhenUnreachable(t *testing.T) {
	device := models.Device{
		MAC: "AA:BB:CC:DD:EE:01", Online: true,
		FWAvailableStable: "2.0.1", FWAvailableBeta: "2.0.2-beta1",
		FWCheckedAt: "2026-09-12T08:00:00Z",
	}
	result := firmware.Result{
		Status: "error", Note: "no route to host", Unreachable: true,
		CheckedAt: "2026-09-12T09:00:00Z",
	}

	applyCheckResult(&device, result)
	if device.FWAvailableStable != "2.0.1" || device.FWAvailableBeta != "2.0.2-beta1" {
		t.Errorf("firmware cache blanked: stable=%q beta=%q", device.FWAvailableStable, device.FWAvailableBeta)
	}
	if device.FWCheckedAt != "2026-09-12T08:00:00Z" {
		t.Errorf("FWCheckedAt = %q, want the old timestamp — a failed check verified nothing", device.FWCheckedAt)
	}
	if device.LastRefreshOK || device.LastRefreshError != "no route to host" {
		t.Errorf("failure not recorded: ok=%v err=%q", device.LastRefreshOK, device.LastRefreshError)
	}
	if device.ConsecutiveMisses != 1 || !device.Online {
		t.Errorf("after one miss: misses=%d online=%v, want 1/true", device.ConsecutiveMisses, device.Online)
	}

	applyCheckResult(&device, result)
	if device.ConsecutiveMisses != 2 || device.Online {
		t.Errorf("after two misses: misses=%d online=%v, want 2/false", device.ConsecutiveMisses, device.Online)
	}
}

// An auth refusal is proof the device is alive — it must not count as a miss.
func TestApplyCheckResultKeepsDeviceOnlineWhenItRefuses(t *testing.T) {
	device := models.Device{MAC: "AA:BB:CC:DD:EE:01", Online: true, ConsecutiveMisses: 1, FWAvailableStable: "2.0.1"}
	applyCheckResult(&device, firmware.Result{Status: "error", Note: "authentication required", Unreachable: false})

	if !device.Online || device.ConsecutiveMisses != 0 {
		t.Errorf("online=%v misses=%d, want true/0 — the device answered", device.Online, device.ConsecutiveMisses)
	}
	if device.LastRefreshOK || device.LastRefreshError != "authentication required" {
		t.Errorf("failure not recorded: ok=%v err=%q", device.LastRefreshOK, device.LastRefreshError)
	}
	if device.FWAvailableStable != "2.0.1" {
		t.Errorf("firmware cache blanked on a refusal: %q", device.FWAvailableStable)
	}
}

func TestApplyCheckResultPersistsCacheAndClearsFailureOnSuccess(t *testing.T) {
	device := models.Device{
		MAC: "AA:BB:CC:DD:EE:01", Online: false, ConsecutiveMisses: 3,
		LastRefreshError: "no route to host", FWAvailableStable: "1.7.5",
	}
	applyCheckResult(&device, firmware.Result{
		Status: "ok", CurrentVer: "2.0.0", Batch: "b1", FWID: "fw1",
		StableVer: "2.0.1", BetaVer: "2.0.2-beta1", CheckedAt: "2026-09-12T09:00:00Z",
	})

	if device.FWAvailableStable != "2.0.1" || device.FWAvailableBeta != "2.0.2-beta1" {
		t.Errorf("cache not persisted: stable=%q beta=%q", device.FWAvailableStable, device.FWAvailableBeta)
	}
	if device.FWCheckedAt != "2026-09-12T09:00:00Z" {
		t.Errorf("FWCheckedAt = %q, want the new timestamp", device.FWCheckedAt)
	}
	if device.FW != "2.0.0" || device.Batch != "b1" || device.FWID != "fw1" {
		t.Errorf("identity fields not written: fw=%q batch=%q fwid=%q", device.FW, device.Batch, device.FWID)
	}
	if !device.LastRefreshOK || device.LastRefreshError != "" || device.ConsecutiveMisses != 0 || !device.Online {
		t.Errorf("failure state not cleared: ok=%v err=%q misses=%d online=%v",
			device.LastRefreshOK, device.LastRefreshError, device.ConsecutiveMisses, device.Online)
	}
}

// "na" is a gen1 short-circuit that never talked to the device: it must not
// move reachability in either direction.
func TestApplyCheckResultLeavesRowAloneOnNotApplicable(t *testing.T) {
	device := models.Device{MAC: "AA:BB:CC:DD:EE:01", Online: true, FWAvailableStable: "1.7.5", FWCheckedAt: "old"}
	applyCheckResult(&device, firmware.Result{Status: "na", Note: "gen1 devices not supported", CheckedAt: "new"})

	if device.FWAvailableStable != "1.7.5" || device.FWCheckedAt != "old" || !device.Online || device.ConsecutiveMisses != 0 {
		t.Errorf("row disturbed by an na result: %+v", device)
	}
}
