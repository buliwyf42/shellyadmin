package setters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

func SetLocation(ctx context.Context, ip string, lat, lon float64, gen int, timeout time.Duration) bool {
	if gen >= 2 {
		return rpcSet(ctx, ip, "Sys.SetConfig", map[string]any{"location": map[string]any{"lat": lat, "lon": lon}}, timeout)
	}
	return getOK(ctx, fmt.Sprintf("http://%s/settings?lat=%v&lng=%v", ip, lat, lon), timeout)
}

func SetTimezone(ctx context.Context, ip, tz string, gen int, timeout time.Duration) bool {
	if gen >= 2 {
		return rpcSet(ctx, ip, "Sys.SetConfig", map[string]any{"location": map[string]any{"tz": tz}}, timeout)
	}
	return getOK(ctx, "http://"+ip+"/settings?tz="+url.QueryEscape(tz), timeout)
}

func SetMQTTServer(ctx context.Context, ip, server string, gen int, timeout time.Duration) bool {
	if gen >= 2 {
		return rpcSet(ctx, ip, "MQTT.SetConfig", map[string]any{"server": server}, timeout)
	}
	return getOK(ctx, "http://"+ip+"/settings/mqtt?server="+url.QueryEscape(server), timeout)
}

func SetMQTTEnabled(ctx context.Context, ip string, enabled bool, gen int, timeout time.Duration) bool {
	if gen >= 2 {
		return rpcSet(ctx, ip, "MQTT.SetConfig", map[string]any{"enable": enabled}, timeout)
	}
	value := "0"
	if enabled {
		value = "1"
	}
	return getOK(ctx, "http://"+ip+"/settings/mqtt?enable="+value, timeout)
}

func SetTimeFormat24h(ctx context.Context, ip string, gen int, timeout time.Duration) bool {
	if gen >= 2 {
		return rpcSet(ctx, ip, "KVS.Set", map[string]any{"key": "units", "value": `{"hour_format": 24}`}, timeout)
	}
	return getOK(ctx, "http://"+ip+"/settings?clock_mode=0", timeout)
}

func rpcSet(ctx context.Context, ip, method string, payload map[string]any, timeout time.Duration) bool {
	buf, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+ip+"/rpc/"+method, bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 400
}

func getOK(ctx context.Context, rawURL string, timeout time.Duration) bool {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 400
}
