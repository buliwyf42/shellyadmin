package scanner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	"shellyadmin/internal/core/clock"
	"shellyadmin/internal/core/shellyclient"
	"shellyadmin/internal/models"
)

// maxScanHosts caps the number of addresses that may be expanded from a single
// CIDR. A /22 (1024 addresses) is comfortably above any realistic home or small
// office LAN and bounds the blast radius if an operator mistypes a subnet.
const maxScanHosts = 1024

// ProbeOptions configures a single probe. Empty values are sensible defaults.
type ProbeOptions struct {
	Timeout       time.Duration
	Scheme        string // "http" (default) or "https"
	Username      string
	Password      string
	HA1           string
	AllowInsecure bool   // skip TLS cert verification
	KnownMAC      string // when set, recoverable failures (auth-required, lockout, TLS-cert-invalid) produce a partial Device record using this MAC so the refresh path can persist the state. When empty (scan path), recoverable failures yield nil — we have no positive Shelly identification, and persisting a partial record would surface non-Shelly LAN gear (UniFi UDM, etc.) in the device list.
	// Clock is an optional injection seam for tests. nil means real wall-clock
	// time; production callers leave this unset.
	Clock clock.Clock
}

func (o ProbeOptions) clock() clock.Clock {
	if o.Clock == nil {
		return clock.Real()
	}
	return o.Clock
}

func (o ProbeOptions) toClientOptions() shellyclient.Options {
	out := shellyclient.Options{
		Timeout:  o.Timeout,
		Scheme:   o.Scheme,
		Username: o.Username,
		Password: o.Password,
		HA1:      o.HA1,
	}
	if o.AllowInsecure {
		out.TLSPolicy = shellyclient.TLSSkip
	}
	return out
}

func ScanSubnets(ctx context.Context, subnets []string, concurrency int, timeout time.Duration, logFn func(level, msg string), progressFn func()) []models.Device {
	if concurrency <= 0 {
		concurrency = 32
	}
	var ips []string
	for _, subnet := range subnets {
		expanded, err := ExpandCIDR(subnet)
		if err != nil {
			logFn("WARN", fmt.Sprintf("[scan] invalid subnet %s: %v", subnet, err))
			continue
		}
		ips = append(ips, expanded...)
	}
	if concurrency > len(ips) {
		concurrency = len(ips)
	}
	if concurrency < 1 {
		concurrency = 1
	}
	work := make(chan string)
	var wg sync.WaitGroup
	var mu sync.Mutex
	results := make([]models.Device, 0)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ip := range work {
				select {
				case <-ctx.Done():
					if progressFn != nil {
						progressFn()
					}
					continue
				default:
				}
				if d := ProbeDevice(ctx, ip, timeout, logFn); d != nil {
					mu.Lock()
					results = append(results, *d)
					mu.Unlock()
				}
				if progressFn != nil {
					progressFn()
				}
			}
		}()
	}
	for _, ip := range ips {
		work <- ip
	}
	close(work)
	wg.Wait()
	return results
}

// ProbeDevice retains the original unauthenticated, http-only signature so the
// scan path (which doesn't yet know which credential matches the device) keeps
// its behaviour. Refresh paths that have a credential should use
// ProbeDeviceWithOptions instead.
func ProbeDevice(ctx context.Context, ip string, timeout time.Duration, logFn func(level, msg string)) *models.Device {
	return ProbeDeviceWithOptions(ctx, ip, ProbeOptions{Timeout: timeout}, logFn)
}

// ProbeDeviceWithOptions probes via shellyclient.Client so digest auth, TLS,
// and 429 lockout signalling are handled in one place. The caller is
// responsible for persisting any scheme upgrade or auth-state change back to
// the device record using the returned device's Scheme/AuthRequired/AuthLockedUntil
// fields.
func ProbeDeviceWithOptions(ctx context.Context, ip string, opts ProbeOptions, logFn func(level, msg string)) *models.Device {
	client := shellyclient.New(opts.toClientOptions())
	return ProbeDeviceOnClient(ctx, client, ip, opts.KnownMAC, opts.clock(), logFn)
}

