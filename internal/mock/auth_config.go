package mock

import (
	"fmt"
	"net/http"

	"github.com/bethmaloney/mailgun-mock-api/internal/response"
)

// GetAuthConfig returns the authentication configuration for the frontend.
func (h *Handlers) GetAuthConfig(w http.ResponseWriter, r *http.Request) {
	if h.cfg.AuthMode != "entra" {
		response.RespondJSON(w, http.StatusOK, map[string]interface{}{
			"enabled": false,
		})
		return
	}
	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"enabled":     true,
		"tenantId":    h.cfg.EntraTenantID,
		"clientId":    h.cfg.EntraClientID,
		"scopes":      []string{fmt.Sprintf("api://%s/%s", h.cfg.EntraClientID, h.cfg.EntraAPIScope)},
		"redirectUri": h.cfg.EntraRedirectURI,
	})
}
