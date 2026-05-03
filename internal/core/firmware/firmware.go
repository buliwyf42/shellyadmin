package firmware

import (
	"context"
	"errors"
	"time"

	"shellyadmin/internal/core/shellyclient"
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

// Options carries the per-device configuration used to build a shellyclient.
type Options struct {
	Timeout       time.Duration
	Scheme        string
	Username      string
	Password      string
	HA1           string
	AllowInsecure bool
}

func (o Options) toClientOptions() shellyclient.Options {
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

// CheckOne preserves the existing signature for callers that don't yet thread
// credentials/scheme; it delegates to CheckOneWithOptions internally.
func CheckOne(ctx context.Context, d models.Device, stage string, timeout time.Duration) Result {
	return CheckOneWithOptions(ctx, d, stage, Options{Timeout: timeout})
}

// CheckOneWithOptions issues Shelly.CheckForUpdate via shellyclient so digest
// auth and lockout signalling are honoured. Gen1 devices are unsupported.
func CheckOneWithOptions(ctx context.Context, d models.Device, stage string, opts Options) Result {
	res := Result{IP: d.IP, MAC: d.MAC, CurrentVer: d.FW, Status: "na", Stage: stage}
	if d.Gen < 2 {
		// Gen1 devices are not supported; the rest of the codebase already
		// rejects them, but be defensive here too.
		res.Status = "na"
		res.Note = "gen1 devices not supported"
		return res
	}
	client := shellyclient.New(opts.toClientOptions())
	payload, err := client.RPC(ctx, d.IP, "Shelly.CheckForUpdate", nil)
	if err != nil {
		res.Status = "error"
		res.Detail(err)
		return res
	}
	stagePayload := pickStage(payload, stage)
	res.AvailableVer = stringValue(stagePayload["version"])
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

// TriggerUpdate retains the original signature; callers wishing to thread
// credentials/scheme should use TriggerUpdateWithOptions.
func TriggerUpdate(ctx context.Context, ip string, gen int, stage string, timeout time.Duration) UpdateResult {
	return TriggerUpdateWithOptions(ctx, ip, gen, stage, Options{Timeout: timeout})
}

func TriggerUpdateWithOptions(ctx context.Context, ip string, gen int, stage string, opts Options) UpdateResult {
	if gen < 2 {
		return UpdateResult{IP: ip, Status: "failed", Detail: "gen1 devices not supported"}
	}
	client := shellyclient.New(opts.toClientOptions())
	_, err := client.RPC(ctx, ip, "Shelly.Update", map[string]any{"stage": stage})
	if err != nil {
		if errors.Is(err, shellyclient.ErrAuthRequired) {
			return UpdateResult{IP: ip, Status: "failed", Detail: "authentication required"}
		}
		if errors.Is(err, shellyclient.ErrAuthLockout) {
			return UpdateResult{IP: ip, Status: "failed", Detail: "device locked (brute-force protection)"}
		}
		return UpdateResult{IP: ip, Status: "failed", Detail: err.Error()}
	}
	return UpdateResult{IP: ip, Status: "triggered", Detail: "update started"}
}

func pickStage(payload map[string]any, stage string) map[string]any {
	if stage == "beta" {
		if beta, ok := payload["beta"].(map[string]any); ok && stringValue(beta["version"]) != "" {
			return beta
		}
	}
	if stable, ok := payload["stable"].(map[string]any); ok {
		return stable
	}
	if beta, ok := payload["beta"].(map[string]any); ok {
		return beta
	}
	return map[string]any{}
}

func stageNote(payload map[string]any, stage string) string {
	if stage != "beta" {
		return ""
	}
	if beta, ok := payload["beta"].(map[string]any); ok && stringValue(beta["version"]) != "" {
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

func (r *Result) Detail(err error) {
	r.Note = err.Error()
}