// ProbeDeviceOnClient is the test seam: it accepts a pre-built shellyclient
// and an explicit Clock, so unit tests can drive a httptest fake-Shelly
// server with a deterministic time source. Production callers go through
// ProbeDeviceWithOptions, which builds the client + real clock for them.
func ProbeDeviceOnClient(ctx context.Context, client *shellyclient.Client, ip, knownMAC string, clk clock.Clock, logFn func(level, msg string)) *models.Device {
	if clk == nil {
		clk = clock.Real()
	}
	base, err := client.Probe(ctx, ip)
	if err != nil {
		logFn("DEBUG", fmt.Sprintf("[scan] %s probe failed: %v", ip, err))
		return reportProbeFailure(ip, err, knownMAC, clk)
	}
	// Reject anything that doesn't look like a Shelly /shelly response. Some
	// non-Shelly endpoints (UniFi UDM, Protect cameras, generic web servers)
	// answer 200 with a JSON body that doesn't match the Shelly shape — a real
	// Shelly always reports either a non-empty MAC or a non-zero `gen` field.
	mac := normalizeMAC(stringField(base, "mac"))
	gen := intField(base, "gen")
	if mac == "" && gen == 0 {
		logFn("DEBUG", fmt.Sprintf("[scan] %s answered /shelly but lacks Shelly markers (no mac, no gen) — skipping", ip))
		return nil
	}
	dev := &models.Device{
		IP:       ip,
		MAC:      mac,
		Model:    firstString(base["model"], base["type"]),
		App:      stringField(base, "app"),
		FWID:     stringField(base, "fw_id"),
		FW:       firstString(base["ver"], base["fw"]),
		Gen:      gen,
		Name:     stringField(base, "name"),
		Serial:   stringField(base, "id"),
		Online:   true,
		LastSeen: clk.Now().UTC().Format(time.RFC3339),
		Scheme:   client.Scheme(),
	}
	if dev.Gen == 0 {
		dev.Gen = 2
	}
	probeGen2(ctx, client, ip, dev, logFn)
	logFn("DEBUG", fmt.Sprintf("[scan] found %s %s @ %s", dev.Model, dev.MAC, ip))
	return dev
}

// reportProbeFailure converts a shellyclient error into a partial device record
// for the refresh-of-known-Shelly path. The caller passes the device's existing
// MAC so the partial record can carry it forward — this lets the UI surface
// auth-required / locked-out / TLS-cert-invalid state on the right row.
//
// When knownMAC is empty (scan path: probing an unknown IP), we have no
// positive Shelly identification — the response was an error, not a Shelly
// /shelly payload — so we return nil. Persisting a partial record without a
// MAC would surface non-Shelly LAN gear (UniFi UDM with self-signed HTTPS,
// nginx servers with HTTP Basic auth, etc.) in the device list.
func reportProbeFailure(ip string, err error, knownMAC string, clk clock.Clock) *models.Device {
	if knownMAC == "" {
		return nil
	}
	if clk == nil {
		clk = clock.Real()
	}
	switch {
	case errors.Is(err, shellyclient.ErrAuthRequired):
		return &models.Device{
			IP:           ip,
			MAC:          knownMAC,
			Online:       true,
			AuthRequired: true,
			AuthError:    "authentication required",
		}
	case errors.Is(err, shellyclient.ErrAuthLockout):
		return &models.Device{
			IP:              ip,
			MAC:             knownMAC,
			Online:          true,
			AuthRequired:    true,
			AuthError:       "device temporarily locked (brute-force protection)",
			AuthLockedUntil: clk.Now().UTC().Add(60 * time.Second).Format(time.RFC3339),
		}
	case errors.Is(err, shellyclient.ErrTLSCertInvalid):
		return &models.Device{
			IP:           ip,
			MAC:          knownMAC,
			Online:       true,
			AuthError:    "TLS certificate validation failed",
			TLSCertValid: boolPtr(false),
		}
	}
	return nil
}

func ExpandCIDR(cidr string) ([]string, error) {
	ip, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}
	if prefix, err := netip.ParsePrefix(cidr); err == nil {
		if !IsAllowedScanNetwork(prefix.Masked().Addr()) {
			return nil, fmt.Errorf("subnet %s is outside the allowed scan range (RFC1918 / link-local only)", cidr)
		}
	}
	ones, bits := network.Mask.Size()
	hostBits := bits - ones
	if hostBits > 16 || (1<<hostBits) > maxScanHosts {
		return nil, fmt.Errorf("subnet %s expands to more than %d hosts; use a /%d or smaller", cidr, maxScanHosts, bits-bitsForMaxHosts(maxScanHosts))
	}
	var out []string
	for cursor := ip.Mask(network.Mask); network.Contains(cursor); incIP(cursor) {
		if isNetworkOrBroadcast(cursor, network) {
			continue
		}
		out = append(out, cursor.String())
	}
	return out, nil
}

// IsAllowedScanNetwork mirrors isProvisionTargetAllowed in services: accept
// only RFC1918 / ULA and link-local addresses; reject loopback, multicast,
// unspecified, and any public address. Applied to the masked network address
// of a CIDR so the whole subnet is inside the allowed ranges.
func IsAllowedScanNetwork(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	if addr.IsLoopback() || addr.IsMulticast() || addr.IsUnspecified() {
		return false
	}
	return addr.IsPrivate() || addr.IsLinkLocalUnicast()
}

