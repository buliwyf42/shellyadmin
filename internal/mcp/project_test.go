package mcp

import (
	"strings"
	"testing"

	"shellyadmin/internal/models"
)

func sampleViews() []models.DeviceListView {
	return []models.DeviceListView{
		{MAC: "AA:BB:CC:DD:EE:01", IP: "192.168.1.10", Name: "alpha", FW: "2.0.0", Gen: 3},
		{MAC: "AA:BB:CC:DD:EE:02", IP: "192.168.1.11", Name: "beta", FW: "1.7.5", Gen: 2},
	}
}

// The whole point of the allowlist: 59 keys per device is what overflows the
// MCP output cap, so a projection has to actually drop the rest.
func TestProjectViewsKeepsOnlyRequestedFields(t *testing.T) {
	rows, err := projectViews(sampleViews(), []string{"name", "fw"})
	if err != nil {
		t.Fatalf("projectViews: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	for _, row := range rows {
		if len(row) != 3 {
			t.Errorf("keys = %v, want exactly name+fw+mac", row)
		}
		if _, ok := row["mac"]; !ok {
			t.Error("mac must be kept even when not requested — a row without its key is unusable")
		}
		if _, ok := row["ip"]; ok {
			t.Error("ip was not requested and must be dropped")
		}
	}
	if rows[0]["name"] != "alpha" || rows[0]["fw"] != "2.0.0" {
		t.Errorf("values not carried through: %v", rows[0])
	}
}

func TestProjectViewsEmptyFieldsReturnsEverything(t *testing.T) {
	rows, err := projectViews(sampleViews(), nil)
	if err != nil {
		t.Fatalf("projectViews: %v", err)
	}
	if len(rows[0]) < 50 {
		t.Errorf("unfiltered row has %d keys, expected the full view (~59)", len(rows[0]))
	}
	// Blank/whitespace entries must not be mistaken for a filter.
	blank, err := projectViews(sampleViews(), []string{"  ", ""})
	if err != nil {
		t.Fatalf("projectViews(blank): %v", err)
	}
	if len(blank[0]) != len(rows[0]) {
		t.Errorf("blank field names acted as a filter: %d vs %d keys", len(blank[0]), len(rows[0]))
	}
}

// A typo must surface at the call, not as a silently missing column.
func TestProjectViewsRejectsUnknownField(t *testing.T) {
	_, err := projectViews(sampleViews(), []string{"name", "firmware"})
	if err == nil {
		t.Fatal("expected an error for the unknown field 'firmware'")
	}
	if !strings.Contains(err.Error(), "firmware") {
		t.Errorf("error must name the offending field: %v", err)
	}
	if !strings.Contains(err.Error(), "fw") {
		t.Errorf("error must list the valid fields so the caller can correct it: %v", err)
	}
}

func TestProjectViewsHandlesEmptyDeviceList(t *testing.T) {
	rows, err := projectViews(nil, []string{"whatever"})
	if err != nil {
		t.Fatalf("empty list must not error: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %v, want empty", rows)
	}
}
