package scanner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"shellyadmin/internal/models"
)

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
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	results := make([]models.Device, 0)

	for _, ip := range ips {
		ip := ip
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()
			if d := ProbeDevice(ctx, ip, timeout, logFn); d != nil {
				mu.Lock()
				results = append(results, *d)
				mu.Unlock()
			}
			if progressFn != nil {
				progressFn()
			}
		}()
	}
	wg.Wait()
	return results
}

func ProbeDevice(ctx context.Context, ip string, timeout time.Duration, logFn func(level, msg string)) *models.Device {
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+ip+"/shelly", nil)
	if err != nil {
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		logFn("DEBUG", fmt.Sprintf("[scan] %s unreachable: %v", ip, err))
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil
	}

	var base struct {
		Gen   int    `json:"gen"`
		Model string `json:"model"`
		Type  string `json:"type"`
		FW    string `json:"fw"`
		Ver   string `json:"ver"`
		MAC   string `json:"mac"`
		Name  string `json:"name"`
		ID    string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&base); err != nil {
		return nil
	}
	dev := &models.Device{
		IP:         ip,
		MAC:        normalizeMAC(base.MAC),
		Model:      firstString(base.Model, base.Type),
		FW:         firstString(base.Ver, base.FW),
		Gen:        base.Gen,
		Name:       base.Name,
		Serial:     base.ID,
		Online:     true,
		LastSeen:   time.Now().UTC().Format(time.RFC3339),
		FWStatus:   "unknown",
		TimeFormat: "24h",
	}
	if dev.Gen == 0 {
		dev.Gen = 1
	}
	if dev.Gen >= 2 {
		probeGen2(ctx, client, ip, dev, logFn)
	} else {
		probeGen1(ctx, client, ip, dev, logFn)
	}
	logFn("DEBUG", fmt.Sprintf("[scan] found %s %s @ %s", dev.Model, dev.MAC, ip))
	return dev
}

func ExpandCIDR(cidr string) ([]string, error) {
	ip, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}
	ones, bits := network.Mask.Size()
	if bits-ones > 16 {
		return nil, fmt.Errorf("subnet too large")
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

func probeGen2(ctx context.Context, client *http.Client, ip string, dev *models.Device, logFn func(level, msg string)) {
	config := rpcCall(ctx, client, ip, "Shelly.GetConfig", nil)
	status := rpcCall(ctx, client, ip, "Shelly.GetStatus", nil)
	kvs := rpcCall(ctx, client, ip, "KVS.Get", map[string]any{"key": "units"})

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
	}
	if wifi, ok := config["wifi"].(map[string]any); ok {
		if sta, ok := wifi["sta"].(map[string]any); ok {
			dev.WiFiSSID = firstString(sta["ssid"], "")
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
	if ws, ok := status["ws"].(map[string]any); ok {
		dev.WSConnected = anyBool(ws["connected"])
	}
	if val := firstString(kvs["value"], ""); val != "" {
		if strings.Contains(val, `"hour_format": 12`) {
			dev.TimeFormat = "12h"
		} else {
			dev.TimeFormat = "24h"
		}
	}
	logFn("DEBUG", fmt.Sprintf("[scan] gen2 probe complete for %s", ip))
}

func probeGen1(ctx context.Context, client *http.Client, ip string, dev *models.Device, logFn func(level, msg string)) {
	status, _ := getJSONMap(ctx, client, "http://"+ip+"/status")
	settings, _ := getJSONMap(ctx, client, "http://"+ip+"/settings")
	if wifi, ok := status["wifi_sta"].(map[string]any); ok {
		dev.WiFiSSID = firstString(wifi["ssid"], "")
	}
	if update, ok := status["update"].(map[string]any); ok {
		dev.FW = firstString(update["old_version"], dev.FW)
		dev.FWAvailableVer = firstString(update["new_version"], "")
		if anyBool(update["has_update"]) {
			dev.FWStatus = "update"
		}
	}
	if cloud, ok := status["cloud"].(map[string]any); ok {
		dev.CloudConnected = anyBool(cloud["connected"])
	}
	if mqtt, ok := settings["mqtt"].(map[string]any); ok {
		dev.MQTTEnabled = anyBoolPtr(mqtt["enable"])
		dev.MQTTServer = firstString(mqtt["server"], "")
	}
	dev.Name = firstString(settings["name"], dev.Name)
	dev.FW = firstString(settings["fw"], dev.FW)
	dev.TZ = firstString(settings["tz"], "")
	dev.Lat = anyFloatPtr(settings["lat"])
	dev.Lon = anyFloatPtr(settings["lng"])
	switch int(anyFloat(settings["clock_mode"])) {
	case 1:
		dev.TimeFormat = "12h"
	default:
		dev.TimeFormat = "24h"
	}
	logFn("DEBUG", fmt.Sprintf("[scan] gen1 probe complete for %s", ip))
}

func rpcCall(ctx context.Context, client *http.Client, ip, method string, body map[string]any) map[string]any {
	if body == nil {
		body = map[string]any{}
	}
	buf, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+ip+"/rpc/"+method, bytes.NewReader(buf))
	if err != nil {
		return map[string]any{}
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return map[string]any{}
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return map[string]any{}
	}
	return out
}

func getJSONMap(ctx context.Context, client *http.Client, url string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out map[string]any
	err = json.NewDecoder(resp.Body).Decode(&out)
	return out, err
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

func anyFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	default:
		return 0
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

func firstString(v any, fallback string) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return fallback
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
