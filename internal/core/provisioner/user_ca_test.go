package provisioner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// allKinds exposes the three certificate-upload kinds for table-driven
// coverage. The semantics are identical across all three RPCs so every
// protocol assertion runs against every kind.
var allKinds = []struct {
	kind       CertificateKind
	name       string
	wantMethod string
}{
	{KindUserCA, "user_ca", "Shelly.PutUserCA"},
	{KindTLSClientCert, "tls_client_cert", "Shelly.PutTLSClientCert"},
	{KindTLSClientKey, "tls_client_key", "Shelly.PutTLSClientKey"},
}

func TestCertificateKind_StringAndRPCMethod(t *testing.T) {
	for _, tc := range allKinds {
		if got := tc.kind.String(); got != tc.name {
			t.Errorf("kind %v String() = %q, want %q", tc.kind, got, tc.name)
		}
		if got := tc.kind.RPCMethod(); got != tc.wantMethod {
			t.Errorf("kind %v RPCMethod() = %q, want %q", tc.kind, got, tc.wantMethod)
		}
	}
}

func TestParseCertificateKind(t *testing.T) {
	cases := []struct {
		in   string
		want CertificateKind
		err  bool
	}{
		{"", KindUserCA, false},
		{"user_ca", KindUserCA, false},
		{"USER_CA", KindUserCA, false},
		{"userca", KindUserCA, false},
		{"tls_client_cert", KindTLSClientCert, false},
		{"  tls_client_key  ", KindTLSClientKey, false},
		{"bogus", KindUserCA, true},
	}
	for _, tc := range cases {
		got, err := ParseCertificateKind(tc.in)
		if tc.err {
			if err == nil {
				t.Errorf("ParseCertificateKind(%q) err = nil, want error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseCertificateKind(%q) unexpected error: %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("ParseCertificateKind(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestUploadCertificate_ChunkedSequenceEveryKind(t *testing.T) {
	for _, tc := range allKinds {
		t.Run(tc.name, func(t *testing.T) {
			// Build a PEM large enough to span multiple chunks.
			pem := "-----BEGIN CERTIFICATE-----\n" +
				strings.Repeat("A", UserCAChunkSize*2+37) +
				"\n-----END CERTIFICATE-----\n"

			var calls []map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/rpc" {
					http.NotFound(w, r)
					return
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatalf("decode: %v", err)
				}
				calls = append(calls, body)
				_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"len": 0}})
			}))
			defer server.Close()

			ip := server.Listener.Addr().String()
			result, err := UploadCertificate(context.Background(), ip, tc.kind, pem, 2*time.Second)
			if err != nil {
				t.Fatalf("UploadCertificate error: %v", err)
			}
			if result.Chunks != 3 {
				t.Fatalf("expected 3 chunks, got %d", result.Chunks)
			}
			if result.BytesSent != len(strings.TrimSpace(pem)) {
				t.Fatalf("bytes_sent = %d, want %d", result.BytesSent, len(strings.TrimSpace(pem)))
			}
			// 3 upload calls + 1 finalize = 4 calls.
			if len(calls) != 4 {
				t.Fatalf("expected 4 RPC calls, got %d", len(calls))
			}
			for i, call := range calls {
				if got := call["method"]; got != tc.wantMethod {
					t.Fatalf("call %d method = %v, want %v", i, got, tc.wantMethod)
				}
				params, ok := call["params"].(map[string]any)
				if !ok {
					t.Fatalf("call %d: params not an object", i)
				}
				appendFlag, _ := params["append"].(bool)
				switch i {
				case 0: // first chunk — new upload
					if appendFlag {
						t.Fatalf("call 0: append = true, want false")
					}
					if _, ok := params["data"].(string); !ok {
						t.Fatalf("call 0: data not a string")
					}
				case 1, 2: // subsequent chunks
					if !appendFlag {
						t.Fatalf("call %d: append = false, want true", i)
					}
				case 3: // finalize
					if appendFlag {
						t.Fatalf("finalize call: append = true, want false")
					}
					if params["data"] != nil {
						t.Fatalf("finalize call: data = %v, want nil", params["data"])
					}
				}
			}
		})
	}
}

func TestUploadUserCA_BackCompatWrapperUsesPutUserCA(t *testing.T) {
	var method string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if m, ok := body["method"].(string); ok && method == "" {
			method = m
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{}})
	}))
	defer server.Close()

	pem := "-----BEGIN CERTIFICATE-----\nabc\n-----END CERTIFICATE-----\n"
	if _, err := UploadUserCA(context.Background(), server.Listener.Addr().String(), pem, time.Second); err != nil {
		t.Fatalf("UploadUserCA error: %v", err)
	}
	if method != "Shelly.PutUserCA" {
		t.Fatalf("UploadUserCA sent method %q, want Shelly.PutUserCA", method)
	}
}

func TestUploadCertificate_RejectsEmptyPEM(t *testing.T) {
	_, err := UploadCertificate(context.Background(), "127.0.0.1:0", KindUserCA, "   \n\n  ", time.Second)
	if err == nil {
		t.Fatal("expected error for empty PEM")
	}
}

func TestUploadCertificate_RejectsNonPEM(t *testing.T) {
	_, err := UploadCertificate(context.Background(), "127.0.0.1:0", KindTLSClientCert, "not a certificate", time.Second)
	if err == nil {
		t.Fatal("expected error for non-PEM input")
	}
}

func TestUploadCertificate_SurfacesMethodNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"code": 404, "message": "Not Found"},
		})
	}))
	defer server.Close()

	ip := server.Listener.Addr().String()
	pem := "-----BEGIN CERTIFICATE-----\nabc\n-----END CERTIFICATE-----\n"
	_, err := UploadCertificate(context.Background(), ip, KindTLSClientKey, pem, time.Second)
	if err == nil {
		t.Fatal("expected error for unsupported device")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "Shelly.PutTLSClientKey") {
		t.Fatalf("error should mention the specific method, got: %v", err)
	}
}

func TestSplitUserCAChunks_ExactBoundary(t *testing.T) {
	pem := strings.Repeat("x", UserCAChunkSize*2)
	chunks := splitUserCAChunks(pem, UserCAChunkSize)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if len(chunks[0]) != UserCAChunkSize || len(chunks[1]) != UserCAChunkSize {
		t.Fatalf("unexpected chunk sizes: %d,%d", len(chunks[0]), len(chunks[1]))
	}
}

func TestSplitUserCAChunks_Remainder(t *testing.T) {
	pem := strings.Repeat("x", UserCAChunkSize+5)
	chunks := splitUserCAChunks(pem, UserCAChunkSize)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if len(chunks[1]) != 5 {
		t.Fatalf("expected tail chunk of 5, got %d", len(chunks[1]))
	}
}
