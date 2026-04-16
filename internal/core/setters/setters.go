package setters

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func SetLocation(ctx context.Context, ip string, lat, lon float64, gen int, timeout time.Duration) bool {
	return rpcSet(ctx, ip, "Sys.SetConfig", map[string]any{"location": map[string]any{"lat": lat, "lon": lon}}, timeout)
}

func SetTimezone(ctx context.Context, ip, tz string, gen int, timeout time.Duration) bool {
	return rpcSet(ctx, ip, "Sys.SetConfig", map[string]any{"location": map[string]any{"tz": tz}}, timeout)
}

func SetMQTTServer(ctx context.Context, ip, server string, gen int, timeout time.Duration) bool {
	return rpcSet(ctx, ip, "MQTT.SetConfig", map[string]any{"server": server}, timeout)
}

func SetMQTTEnabled(ctx context.Context, ip string, enabled bool, gen int, timeout time.Duration) bool {
	return rpcSet(ctx, ip, "MQTT.SetConfig", map[string]any{"enable": enabled}, timeout)
}

func SetSNTPServer(ctx context.Context, ip, server string, gen int, timeout time.Duration) bool {
	return rpcSet(ctx, ip, "Sys.SetConfig", map[string]any{"sntp": map[string]any{"server": server}}, timeout)
}

func Reboot(ctx context.Context, ip string, gen int, timeout time.Duration) bool {
	return rpcSet(ctx, ip, "Shelly.Reboot", map[string]any{}, timeout)
}

func rpcSet(ctx context.Context, ip, method string, payload map[string]any, timeout time.Duration) bool {
	params := payload
	if strings.HasSuffix(method, ".SetConfig") {
		params = map[string]any{"config": payload}
	}
	body := map[string]any{
		"id":     1,
		"method": method,
		"params": params,
	}
	buf, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+ip+"/rpc", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 400
}
