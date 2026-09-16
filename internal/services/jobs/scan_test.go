package jobs

import (
	"context"
	"testing"

	"shellyadmin/internal/models"
)

// scanStatusStore reuses installStore's no-op Store surface and serves one
// fixed scan job, which is all ScanStatus reads.
type scanStatusStore struct {
	installStore
	job models.Job
}

func (s *scanStatusStore) GetLatestJob(string) (models.Job, error) { return s.job, nil }

// Scans have two triggers — the MCP tool and the SPA — and scan_status used to
// describe a scan without saying WHICH one. A client polling its own sweep
// could be served a foreign one with nothing in the payload to tell them
// apart: `running: false` is true of a finished own scan and of a foreign scan
// whose row has not flipped yet. That ambiguity produced a wrong conclusion in
// a real measurement session before the identity fields existed.
func TestScanStatusIdentifiesTheJobItDescribes(t *testing.T) {
	store := &scanStatusStore{job: models.Job{
		ID:        4711,
		Type:      "scan",
		Status:    "done",
		Done:      254,
		Total:     254,
		CreatedAt: "2026-09-15T20:08:26Z",
		Result:    `{"pending":[]}`,
	}}
	svc := New(store, &installHost{ctx: context.Background()})

	status, err := svc.ScanStatus()
	if err != nil {
		t.Fatalf("ScanStatus: %v", err)
	}
	if status.JobID != 4711 {
		t.Errorf("JobID = %d, want 4711 — without it a caller cannot tell its own sweep from the SPA's", status.JobID)
	}
	if status.StartedAt != "2026-09-15T20:08:26Z" {
		t.Errorf("StartedAt = %q, want the job's CreatedAt", status.StartedAt)
	}
	if status.Running {
		t.Errorf("Running = true for a done job")
	}
}
