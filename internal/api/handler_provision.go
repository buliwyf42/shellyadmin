package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"shellyadmin/internal/services"
)

// Provision applies a template to a batch of IP addresses. The template
// is the same JSON the Provisioning UI builds; credential_ref selects
// which stored credential group the provisioner authenticates with.
func (h *Handler) Provision(c *gin.Context) {
	var req struct {
		IPs           []string               `json:"ips"`
		Template      map[string]interface{} `json:"template"`
		CredentialRef string                 `json:"credential_ref"`
	}
	if err := decodeJSON(c, &req, services.MaxJSONBytes); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	results, err := h.service.Provision(c.Request.Context(), req.IPs, req.Template, req.CredentialRef)
	if err != nil {
		h.respondUserError(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, results)
}

// UploadUserCA uploads a PEM-encoded certificate bundle to the listed
// Shelly devices. Kind selects the on-device storage slot
// (user_ca / tls_client_cert / tls_client_key).
func (h *Handler) UploadUserCA(c *gin.Context) {
	var req struct {
		IPs  []string `json:"ips"`
		Kind string   `json:"kind"`
		PEM  string   `json:"pem"`
	}
	// PEM cap (MaxUserCABytes) plus headroom for the IP list and JSON envelope.
	if err := decodeJSON(c, &req, services.MaxUserCABytes+32*1024); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	results, err := h.service.UploadUserCA(c.Request.Context(), req.IPs, req.Kind, req.PEM)
	if err != nil {
		h.respondUserError(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, results)
}
