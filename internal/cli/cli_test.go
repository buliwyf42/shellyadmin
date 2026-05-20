package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandles(t *testing.T) {
	for _, cmd := range []string{"devices", "device", "logs", "firmware", "templates"} {
		if !Handles(cmd) {
			t.Errorf("Handles(%q) = false, want true", cmd)
		}
	}
	for _, cmd := range []string{"", "server", "hash-password", "mcp", "unlock", "reset-auth"} {
		if Handles(cmd) {
			t.Errorf("Handles(%q) = true, want false", cmd)
		}
	}
}

func TestScalarString(t *testing.T) {
	cases := []struct {
		in    any
		want  string
		wantK bool
	}{
		{"hello", "hello", true},
		{true, "true", true},
		{float64(42), "42", true},   // integral floats render without a decimal
		{float64(1.5), "1.5", true}, // genuine fractions keep precision
		{nil, "", false},
		{map[string]any{"a": 1}, "", false}, // nested objects are skipped
		{[]any{1, 2}, "", false},            // arrays are skipped
	}
	for _, tc := range cases {
		got, ok := scalarString(tc.in)
		if got != tc.want || ok != tc.wantK {
			t.Errorf("scalarString(%v) = (%q,%t), want (%q,%t)", tc.in, got, ok, tc.want, tc.wantK)
		}
	}
}

func TestDashAndYesNo(t *testing.T) {
	if dash("") != "—" || dash("x") != "x" {
		t.Error("dash: empty should render an em-dash, non-empty unchanged")
	}
	if yesno(true) != "yes" || yesno(false) != "no" {
		t.Error("yesno mapping wrong")
	}
}

func TestUpdateFlag(t *testing.T) {
	cases := []struct {
		stable, beta bool
		want         string
	}{
		{true, true, "stable+beta"},
		{true, false, "stable"},
		{false, true, "beta"},
		{false, false, "—"},
	}
	for _, tc := range cases {
		if got := updateFlag(tc.stable, tc.beta); got != tc.want {
			t.Errorf("updateFlag(%t,%t) = %q, want %q", tc.stable, tc.beta, got, tc.want)
		}
	}
}

func TestFirmwareDecodesStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"running":false,"done":2,"total":2,"results":[{"mac":"AABB","current_ver":"1.0","stable_ver":"1.1","stable_update":true,"status":"ok"}]}`))
	}))
	defer srv.Close()

	c := newClient()
	c.url = srv.URL
	c.token = "pat_test"
	var st fwStatus
	if msg := c.get("/api/firmware/status", &st); msg != "" {
		t.Fatalf("get returned error: %s", msg)
	}
	if st.Done != 2 || len(st.Results) != 1 || !st.Results[0].StableUpdate {
		t.Fatalf("decoded unexpected firmware status: %+v", st)
	}
}

func TestGetRequiresToken(t *testing.T) {
	c := newClient() // no token
	if msg := c.get("/api/devices", &[]deviceRow{}); msg == "" {
		t.Error("get with no token should return an error message")
	}
}

func TestGetSendsBearerAndDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer pat_test" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer pat_test")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"device_num":1,"name":"kitchen","online":true,"gen":2}]`))
	}))
	defer srv.Close()

	c := newClient()
	c.url = srv.URL
	c.token = "pat_test"
	var devices []deviceRow
	if msg := c.get("/api/devices", &devices); msg != "" {
		t.Fatalf("get returned error: %s", msg)
	}
	if len(devices) != 1 || devices[0].Name != "kitchen" || !devices[0].Online {
		t.Fatalf("decoded unexpected payload: %+v", devices)
	}
}

func TestGetAuthFailureMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := newClient()
	c.url = srv.URL
	c.token = "pat_wrongscope"
	msg := c.get("/api/devices", &[]deviceRow{})
	if msg == "" {
		t.Fatal("expected an auth-failure message on HTTP 403")
	}
}