// bitsForMaxHosts returns the host-bit width needed to fit max addresses,
// used to phrase the rejection message in CIDR terms ("use a /22 or smaller").
func bitsForMaxHosts(max int) int {
	bits := 0
	for (1 << bits) < max {
		bits++
	}
	return bits
}

func probeGen2(ctx context.Context, client *shellyclient.Client, ip string, dev *models.Device, logFn func(level, msg string)) {
	config, _ := client.RPC(ctx, ip, "Shelly.GetConfig", nil)
	status, _ := client.RPC(ctx, ip, "Shelly.GetStatus", nil)
	dev.RawConfig = marshalMap(config)
	dev.RawStatus = marshalMap(status)

	if sys, ok := config["sys"].(map[string]any); ok {
		if device, ok := sys["device"].(map[string]any); ok {
			dev.Name = firstString(device["name"], dev.Name)
			dev.EcoMode = anyBoolPtr(device["eco_mode"])
			dev.Discoverable = anyBoolPtr(device["discoverable"])
		}
		if location, ok := sys["location"].(map[string]any); ok {
			dev.TZ = firstString(location["tz"], dev.TZ)
			dev.Lat = anyFloatPtr(location["lat"])
			dev.Lon = anyFloatPtr(location["lon"])
		}
		if sntp, ok := sys["sntp"].(map[string]any); ok {
			dev.SNTPServer = firstString(sntp["server"], "")
		}
		// FW 2.0.0-beta1: enhanced_security flag flips client-side TLS expectations.
		if v, ok := sys["enhanced_security"]; ok {
			dev.EnhancedSecurity = anyBoolPtr(v)
		}
	}
	if mqtt, ok := config["mqtt"].(map[string]any); ok {
		dev.MQTTEnabled = anyBoolPtr(mqtt["enable"])
		dev.MQTTServer = firstString(mqtt["server"], "")
		dev.MQTTClientID = firstString(mqtt["client_id"], "")
		dev.MQTTTopicPrefix = firstString(mqtt["topic_prefix"], "")
		dev.MQTTFlagsNA = flagsCSV(mqtt, "rpc_ntf", "status_ntf", "enable_rpc", "enable_control")
	}
	if ws, ok := config["ws"].(map[string]any); ok {
		dev.WSEnabled = anyBoolPtr(ws["enable"])
		dev.WSServer = firstString(ws["server"], "")
	}
	if ble, ok := config["ble"].(map[string]any); ok {
		if gw, ok := ble["gateway"].(map[string]any); ok {
			dev.BLEGWEnabled = anyBoolPtr(gw["enable"])
		}
		if rpc, ok := ble["rpc"].(map[string]any); ok {
			dev.BLERPCEnabled = anyBoolPtr(rpc["enable"])
		}
		if obs, ok := ble["observer"].(map[string]any); ok {
			dev.BLEObserverEnabled = anyBoolPtr(obs["enable"])
		}
	}
	if wifi, ok := config["wifi"].(map[string]any); ok {
		if sta, ok := wifi["sta"].(map[string]any); ok {
			dev.WiFiSSID = firstString(sta["ssid"], "")
		}
		// FW 2.0.0-beta1 added wifi.ap.hostname and a per-device hostname slot;
		// take whichever is populated (sta-side wins to match user expectation).
		if sta, ok := wifi["sta"].(map[string]any); ok {
			if host := strings.TrimSpace(firstString(sta["hostname"], "")); host != "" {
				dev.WiFiHostname = host
			}
		}
		if dev.WiFiHostname == "" {
			if ap, ok := wifi["ap"].(map[string]any); ok {
				if host := strings.TrimSpace(firstString(ap["hostname"], "")); host != "" {
					dev.WiFiHostname = host
				}
			}
		}
	}
	if cloud, ok := config["cloud"].(map[string]any); ok {
		dev.CloudEnabled = anyBoolPtr(cloud["enable"])
	}
	if matter, ok := config["matter"].(map[string]any); ok {
		dev.MatterEnabled = anyBoolPtr(matter["enable"])
	}
	if cloud, ok := status["cloud"].(map[string]any); ok {
		dev.CloudConnected = anyBool(cloud["connected"])
	}
	if mqtt, ok := status["mqtt"].(map[string]any); ok {
		dev.MQTTConnected = anyBool(mqtt["connected"])
	}
	if ws, ok := status["ws"].(map[string]any); ok {
		dev.WSConnected = anyBool(ws["connected"])
	}
	// FW 2.0.0-beta1 surfaces wifi channel in wifi.sta_status (renamed from wifi.sta in some firmwares).
	if wifi, ok := status["wifi"].(map[string]any); ok {
		if sta, ok := wifi["sta_status"].(map[string]any); ok {
			dev.WiFiChannel = intField(sta, "channel")
		} else if sta, ok := wifi["sta"].(map[string]any); ok {
			dev.WiFiChannel = intField(sta, "channel")
		}
	}
	extractPowerReadings(status, dev)
	logFn("DEBUG", fmt.Sprintf("[scan] gen2 probe complete for %s", ip))
}

