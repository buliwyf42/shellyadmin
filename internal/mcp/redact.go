package mcp

import "shellyadmin/internal/models"

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
