package jobs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"shellyadmin/internal/core/firmware"
	"shellyadmin/internal/core/scanner"
	"shellyadmin/internal/models"
)

// --- fake device -------------------------------------------------------

// otaShelly is a Shelly that behaves like real hardware during an OTA: it
// accepts Shelly.Update, reports the old version for a while, then flips to the
// new one (as if it had flashed and rebooted). Every RPC is recorded with the
// time it arrived, which is what the quiet-period assertion reads.
type otaShelly struct {
	srv *httptest.Server

	mu        sync.Mutex
	calls     []otaCall
	flipAt    time.Time // when GetDeviceInfo starts reporting newVer
	oldVer    string
	newVer    string
	triggered bool
}

type otaCall struct {
	method string
	at     time.Time
}

func newOTAShelly(t *testing.T, oldVer, newVer string, flashDuration time.Duration) *otaShelly {
	t.Helper()
	f := &otaShelly{oldVer: oldVer, newVer: newVer}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     int            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		f.mu.Lock()
		f.calls = append(f.calls, otaCall{method: req.Method, at: time.Now()})
		var result any
		switch req.Method {
		case "Shelly.Update":
			f.triggered = true
			f.flipAt = time.Now().Add(flashDuration)
			result = map[string]any{}
		case "Shelly.GetDeviceInfo":
			ver := f.oldVer
			if !f.flipAt.IsZero() && time.Now().After(f.flipAt) {
				ver = f.newVer
			}
			result = map[string]any{"ver": ver}
		default:
			result = map[string]any{}
		}
		f.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": req.ID, "result": result})
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *otaShelly) host() string { return strings.TrimPrefix(f.srv.URL, "http://") }

// callsBetween returns the methods received in (from, to].
func (f *otaShelly) callsBetween(from, to time.Time) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.calls {
		if c.at.After(from) && !c.at.After(to) {
			out = append(out, c.method)
		}
	}
	return out
}

// --- fake store + host -------------------------------------------------

// installStore implements Store. Only ListDevices/UpsertDevice are exercised by
// installOne; the rest satisfy the interface.
type installStore struct {
	mu      sync.Mutex
	devices []models.Device
}

func (s *installStore) ListDevices() ([]models.Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]models.Device, len(s.devices))
	copy(out, s.devices)
	return out, nil
}

func (s *installStore) UpsertDevice(d models.Device) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.devices {
		if s.devices[i].MAC == d.MAC {
			s.devices[i] = d
			return nil
		}
	}
	s.devices = append(s.devices, d)
	return nil
}

func (s *installStore) UpsertDevices([]models.Device) error { return nil }
func (s *installStore) GetSettings() (models.AppSettings, error) {
	return models.DefaultSettings(), nil
}
func (s *installStore) SaveSettings(models.AppSettings) error                     { return nil }
func (s *installStore) CreateJob(string, string, string, int) (int64, error)      { return 1, nil }
func (s *installStore) UpdateJobProgress(int64, int, int, string) error           { return nil }
func (s *installStore) IncrementJobDone(int64) error                              { return nil }
func (s *installStore) CompleteJob(int64, string, string, string, int, int) error { return nil }
func (s *installStore) InterruptJob(int64, string) error                          { return nil }
func (s *installStore) GetLatestJob(string) (models.Job, error)                   { return models.Job{}, nil }
func (s *installStore) GetJob(int64) (models.Job, error)                          { return models.Job{}, nil }
func (s *installStore) ListInterruptedRestartableJobs() ([]models.Job, error)     { return nil, nil }

// installHost implements Host. installOne only reaches for ShutdownContext,
// FirmwareOptions and Log.
type installHost struct {
	ctx context.Context
	wg  sync.WaitGroup
	mu  sync.Mutex
}

func (h *installHost) ShutdownContext() context.Context { return h.ctx }
func (h *installHost) BackgroundJobs() *sync.WaitGroup  { return &h.wg }
func (h *installHost) JobSpawnMu() *sync.Mutex          { return &h.mu }
func (h *installHost) LinkedContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(parent)
}
func (h *installHost) Log(string, string)                     {}
func (h *installHost) LogCtx(context.Context, string, string) {}
func (h *installHost) MetricInc(string)                       {}
func (h *installHost) FirmwareOptions(_ models.Device, timeout time.Duration) firmware.Options {
	return firmware.Options{Timeout: timeout, Scheme: "http"}
}
func (h *installHost) ScannerProbeOptions(models.Device, time.Duration) scanner.ProbeOptions {
	return scanner.ProbeOptions{}
}
func (h *installHost) RefreshDeviceCapabilities(context.Context, *models.Device) {}
func (h *installHost) GetDevices() ([]models.Device, error)                      { return nil, nil }
func (h *installHost) ValidateScanParams(models.AppSettings) (int, error)        { return 0, nil }
func (h *installHost) ReserveFirmwareTargets(req []string) ([]string, []string)  { return req, nil }
func (h *installHost) ReleaseFirmwareTargets([]string)                           {}

