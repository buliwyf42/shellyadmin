package scanner

import (
	"testing"

	"shellyadmin/internal/models"
)

func TestExtractPowerReadings_SwitchAndEM(t *testing.T) {
	status := map[string]any{
		"switch:0": map[string]any{
			"apower":  120.5,
			"voltage": 230.0,
			"current": 0.52,
		},
		"em:0": map[string]any{
			"total_act_power": 1500.0,
			"total_current":   6.5,
			"a_voltage":       228.0,
			"b_voltage":       232.0,
			"c_voltage":       0.0,
		},
		"sys": map[string]any{}, // ignored
	}
	dev := &models.Device{}
	extractPowerReadings(status, dev)
	if dev.PowerW == nil || *dev.PowerW < 1620.4 || *dev.PowerW > 1620.6 {
		t.Errorf("PowerW = %v, want ~1620.5", dev.PowerW)
	}
	if dev.CurrentA == nil || *dev.CurrentA < 7.01 || *dev.CurrentA > 7.03 {
		t.Errorf("CurrentA = %v, want ~7.02", dev.CurrentA)
	}
	if dev.VoltageV == nil || *dev.VoltageV != 232.0 {
		t.Errorf("VoltageV = %v, want 232", dev.VoltageV)
	}
}

func TestExtractPowerReadings_NoComponents(t *testing.T) {
	status := map[string]any{
		"sys":   map[string]any{},
		"wifi":  map[string]any{"channel": float64(6)},
		"cloud": map[string]any{"connected": true},
	}
	dev := &models.Device{}
	extractPowerReadings(status, dev)
	if dev.PowerW != nil || dev.VoltageV != nil || dev.CurrentA != nil {
		t.Errorf("expected no readings, got W=%v V=%v A=%v", dev.PowerW, dev.VoltageV, dev.CurrentA)
	}
}

func TestExtractPowerReadings_PM1(t *testing.T) {
	status := map[string]any{
		"pm1:0": map[string]any{
			"apower":  50.0,
			"voltage": 240.0,
			"current": 0.21,
		},
	}
	dev := &models.Device{}
	extractPowerReadings(status, dev)
	if dev.PowerW == nil || *dev.PowerW != 50.0 {
		t.Errorf("PowerW = %v, want 50", dev.PowerW)
	}
	if dev.VoltageV == nil || *dev.VoltageV != 240.0 {
		t.Errorf("VoltageV = %v, want 240", dev.VoltageV)
	}
}
