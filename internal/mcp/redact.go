package mcp

import "shellyadmin/internal/models"

// Redaction inventory for the MCP surface: list_credentials goes through
// RedactedCredential below; get_settings (tools.go) masks the decrypted
// MCPToken to services.MCPTokenRedacted. Any new tool that returns a type
// carrying secret material must add its redactor here or inline before
// the tool ships — see CLAUDE.md "Secret hygiene".

// RedactedCredential is the safe shape returned by list_credentials. It
// deliberately omits Password and HA1 so the bearer-token holder cannot
// pull plaintext secrets out of the credential store via MCP.
type RedactedCredential struct {
	Name     string   `json:"name"`
	Username string   `json:"username"`
	Tags     []string `json:"tags"`
}

func redactCredentials(in []models.Credential) []RedactedCredential {
	out := make([]RedactedCredential, 0, len(in))
	for _, c := range in {
		out = append(out, RedactedCredential{
			Name:     c.Name,
			Username: c.Username,
			Tags:     append([]string(nil), c.Tags...),
		})
	}
	return out
}
