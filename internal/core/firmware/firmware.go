package firmware

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"shellyadmin/internal/models"
)

type Result struct {
	IP              string `json:"ip"`
	MAC             string `json:"mac"`
	CurrentVer      string `json:"current_ver"`
	AvailableVer    string `json:"available_ver"`
	UpdateAvailable bool   `json:"update_available"`
	Status          string `json:"status"`
	Note            string `json:"note"`
	Stage           string `json:"stage"`
}

type UpdateResult struct {
	IP     string `json:"ip"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

func CheckOne(ctx context.Context, d models.Device, stage string, timeout time.Duration) Result {
	res := Result{IP: d.IP, MAC: d.MAC, CurrentVer: d.FW, Status: "na", Stage: stage}
	client := &http.Client{Timeout: timeout}
	if d.Gen >= 2 {
		body, _ := json.Marshal(map[string]any{})
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+d.IP+"/rpc/Shelly.CheckForUpdate", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			res.Status = "error"
			res.Detail(err)
			return res
		}
		defer resp.Body.Close()
		var payload map[string]map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			res.Status = "error"
			res.Detail(err)
			return res
		}
		selected := pickStage(payload, stage)
		res.AvailableVer = stringValue(selected["version"])
		res.Note = stageNote(payload, stage)
		if res.AvailableVer == "" {
			res.Status = "na"
			return res
		}
		res.UpdateAvailable = res.AvailableVer != d.FW
		if res.UpdateAvailable {
			res.Status = "update"
		} else {
			res.Status = "current"
		}
		return res
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+d.IP+"/ota/check", nil)
	resp, err := client.Do(req)
	if err != nil {
		res.Status = "error"
		res.Detail(err)
		return res
	}
	defer resp.Body.Close()
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		res.Status = "error"
		res.Detail(err)
		return res
	}
	res.AvailableVer = stringValue(payload["new_version"])
	res.UpdateAvailable = boolValue(payload["has_update"])
	if res.AvailableVer == res.CurrentVer {
		res.UpdateAvailable = false
		res.AvailableVer = ""
	}
	if res.UpdateAvailable {
		res.Status = "update"
	} else {
		res.Status = "current"
	}
	return res
}

func TriggerUpdate(ctx context.Context, ip string, gen int, stage string, timeout time.Duration) UpdateResult {
	client := &http.Client{Timeout: timeout}
	if gen >= 2 {
		body, _ := json.Marshal(map[string]any{"stage": stage})
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+ip+"/rpc/Shelly.Update", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return UpdateResult{IP: ip, Status: "failed", Detail: err.Error()}
		}
		resp.Body.Close()
		if resp.StatusCode >= 400 {
			return UpdateResult{IP: ip, Status: "failed", Detail: resp.Status}
		}
		return UpdateResult{IP: ip, Status: "triggered", Detail: "update started"}
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+ip+"/ota?update=1", nil)
	resp, err := client.Do(req)
	if err != nil {
		return UpdateResult{IP: ip, Status: "failed", Detail: err.Error()}
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return UpdateResult{IP: ip, Status: "failed", Detail: resp.Status}
	}
	return UpdateResult{IP: ip, Status: "triggered", Detail: "update started"}
}

func pickStage(payload map[string]map[string]any, stage string) map[string]any {
	if stage == "beta" {
		if beta, ok := payload["beta"]; ok && stringValue(beta["version"]) != "" {
			return beta
		}
	}
	if stable, ok := payload["stable"]; ok {
		return stable
	}
	return payload["beta"]
}

func stageNote(payload map[string]map[string]any, stage string) string {
	if stage == "beta" && stringValue(payload["beta"]["version"]) != "" {
		return "beta"
	}
	return ""
}

func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func boolValue(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

func (r *Result) Detail(err error) {
	r.Note = err.Error()
}
