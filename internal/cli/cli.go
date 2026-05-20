// Package cli implements the read-only `shellyctl` operator CLI: a thin
// HTTP client over the running instance's /api surface, authenticated with a
// Personal Access Token. See docs/adr/0016-shellyctl-cli.md.
package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

// commands maps the first CLI verb to its handler. cmd/shellyctl/main.go
// routes a matching os.Args[1] here; everything else starts the server.
var commands = map[string]func(*client, []string) int{
	"devices": cmdDevices,
	"device":  cmdDevice,
	"logs":    cmdLogs,
}

// Handles reports whether name is a CLI verb (so main.go knows to dispatch
// here instead of booting the server).
func Handles(name string) bool {
	_, ok := commands[name]
	return ok
}

// Run executes args[0] as a CLI command and returns a process exit code.
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, usage())
		return 2
	}
	handler, ok := commands[args[0]]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s\n", args[0], usage())
		return 2
	}
	return handler(newClient(), args[1:])
}

func usage() string {
	return strings.TrimSpace(`
shellyctl — read-only fleet CLI (talks to a running instance via /api)

Commands:
  devices              list the device inventory
  device <mac|ip|name> show one device's detail
  logs                 show the audit log

Global flags (or env):
  --url    base URL    (default http://localhost:8080, env SHELLYADMIN_URL)
  --token  PAT         (env SHELLYADMIN_TOKEN; format pat_<id>_<secret>)
  --json   emit raw API JSON instead of a table`)
}

// client carries the resolved transport config for one invocation.
type client struct {
	url     string
	token   string
	jsonOut bool
	http    *http.Client
}

func newClient() *client {
	return &client{http: &http.Client{Timeout: 15 * time.Second}}
}

// bindFlags registers the global flags on fs and binds them to the client.
// Env vars supply defaults so scripted callers can omit the flags.
func (c *client) bindFlags(fs *flag.FlagSet) {
	fs.StringVar(&c.url, "url", envOr("SHELLYADMIN_URL", "http://localhost:8080"),
		"base URL of the running instance")
	fs.StringVar(&c.token, "token", os.Getenv("SHELLYADMIN_TOKEN"),
		"personal access token (pat_...)")
	fs.BoolVar(&c.jsonOut, "json", false, "emit raw API JSON")
}

// get fetches path (e.g. "/api/devices") and decodes the JSON body into out.
// Returns a user-facing error string (empty on success).
func (c *client) get(path string, out any) string {
	if strings.TrimSpace(c.token) == "" {
		return "no token: set --token or SHELLYADMIN_TOKEN (mint one in Settings → Personal Access Tokens)"
	}
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(c.url, "/")+path, nil)
	if err != nil {
		return fmt.Sprintf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Sprintf("request %s: %v", c.url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Sprintf("auth failed (HTTP %d) — check the token and its scopes", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if c.jsonOut {
		printJSON(body)
		return ""
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Sprintf("decode response: %v", err)
	}
	return ""
}

func cmdDevices(c *client, args []string) int {
	fs := flag.NewFlagSet("shellyctl devices", flag.ContinueOnError)
	c.bindFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	var devices []deviceRow
	if msg := c.get("/api/devices", &devices); msg != "" {
		return fail(msg)
	}
	if c.jsonOut {
		return 0
	}
	if len(devices) == 0 {
		fmt.Println("no devices")
		return 0
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "#\tNAME\tIP\tMAC\tMODEL\tGEN\tONLINE\tFW")
	for _, d := range devices {
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
			d.DeviceNum, dash(d.Name), dash(d.IP), dash(d.MAC), dash(d.Model),
			d.Gen, yesno(d.Online), dash(d.FW))
	}
	_ = w.Flush()
	return 0
}

func cmdDevice(c *client, args []string) int {
	fs := flag.NewFlagSet("shellyctl device", flag.ContinueOnError)
	c.bindFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	target := fs.Arg(0)
	if target == "" {
		return fail("usage: shellyctl device [flags] <mac|ip|name>")
	}
	var detail map[string]any
	if msg := c.get("/api/devices/"+target, &detail); msg != "" {
		return fail(msg)
	}
	if c.jsonOut {
		return 0
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	for _, k := range sortedKeys(detail) {
		if s, ok := scalarString(detail[k]); ok {
			fmt.Fprintf(w, "%s\t%s\n", k, s)
		}
	}
	_ = w.Flush()
	return 0
}

func cmdLogs(c *client, args []string) int {
	fs := flag.NewFlagSet("shellyctl logs", flag.ContinueOnError)
	c.bindFlags(fs)
	level := fs.String("level", "", "filter by level (INFO|WARN|ERROR)")
	search := fs.String("search", "", "substring match on the message")
	risk := fs.String("risk", "", "filter by risk (low|medium|high)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	path := fmt.Sprintf("/api/logs?level=%s&search=%s&risk=%s",
		urlValue(*level), urlValue(*search), urlValue(*risk))
	var logs []logRow
	if msg := c.get(path, &logs); msg != "" {
		return fail(msg)
	}
	if c.jsonOut {
		return 0
	}
	if len(logs) == 0 {
		fmt.Println("no log entries")
		return 0
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "TIMESTAMP\tLEVEL\tRISK\tREQUEST\tMESSAGE")
	for _, l := range logs {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			dash(l.TS), dash(l.Level), dash(l.RiskLevel), dash(l.RequestID), l.Message)
	}
	_ = w.Flush()
	return 0
}

// --- response shapes (only the fields the CLI renders) ---

type deviceRow struct {
	DeviceNum int    `json:"device_num"`
	Name      string `json:"name"`
	IP        string `json:"ip"`
	MAC       string `json:"mac"`
	Model     string `json:"model"`
	Gen       int    `json:"gen"`
	Online    bool   `json:"online"`
	FW        string `json:"fw"`
}

type logRow struct {
	TS        string `json:"ts"`
	Level     string `json:"level"`
	RiskLevel string `json:"risk_level"`
	RequestID string `json:"request_id"`
	Message   string `json:"message"`
}

// --- helpers ---

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// scalarString renders a JSON scalar for the detail key/value view and
// reports false for objects/arrays (skipped to keep the view flat).
func scalarString(v any) (string, bool) {
	switch t := v.(type) {
	case nil:
		return "", false
	case string:
		return t, true
	case bool:
		return fmt.Sprintf("%t", t), true
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t)), true
		}
		return fmt.Sprintf("%g", t), true
	default:
		return "", false
	}
}

func dash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func yesno(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func urlValue(s string) string {
	return strings.ReplaceAll(s, " ", "%20")
}

func printJSON(body []byte) {
	var pretty any
	if err := json.Unmarshal(body, &pretty); err != nil {
		os.Stdout.Write(body)
		fmt.Println()
		return
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(pretty)
}

func fail(msg string) int {
	fmt.Fprintln(os.Stderr, "error: "+msg)
	return 1
}
