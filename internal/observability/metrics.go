// Package observability hosts the operator-facing observability
// surface: the optional /metrics Prometheus endpoint (M4) and any
// future health/readiness helpers (M10).
//
// The package is deliberately Go-stdlib-only — no Prometheus client
// library — because the v0.2.x metrics surface is small (~10
// counters/gauges) and the value of an extra runtime dependency for
// formatting text/plain output does not pay back here. A future
// expansion (histograms, summary buckets) is the trigger to swap in
// `prometheus/client_golang`.
package observability

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// metric holds a single counter or gauge. Counters monotonically
// increase via Add(); gauges are written via Set(). Both serialise
// identically in the Prometheus text exposition format — the only
// difference is the `# TYPE` line.
type metric struct {
	kind     string // "counter" | "gauge"
	help     string
	value    atomic.Int64
	labelled bool
	// labelledValues backs the labelled-counter path (one bucket
	// per concrete label-value combination, joined by `|`).
	labelledMu     sync.RWMutex
	labelledValues map[string]*atomic.Int64
}

// Registry is the goroutine-safe in-process metric store the HTTP
// handler reads from. Created once at startup; passed to the HTTP
// router which mounts /metrics on its own listener (loopback by
// default, see M4 in the consolidated review).
type Registry struct {
	mu      sync.RWMutex
	metrics map[string]*metric
}

// NewRegistry constructs an empty registry. Callers register every
// metric they intend to emit before any HTTP request can hit
// ServeHTTP — there is no lazy-create on read, so a missing metric
// returns zero values rather than auto-registering.
func NewRegistry() *Registry {
	return &Registry{metrics: map[string]*metric{}}
}

// RegisterCounter declares a monotonically-increasing counter. Help
// is the operator-readable description that follows `# HELP` in the
// exposition output.
func (r *Registry) RegisterCounter(name, help string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.metrics[name] = &metric{kind: "counter", help: help}
}

// RegisterLabelledCounter is the same as RegisterCounter but
// supports label dimensions (e.g. `mcp_calls_total{tool=...}`).
// Label-value combinations are created on first Add.
func (r *Registry) RegisterLabelledCounter(name, help string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.metrics[name] = &metric{kind: "counter", help: help, labelled: true, labelledValues: map[string]*atomic.Int64{}}
}

// RegisterGauge declares a gauge — a value that can move up or down.
// Used for "current state" measurements (online devices, audit-log
// row count, etc.).
func (r *Registry) RegisterGauge(name, help string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.metrics[name] = &metric{kind: "gauge", help: help}
}

// Inc increments a counter by 1. No-op when name is unregistered.
func (r *Registry) Inc(name string) {
	r.mu.RLock()
	m, ok := r.metrics[name]
	r.mu.RUnlock()
	if !ok || m.kind != "counter" || m.labelled {
		return
	}
	m.value.Add(1)
}

// IncLabelled increments a labelled counter for the given label-value
// combination. Labels are sorted by key before joining to ensure
// `{a=1,b=2}` and `{b=2,a=1}` map to the same bucket.
func (r *Registry) IncLabelled(name string, labels map[string]string) {
	r.mu.RLock()
	m, ok := r.metrics[name]
	r.mu.RUnlock()
	if !ok || !m.labelled {
		return
	}
	key := encodeLabels(labels)
	m.labelledMu.Lock()
	v, ok := m.labelledValues[key]
	if !ok {
		v = &atomic.Int64{}
		m.labelledValues[key] = v
	}
	m.labelledMu.Unlock()
	v.Add(1)
}

// Set writes a gauge to the given value. No-op when name is
// unregistered or the metric is a counter.
func (r *Registry) Set(name string, value int64) {
	r.mu.RLock()
	m, ok := r.metrics[name]
	r.mu.RUnlock()
	if !ok || m.kind != "gauge" {
		return
	}
	m.value.Store(value)
}

// ServeHTTP emits the registered metrics in the Prometheus text
// exposition format (v0.0.4) so any Prometheus / VictoriaMetrics /
// Grafana Agent / OTel collector can scrape them. Sorted by name so
// the output diffs cleanly between scrapes.
func (r *Registry) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.metrics))
	for name := range r.metrics {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		writeMetric(w, name, r.metrics[name])
	}
}

func writeMetric(w io.Writer, name string, m *metric) {
	fmt.Fprintf(w, "# HELP %s %s\n", name, m.help)
	fmt.Fprintf(w, "# TYPE %s %s\n", name, m.kind)
	if !m.labelled {
		fmt.Fprintf(w, "%s %d\n", name, m.value.Load())
		return
	}
	m.labelledMu.RLock()
	defer m.labelledMu.RUnlock()
	keys := make([]string, 0, len(m.labelledValues))
	for k := range m.labelledValues {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(w, "%s{%s} %d\n", name, k, m.labelledValues[k].Load())
	}
}

// encodeLabels turns a map into the Prometheus `a="1",b="2"` form
// with stable key ordering for deterministic bucket identity.
func encodeLabels(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		// Escape inner quotes the way the Prometheus exposition
		// format expects. Backslash-escape backslashes first, then
		// double-quotes, then line breaks.
		v := strings.ReplaceAll(labels[k], `\`, `\\`)
		v = strings.ReplaceAll(v, `"`, `\"`)
		v = strings.ReplaceAll(v, "\n", `\n`)
		parts = append(parts, fmt.Sprintf(`%s="%s"`, k, v))
	}
	return strings.Join(parts, ",")
}