// extractPowerReadings sums power telemetry across every component that
// reports it: switch:N, em:N (3-phase), em1:N (single-phase), pm1:N. Voltage
// is reported as the maximum non-zero reading across all components and
// phases — summing volts is meaningless, and "most recent" is non-deterministic
// because Go map iteration order is randomized. Max is stable and matches
// what a user would intuitively expect to see in a "live readings" badge.
func extractPowerReadings(status map[string]any, dev *models.Device) {
	if len(status) == 0 {
		return
	}
	var totalW, totalA float64
	var maxV float64
	var sawAny bool
	consider := func(v float64) {
		if v > maxV {
			maxV = v
		}
	}
	for key, val := range status {
		if !isPowerComponent(key) {
			continue
		}
		obj, ok := val.(map[string]any)
		if !ok {
			continue
		}
		// switch:N / pm1:N / em1:N → apower (W), voltage (V), current (A)
		if w, ok := numberField(obj, "apower"); ok {
			totalW += w
			sawAny = true
		}
		if a, ok := numberField(obj, "current"); ok {
			totalA += a
			sawAny = true
		}
		if v, ok := numberField(obj, "voltage"); ok && v > 0 {
			consider(v)
			sawAny = true
		}
		// em:N (3-phase) → total_act_power and aggregate current
		if w, ok := numberField(obj, "total_act_power"); ok {
			totalW += w
			sawAny = true
		}
		if a, ok := numberField(obj, "total_current"); ok {
			totalA += a
			sawAny = true
		}
		// em:N also exposes a_voltage / b_voltage / c_voltage.
		for _, phaseKey := range []string{"a_voltage", "b_voltage", "c_voltage"} {
			if v, ok := numberField(obj, phaseKey); ok && v > 0 {
				consider(v)
				sawAny = true
			}
		}
	}
	if !sawAny {
		return
	}
	dev.PowerW = floatPtr(totalW)
	dev.CurrentA = floatPtr(totalA)
	if maxV > 0 {
		dev.VoltageV = floatPtr(maxV)
	}
}

func isPowerComponent(key string) bool {
	switch {
	case strings.HasPrefix(key, "switch:"):
		return true
	case strings.HasPrefix(key, "em:"):
		return true
	case strings.HasPrefix(key, "em1:"):
		return true
	case strings.HasPrefix(key, "pm1:"):
		return true
	}
	return false
}

func numberField(m map[string]any, key string) (float64, bool) {
	switch v := m[key].(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	}
	return 0, false
}

func floatPtr(v float64) *float64 { return &v }

func marshalMap(data map[string]any) string {
	if len(data) == 0 {
		return ""
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func normalizeMAC(raw string) string {
	raw = strings.ReplaceAll(strings.ReplaceAll(strings.ToUpper(raw), ":", ""), "-", "")
	if len(raw) != 12 {
		return raw
	}
	parts := make([]string, 0, 6)
	for i := 0; i < 12; i += 2 {
		parts = append(parts, raw[i:i+2])
	}
	return strings.Join(parts, ":")
}

func incIP(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] > 0 {
			break
		}
	}
}

func isNetworkOrBroadcast(ip net.IP, network *net.IPNet) bool {
	if ip.Equal(network.IP.Mask(network.Mask)) {
		return true
	}
	broadcast := make(net.IP, len(network.IP))
	copy(broadcast, network.IP)
	for i := range broadcast {
		broadcast[i] |= ^network.Mask[i]
	}
	return ip.Equal(broadcast)
}

func anyBoolPtr(v any) *bool {
	b := anyBool(v)
	switch v.(type) {
	case bool, float64:
		return &b
	default:
		return nil
	}
}

func anyBool(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case float64:
		return x != 0
	default:
		return false
	}
}

func anyFloatPtr(v any) *float64 {
	switch x := v.(type) {
	case float64:
		return &x
	case int:
		y := float64(x)
		return &y
	default:
		return nil
	}
}

func firstString(v any, fallbackRaw any) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	switch fb := fallbackRaw.(type) {
	case string:
		return fb
	case nil:
		return ""
	}
	if s, ok := fallbackRaw.(string); ok {
		return s
	}
	return ""
}

func stringField(m map[string]any, key string) string {
	if s, ok := m[key].(string); ok {
		return s
	}
	return ""
}

func intField(m map[string]any, key string) int {
	switch v := m[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}

func flagsCSV(m map[string]any, names ...string) string {
	flags := make([]string, 0, len(names))
	for _, name := range names {
		if anyBool(m[name]) {
			flags = append(flags, name)
		}
	}
	return strings.Join(flags, ",")
}

func boolPtr(b bool) *bool { return &b }
