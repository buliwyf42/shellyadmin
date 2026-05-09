package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"shellyadmin/internal/db"
	"shellyadmin/internal/models"
	"shellyadmin/internal/services"
)

// connectInMemory builds an MCP server bound to a fresh in-memory app
// service over a fresh DB and returns a connected client session. Used
// by tool-level unit tests to avoid the HTTP roundtrip.
func connectInMemory(t *testing.T) (*db.DB, *mcp.ClientSession) {
	t.Helper()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	svc := services.NewAppService(database, t.TempDir(), func(context.Context, string, string) {})
	server := mcp.NewServer(&mcp.Implementation{Name: "shellyadmin-test", Version: "v0"}, nil)
	register(server, svc)

	t1, t2 := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "v0"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return database, session
}

func TestListDevicesFilters(t *testing.T) {
	database, session := connectInMemory(t)
	for _, d := range []models.Device{
		{MAC: "AA:01", IP: "10.0.0.1", Name: "kitchen", App: "PlugSG3", Gen: 3, Online: true},
		{MAC: "AA:02", IP: "10.0.0.2", Name: "office", App: "Pro4PM", Gen: 2, Online: true},
		{MAC: "AA:03", IP: "10.0.0.3", Name: "bedroom-plug", App: "PlugSG3", Gen: 3, Online: false},
	} {
		if err := database.UpsertDevice(d); err != nil {
			t.Fatalf("UpsertDevice: %v", err)
		}
	}

	cases := []struct {
		name      string
		args      map[string]any
		wantCount int
	}{
		{"no filter", map[string]any{}, 3},
		{"search by name", map[string]any{"search": "plug"}, 2},
		{"filter by gen", map[string]any{"gen": 2}, 1},
		{"limit", map[string]any{"limit": 1}, 1},
		{"search no match", map[string]any{"search": "nothing-matches"}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name:      "list_devices",
				Arguments: tc.args,
			})
			if err != nil {
				t.Fatalf("CallTool: %v", err)
			}
			if res.IsError {
				t.Fatalf("tool error: %+v", res)
			}
			var out ListDevicesOutput
			if err := remarshal(res.StructuredContent, &out); err != nil {
				t.Fatalf("structured content unmarshal: %v", err)
			}
			if out.Total != tc.wantCount {
				t.Errorf("total = %d, want %d (devices=%v)", out.Total, tc.wantCount, out.Devices)
			}
		})
	}
}

func TestListCredentialsRedactsSecrets(t *testing.T) {
	database, session := connectInMemory(t)
	if err := database.SaveCredential(models.Credential{
		Name:     "site-a",
		Username: "admin",
		Password: "PlaintextSecret-do-not-leak",
		HA1:      "deadbeefcafebabe",
		Tags:     []string{"prod"},
	}); err != nil {
		t.Fatalf("SaveCredential: %v", err)
	}

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "list_credentials",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %+v", res)
	}
	blob, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	body := string(blob)
	for _, secret := range []string{"PlaintextSecret-do-not-leak", "deadbeefcafebabe", "\"password\"", "\"ha1\""} {
		if contains(body, secret) {
			t.Errorf("list_credentials response leaks %q: %s", secret, body)
		}
	}
	if !contains(body, "site-a") || !contains(body, "admin") {
		t.Errorf("list_credentials response missing non-secret fields: %s", body)
	}
}

func TestScanStatusReturnsSlimPending(t *testing.T) {
	database, session := connectInMemory(t)

	// Mimic the SPA's scan-result shape: a job whose result holds a
	// JSON-encoded ScanJobResult{Pending: []models.Device{...}}. The MCP
	// adapter must collapse each pending entry to {mac,ip,name,model,gen,app}
	// regardless of how many fields the underlying device row carries.
	pending := models.Device{
		MAC:              "AA:BB:CC:DD:EE:09",
		IP:               "10.0.0.99",
		Name:             "discovered-plug",
		Model:            "S3PL-00112EU",
		App:              "PlugSG3",
		Gen:              3,
		Online:           true,
		FW:               "1.7.5",
		SupportedMethods: []string{"Switch.Toggle", "Switch.Set", "Sys.GetConfig", "Sys.SetConfig"},
		RawConfig:        `{"a":"b"}`,
	}
	payload, err := json.Marshal(services.ScanJobResult{Pending: []models.Device{pending}})
	if err != nil {
		t.Fatalf("marshal job result: %v", err)
	}
	jobID, err := database.CreateJob("scan", "continue", "{}", 1)
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := database.CompleteJob(jobID, "done", string(payload), "", 1, 1); err != nil {
		t.Fatalf("CompleteJob: %v", err)
	}

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "scan_status",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %+v", res)
	}
	var out ScanStatusOutput
	if err := remarshal(res.StructuredContent, &out); err != nil {
		t.Fatalf("remarshal: %v", err)
	}
	if len(out.Pending) != 1 {
		t.Fatalf("pending len = %d, want 1 (out=%+v)", len(out.Pending), out)
	}
	got := out.Pending[0]
	want := ScanPendingItem{
		MAC: "AA:BB:CC:DD:EE:09", IP: "10.0.0.99", Name: "discovered-plug",
		Model: "S3PL-00112EU", Gen: 3, App: "PlugSG3",
	}
	if got != want {
		t.Errorf("pending[0] = %+v, want %+v", got, want)
	}

	// Confirm the heavy fields really are gone — the structured payload
	// must not echo supported_methods, raw_config, fw, etc.
	blob, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	body := string(blob)
	for _, banned := range []string{"supported_methods", "Switch.Toggle", "raw_config", "\"fw\"", "1.7.5"} {
		if contains(body, banned) {
			t.Errorf("scan_status pending leaked %q: %s", banned, body)
		}
	}
}

func remarshal(in any, out any) error {
	blob, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return json.Unmarshal(blob, out)
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return len(sub) == 0
}
