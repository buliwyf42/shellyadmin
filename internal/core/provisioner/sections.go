package provisioner

import (
	"sort"
	"strings"
)

// knownTemplateSections is the canonical answer to "is this top-level
// template key something ApplyTemplate handles deliberately?". It feeds
// validation.Template so a typo'd section name ("syss") fails at
// template-save time instead of being routed to the <Capitalized>.SetConfig
// catch-all and silently 404-ing at every device during a fleet provision
// run. applySection itself keeps the catch-all so templates already stored
// in the DB provision unchanged.
//
// Three groups:
//   - sections with an explicit applySection case
//   - device-level components served correctly by the catch-all — Shelly
//     RPC method matching is case-insensitive, so ui → "Ui.SetConfig"
//     reaches UI.SetConfig
//   - ota: form + normalizer removed in v0.0.16 but still tolerated in
//     stored templates; the catch-all 404s and the section reports skipped
var knownTemplateSections = map[string]struct{}{
	// explicit applySection cases
	"gen2_rpc":    {},
	"gen1_http":   {},
	"mqtt":        {},
	"sys":         {},
	"ws":          {},
	"ble":         {},
	"matter":      {},
	"cloud":       {},
	"wifi":        {},
	"eth":         {},
	"kvs":         {},
	"script":      {},
	"auth":        {},
	"cover":       {},
	"lnm":         {},
	"auto_update": {},
	"webhooks":    {},
	// catch-all-served device-level components
	"ui":       {},
	"modbus":   {},
	"zigbee":   {},
	"knx":      {},
	"bthome":   {},
	"plugs_ui": {},
	// legacy, tolerated
	"ota": {},
}

// KnownTemplateSection reports whether a top-level template key is one
// ApplyTemplate handles deliberately. Matching mirrors applySection exactly:
// lowercased, NOT trimmed — "sys " would dispatch a broken "Sys .SetConfig"
// at runtime, so it must not pass validation either.
func KnownTemplateSection(name string) bool {
	_, ok := knownTemplateSections[strings.ToLower(name)]
	return ok
}

// KnownTemplateSections returns the sorted section list for error messages.
func KnownTemplateSections() []string {
	out := make([]string, 0, len(knownTemplateSections))
	for s := range knownTemplateSections {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
