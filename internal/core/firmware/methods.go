package firmware

import (
	"context"
	"errors"
	"sort"

	"shellyadmin/internal/core/shellyclient"
)

// ListSupportedMethods returns the device's Shelly.ListMethods output as a
// sorted slice of method names. Used to drive ShellyAdmin's per-device
// action catalog: actions whose RequiredMethods aren't in this list are
// hidden from the per-device action surface entirely (rather than rendered
// as "supported: false" or letting the failure surface at runtime).
//
// Errors bubble up so callers can decide whether to treat a missing method
// list as "unknown set" (best-effort: keep prior cached value, fall back to
// a conservative hardcoded set) or as a hard probe failure.
func ListSupportedMethods(ctx context.Context, ip string, gen int, opts Options) ([]string, error) {
	if gen < 2 {
		return nil, errors.New("gen1 devices not supported")
	}
	client := shellyclient.New(opts.toClientOptions())
	return ListSupportedMethodsOnClient(ctx, client, ip)
}

// ListSupportedMethodsOnClient is the variant for callers that already have
// a configured shellyclient.Client (e.g. provisioner section handlers).
func ListSupportedMethodsOnClient(ctx context.Context, client *shellyclient.Client, ip string) ([]string, error) {
	payload, err := client.RPC(ctx, ip, "Shelly.ListMethods", nil)
	if err != nil {
		return nil, err
	}
	rawList, _ := payload["methods"].([]any)
	out := make([]string, 0, len(rawList))
	for _, m := range rawList {
		s, ok := m.(string)
		if !ok || s == "" {
			continue
		}
		out = append(out, s)
	}
	sort.Strings(out)
	return out, nil
}
