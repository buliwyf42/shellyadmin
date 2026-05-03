package scanner

import "encoding/json"

// jsonMarshal is a thin wrapper so the rest of the package can stay free of
// encoding/json imports — keeping import lists tight makes the probe path
// easier to scan.
func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }
