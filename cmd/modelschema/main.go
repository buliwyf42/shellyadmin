// modelschema emits a canonical JSON representation of the public
// `internal/models.*` structs that the SPA depends on. CI uses
// `go run ./cmd/modelschema --check` to fail the build when a Go
// struct field is added/removed/renamed without a matching update
// in web/src/lib/types.ts.
//
// This is the lightweight Phase 3 / M3 stand-in for a full Go→TS
// codegen pipeline. Drift prevention is the main M3 deliverable;
// regenerating the entire types.ts on every model change costs
// more than it pays back at the current rate of schema evolution.
// A future move to tygo / typegen-go is a non-breaking follow-up
// because the canonical schema this tool emits is the bridge.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"

	"shellyadmin/internal/models"
)

// emitted is the canonical wire shape: every tracked struct flat-listed
// with its JSON field name, Go type, and json-tag options (omitempty
// etc.). Diff-friendly (one field per line in `json.MarshalIndent`).
type emitted struct {
	Structs map[string]structSchema `json:"structs"`
}

type structSchema struct {
	Fields []fieldSchema `json:"fields"`
}

type fieldSchema struct {
	Name     string   `json:"name"`
	JSONName string   `json:"json_name"`
	GoType   string   `json:"go_type"`
	Tag      string   `json:"tag,omitempty"`
	Options  []string `json:"options,omitempty"`
}

// tracked is the explicit allowlist of structs the SPA serialises.
// Adding a new top-level model here is the deliberate gate the M3
// drift check enforces — silently exporting a new struct over the API
// without listing it here is a CI failure.
var tracked = map[string]reflect.Type{
	"AppSettings":                     reflect.TypeOf(models.AppSettings{}),
	"ComplianceRules":                 reflect.TypeOf(models.ComplianceRules{}),
	"CustomRule":                      reflect.TypeOf(models.CustomRule{}),
	"Device":                          reflect.TypeOf(models.Device{}),
	"Credential":                      reflect.TypeOf(models.Credential{}),
	"CredentialGroup":                 reflect.TypeOf(models.CredentialGroup{}),
	"DeviceCredentialGroupAssignment": reflect.TypeOf(models.DeviceCredentialGroupAssignment{}),
	"Job":                             reflect.TypeOf(models.Job{}),
}

func main() {
	check := flag.Bool("check", false, "Verify that internal/models/schema.gen.json matches the live struct state. Non-zero exit on drift.")
	output := flag.String("output", "internal/models/schema.gen.json", "Path to write the canonical schema (when --check is not set).")
	flag.Parse()

	body, err := generate()
	if err != nil {
		fmt.Fprintln(os.Stderr, "generate:", err)
		os.Exit(1)
	}

	if *check {
		existing, readErr := os.ReadFile(*output)
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "schema check: cannot read %s: %v\n", *output, readErr)
			os.Exit(2)
		}
		if !bytes.Equal(bytes.TrimSpace(existing), bytes.TrimSpace(body)) {
			fmt.Fprintln(os.Stderr, "schema drift detected — regenerate with `go run ./cmd/modelschema`:")
			fmt.Fprintln(os.Stderr, "  expected (live struct state) vs. committed (internal/models/schema.gen.json) differ.")
			os.Exit(3)
		}
		fmt.Println("schema OK")
		return
	}

	if err := os.WriteFile(*output, body, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "write:", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%d structs)\n", *output, len(tracked))
}

func generate() ([]byte, error) {
	out := emitted{Structs: map[string]structSchema{}}
	for name, t := range tracked {
		out.Structs[name] = describe(t)
	}
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func describe(t reflect.Type) structSchema {
	if t.Kind() != reflect.Struct {
		return structSchema{}
	}
	var fields []fieldSchema
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		tag := f.Tag.Get("json")
		// Treat `json:"-"` as "not serialised over the wire" — the SPA
		// never sees these, so they aren't part of the drift surface.
		if tag == "-" {
			continue
		}
		jsonName, opts := parseJSONTag(tag, f.Name)
		fields = append(fields, fieldSchema{
			Name:     f.Name,
			JSONName: jsonName,
			GoType:   describeType(f.Type),
			Tag:      tag,
			Options:  opts,
		})
	}
	sort.Slice(fields, func(i, j int) bool { return fields[i].JSONName < fields[j].JSONName })
	return structSchema{Fields: fields}
}

// parseJSONTag returns the wire name + the option flags (omitempty,
// string, etc.). Falls back to the Go field name if no json tag is set,
// matching encoding/json's runtime behaviour.
func parseJSONTag(tag, fallback string) (string, []string) {
	if tag == "" {
		return fallback, nil
	}
	parts := strings.Split(tag, ",")
	name := parts[0]
	if name == "" {
		name = fallback
	}
	var opts []string
	if len(parts) > 1 {
		opts = parts[1:]
		sort.Strings(opts)
	}
	return name, opts
}

// describeType prints a stable, parser-friendly representation of a Go
// type. Pointers become "*T"; slices become "[]T"; maps become
// "map[K]V"; everything else is the Go-source rendering.
func describeType(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Ptr:
		return "*" + describeType(t.Elem())
	case reflect.Slice, reflect.Array:
		return "[]" + describeType(t.Elem())
	case reflect.Map:
		return "map[" + describeType(t.Key()) + "]" + describeType(t.Elem())
	default:
		// Named types render as `pkg.Name`; primitives as their kind.
		if t.PkgPath() != "" {
			return t.PkgPath() + "." + t.Name()
		}
		return t.String()
	}
}
