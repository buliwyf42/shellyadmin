package services

import (
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
)

func TestExportLogsCSVShape(t *testing.T) {
	database, service := testService(t)

	if err := database.AddLog("INFO", "one"); err != nil {
		t.Fatalf("AddLog() error = %v", err)
	}
	if err := database.AddLog("ERROR", "two, with comma"); err != nil {
		t.Fatalf("AddLog() error = %v", err)
	}

	body, filename, contentType, err := service.ExportLogs("", "", "csv")
	if err != nil {
		t.Fatalf("ExportLogs() error = %v", err)
	}
	if !strings.HasSuffix(filename, ".csv") {
		t.Fatalf("filename = %q, want .csv suffix", filename)
	}
	if !strings.HasPrefix(contentType, "text/csv") {
		t.Fatalf("contentType = %q, want text/csv prefix", contentType)
	}

	reader := csv.NewReader(strings.NewReader(string(body)))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("csv parse error = %v", err)
	}
	if len(records) < 3 {
		t.Fatalf("records = %d, want >= 3 (header + 2 rows)", len(records))
	}
	// Header order: id, ts, level, risk_level, request_id, message.
	// risk_level inserted in v0.1.10 between level and request_id; CSV
	// consumers see an extra column, name change is forward-compat.
	if records[0][0] != "id" || records[0][3] != "risk_level" || records[0][4] != "request_id" || records[0][5] != "message" {
		t.Fatalf("header = %v, want id..risk_level..request_id..message", records[0])
	}
	// Newest row is first after header.
	if records[1][2] != "ERROR" || records[1][5] != "two, with comma" {
		t.Fatalf("row 1 = %v, want ERROR/comma-safe quoting in message column", records[1])
	}
}

func TestExportLogsNDJSONShape(t *testing.T) {
	database, service := testService(t)

	if err := database.AddLog("INFO", "alpha"); err != nil {
		t.Fatalf("AddLog() error = %v", err)
	}
	if err := database.AddLog("WARN", "beta"); err != nil {
		t.Fatalf("AddLog() error = %v", err)
	}

	body, filename, contentType, err := service.ExportLogs("", "", "ndjson")
	if err != nil {
		t.Fatalf("ExportLogs() error = %v", err)
	}
	if !strings.HasSuffix(filename, ".ndjson") {
		t.Fatalf("filename = %q, want .ndjson suffix", filename)
	}
	if contentType != "application/x-ndjson" {
		t.Fatalf("contentType = %q, want application/x-ndjson", contentType)
	}

	lines := strings.Split(strings.TrimRight(string(body), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(lines))
	}
	for i, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("line %d parse error = %v", i, err)
		}
		if _, ok := entry["level"]; !ok {
			t.Fatalf("line %d missing level: %s", i, line)
		}
	}
}

func TestExportLogsRejectsUnknownFormat(t *testing.T) {
	_, service := testService(t)
	if _, _, _, err := service.ExportLogs("", "", "xml"); err == nil {
		t.Fatal("ExportLogs(format=xml) error = nil, want non-nil")
	}
}

func TestExportLogsFiltersByLevel(t *testing.T) {
	database, service := testService(t)

	if err := database.AddLog("INFO", "ignored"); err != nil {
		t.Fatalf("AddLog() error = %v", err)
	}
	if err := database.AddLog("ERROR", "kept"); err != nil {
		t.Fatalf("AddLog() error = %v", err)
	}

	body, _, _, err := service.ExportLogs("ERROR", "", "csv")
	if err != nil {
		t.Fatalf("ExportLogs() error = %v", err)
	}
	records, err := csv.NewReader(strings.NewReader(string(body))).ReadAll()
	if err != nil {
		t.Fatalf("csv parse error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2 (header + 1 kept)", len(records))
	}
	if records[1][5] != "kept" {
		t.Fatalf("row = %v, want ERROR row only (message in column 5 after risk_level shifted layout)", records[1])
	}
}

func TestExportDeviceUnknownTarget(t *testing.T) {
	_, service := testService(t)
	if _, err := service.ExportDevice("aa:bb:cc:dd:ee:ff"); err == nil {
		t.Fatal("ExportDevice(unknown) error = nil, want non-nil")
	}
}