// --- the regression --------------------------------------------------------

// installOne must send the device nothing between the trigger and the end of the
// quiet period. The reported version cannot change until the device has flashed
// and rebooted, so polling during the download learns nothing while costing the
// device heap it is short of mid-OTA.
//
// This pins the behaviour, not a theory of failure: v0.5.6 claimed the old
// poll-immediately loop was what broke OTAs, and v0.5.7 retracted that (the
// failures reproduce identically unpolled). The contract is still worth holding
// — but if you are here because an OTA failed, this is not your culprit.
func TestInstallOneSendsNothingDuringQuietPeriod(t *testing.T) {
	const (
		quietPeriod = 400 * time.Millisecond
		// Flash finishes inside the quiet window, so any GetDeviceInfo seen
		// during it is the job's doing and not a race with the version flip.
		flashDuration = 150 * time.Millisecond
	)

	dev := newOTAShelly(t, "2.0.0-beta3", "2.0.0", flashDuration)
	device := models.Device{
		MAC: "AA:BB:CC:DD:EE:FF", IP: dev.host(), Gen: 3,
		FW: "2.0.0-beta3", FWAvailableStable: "2.0.0",
	}
	store := &installStore{devices: []models.Device{device}}
	svc := New(store, &installHost{ctx: context.Background()})

	results := []FirmwareInstallResult{{MAC: device.MAC, Status: "pending"}}
	var resMu sync.Mutex
	setResult := func(i int, mut func(*FirmwareInstallResult)) {
		resMu.Lock()
		defer resMu.Unlock()
		mut(&results[i])
	}

	start := time.Now()
	svc.installOne(1, 0, device.MAC, "stable", device,
		5*time.Second, quietPeriod, 20*time.Millisecond,
		setResult, func() {})

	// The assertion that matters: the trigger, and nothing else, until the
	// quiet period is over.
	during := dev.callsBetween(start, start.Add(quietPeriod))
	if len(during) != 1 || during[0] != "Shelly.Update" {
		t.Errorf("device was contacted during the quiet period: got %v, want exactly [Shelly.Update].\n"+
			"Polling an in-flight OTA stalls the download — nothing may be sent until it has flashed.", during)
	}

	resMu.Lock()
	got := results[0]
	resMu.Unlock()
	if got.Status != "current" {
		t.Errorf("Status = %q (%s), want current", got.Status, got.Detail)
	}
	if got.ToVer != "2.0.0" {
		t.Errorf("ToVer = %q, want 2.0.0", got.ToVer)
	}
}

// The expected target is only a prediction from the last firmware_check. When
// the device installs something else, the update still happened — gating on an
// exact match would poll to the timeout and report success as "unknown".
func TestInstallOneAcceptsAVersionOtherThanPredicted(t *testing.T) {
	dev := newOTAShelly(t, "2.0.0-beta3", "2.0.1", 10*time.Millisecond)
	device := models.Device{
		MAC: "AA:BB:CC:DD:EE:01", IP: dev.host(), Gen: 3,
		FW: "2.0.0-beta3", FWAvailableStable: "2.0.0", // predicted 2.0.0, device ships 2.0.1
	}
	store := &installStore{devices: []models.Device{device}}
	svc := New(store, &installHost{ctx: context.Background()})

	results := []FirmwareInstallResult{{MAC: device.MAC, Status: "pending"}}
	var resMu sync.Mutex
	setResult := func(i int, mut func(*FirmwareInstallResult)) {
		resMu.Lock()
		defer resMu.Unlock()
		mut(&results[i])
	}

	svc.installOne(1, 0, device.MAC, "stable", device,
		3*time.Second, 20*time.Millisecond, 20*time.Millisecond,
		setResult, func() {})

	resMu.Lock()
	got := results[0]
	resMu.Unlock()
	if got.Status != "current" {
		t.Errorf("Status = %q (%s), want current — any move off the original version is a landed update",
			got.Status, got.Detail)
	}
	if got.ToVer != "2.0.1" {
		t.Errorf("ToVer = %q, want the version actually installed (2.0.1)", got.ToVer)
	}
}
