package provisioner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// UserCAChunkSize is the byte size of each chunk sent via the chunked
// Shelly.Put* certificate RPCs. Shelly's Gen2+ implementation accepts chunks
// of up to ~1024 bytes per call; keeping chunks small also keeps the
// per-request payload well under the device's RPC body cap.
const UserCAChunkSize = 1024

// CertificateKind identifies which Shelly chunked-upload RPC to drive. All
// three RPCs (PutUserCA, PutTLSClientCert, PutTLSClientKey) share identical
// {data, append} chunk semantics; only the method name differs.
type CertificateKind int

const (
	KindUserCA CertificateKind = iota
	KindTLSClientCert
	KindTLSClientKey
)

// String returns the short stable identifier used on the wire for each kind.
// This is also the accepted value for the HTTP API's "kind" field.
func (k CertificateKind) String() string {
	switch k {
	case KindTLSClientCert:
		return "tls_client_cert"
	case KindTLSClientKey:
		return "tls_client_key"
	default:
		return "user_ca"
	}
}

// RPCMethod returns the Shelly.* JSON-RPC method that uploads this certificate kind.
func (k CertificateKind) RPCMethod() string {
	switch k {
	case KindTLSClientCert:
		return "Shelly.PutTLSClientCert"
	case KindTLSClientKey:
		return "Shelly.PutTLSClientKey"
	default:
		return "Shelly.PutUserCA"
	}
}

// ParseCertificateKind maps an API-facing string to a CertificateKind.
// An empty string defaults to KindUserCA for back-compat with the original
// /api/provision/user-ca callers that omit the field.
func ParseCertificateKind(raw string) (CertificateKind, error) {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "", "user_ca", "userca":
		return KindUserCA, nil
	case "tls_client_cert", "tlsclientcert":
		return KindTLSClientCert, nil
	case "tls_client_key", "tlsclientkey":
		return KindTLSClientKey, nil
	default:
		return KindUserCA, fmt.Errorf("unknown certificate kind %q", raw)
	}
}

// UploadUserCAResult reports a single chunk upload outcome. Named for the
// original PutUserCA-only uploader; retained unchanged so JSON clients keep
// working.
type UploadUserCAResult struct {
	Chunks    int    `json:"chunks"`
	BytesSent int    `json:"bytes_sent"`
	Detail    string `json:"detail"`
}

// UploadCertificate uploads a PEM-encoded certificate (CA, client cert, or
// client key) to the device via a series of chunked RPCs.
//
// The Shelly Gen2+ chunked-upload protocol is identical across the three
// methods:
//
//   - First chunk:     {"data": "<chunk>", "append": false}  (clears buffer, starts upload)
//   - Middle chunks:   {"data": "<chunk>", "append": true}   (appends)
//   - Final chunk:     {"data": "<chunk>", "append": true}   (appends, last one)
//   - Commit / close:  {"data": null,      "append": false}  (finalizes file)
//
// The final null+append=false call is what persists the upload to the
// corresponding device file (e.g. /user_ca.pem). Without it, the buffered
// data is discarded. TODO: verify against real hardware — see plan §1.
func UploadCertificate(ctx context.Context, ip string, kind CertificateKind, pem string, timeout time.Duration) (UploadUserCAResult, error) {
	pem = strings.TrimSpace(pem)
	if pem == "" {
		return UploadUserCAResult{}, errors.New("pem is empty")
	}
	if !strings.Contains(pem, "-----BEGIN") {
		return UploadUserCAResult{}, errors.New("pem does not contain a PEM header")
	}

	method := kind.RPCMethod()
	client := &http.Client{Timeout: timeout}
	chunks := splitUserCAChunks(pem, UserCAChunkSize)
	bytesSent := 0
	for i, chunk := range chunks {
		params := map[string]interface{}{
			"data":   chunk,
			"append": i > 0,
		}
		if err := putCertificate(ctx, client, ip, method, params); err != nil {
			return UploadUserCAResult{Chunks: i, BytesSent: bytesSent}, fmt.Errorf("chunk %d/%d: %w", i+1, len(chunks), err)
		}
		bytesSent += len(chunk)
	}
	// Commit / finalize the upload. Without this, buffered chunks are discarded.
	if err := putCertificate(ctx, client, ip, method, map[string]interface{}{"data": nil, "append": false}); err != nil {
		return UploadUserCAResult{Chunks: len(chunks), BytesSent: bytesSent}, fmt.Errorf("finalize: %w", err)
	}
	return UploadUserCAResult{
		Chunks:    len(chunks),
		BytesSent: bytesSent,
		Detail:    fmt.Sprintf("uploaded %d chunks (%d bytes) and committed via %s", len(chunks), bytesSent, method),
	}, nil
}

// UploadUserCA is a back-compat wrapper that uploads a user CA bundle.
func UploadUserCA(ctx context.Context, ip string, pem string, timeout time.Duration) (UploadUserCAResult, error) {
	return UploadCertificate(ctx, ip, KindUserCA, pem, timeout)
}

// RemoveCertificate deletes any currently stored certificate of the given
// kind on the device by sending {"data": null, "append": false} without a
// preceding upload.
func RemoveCertificate(ctx context.Context, ip string, kind CertificateKind, timeout time.Duration) error {
	client := &http.Client{Timeout: timeout}
	return putCertificate(ctx, client, ip, kind.RPCMethod(), map[string]interface{}{"data": nil, "append": false})
}

// RemoveUserCA is a back-compat wrapper around RemoveCertificate(KindUserCA).
func RemoveUserCA(ctx context.Context, ip string, timeout time.Duration) error {
	return RemoveCertificate(ctx, ip, KindUserCA, timeout)
}

// splitUserCAChunks breaks a PEM string into fixed-size byte chunks.
func splitUserCAChunks(pem string, size int) []string {
	if size <= 0 {
		size = UserCAChunkSize
	}
	if len(pem) == 0 {
		return nil
	}
	out := make([]string, 0, (len(pem)+size-1)/size)
	for start := 0; start < len(pem); start += size {
		end := start + size
		if end > len(pem) {
			end = len(pem)
		}
		out = append(out, pem[start:end])
	}
	return out
}

func putCertificate(ctx context.Context, client *http.Client, ip, method string, params map[string]interface{}) error {
	reqBody := map[string]any{
		"id":     1,
		"method": method,
		"params": params,
	}
	buf, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+ip+"/rpc", bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == 404 {
		return fmt.Errorf("%s not supported by this device", method)
	}
	if resp.StatusCode >= 400 {
		return errors.New(firstNonEmpty(rpcErrorDetail(body), resp.Status))
	}
	if len(body) == 0 {
		return nil
	}
	var rpcResp struct {
		Error any `json:"error"`
	}
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		// Non-JSON success response is tolerated.
		return nil
	}
	if rpcResp.Error == nil {
		return nil
	}
	if isMethodNotFound(rpcResp.Error) {
		return fmt.Errorf("%s not supported by this device", method)
	}
	return errors.New(rpcErrorValue(rpcResp.Error))
}
