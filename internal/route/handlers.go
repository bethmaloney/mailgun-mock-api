package route

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/bethmaloney/mailgun-mock-api/internal/database"
	"github.com/bethmaloney/mailgun-mock-api/internal/response"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// Route is the GORM model for inbound email routes.
type Route struct {
	database.BaseModel
	RouteID     string `gorm:"uniqueIndex;size:24"`
	Priority    int
	Description string
	Expression  string
	Actions     string // JSON-encoded string array
}

// Handlers holds the route HTTP handlers.
type Handlers struct {
	db *gorm.DB
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(db *gorm.DB) *Handlers {
	return &Handlers{db: db}
}

// generateRouteID generates a 24-character lowercase hex string ID.
func generateRouteID() string {
	b := make([]byte, 12) // 12 bytes = 24 hex chars
	rand.Read(b)
	return hex.EncodeToString(b)
}

// toJSON converts a Route model to a JSON-serializable map.
func (rt *Route) toJSON() map[string]interface{} {
	var actions []string
	json.Unmarshal([]byte(rt.Actions), &actions)
	if actions == nil {
		actions = []string{}
	}
	return map[string]interface{}{
		"id":          rt.RouteID,
		"priority":    rt.Priority,
		"description": rt.Description,
		"expression":  rt.Expression,
		"actions":     actions,
		"created_at":  rt.CreatedAt.UTC().Format("Mon, 02 Jan 2006 15:04:05 MST"),
	}
}

// CreateRoute handles POST /v3/routes.
func (h *Handlers) CreateRoute(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		response.RespondError(w, http.StatusBadRequest, "Failed to parse form")
		return
	}

	expression := r.Form.Get("expression")
	if expression == "" {
		response.RespondError(w, http.StatusBadRequest, "expression is required")
		return
	}

	actions := r.Form["action"]
	if len(actions) == 0 {
		response.RespondError(w, http.StatusBadRequest, "At least one action is required")
		return
	}

	priority := 0
	if p := r.Form.Get("priority"); p != "" {
		parsed, err := strconv.Atoi(p)
		if err != nil {
			response.RespondError(w, http.StatusBadRequest, "Invalid priority value")
			return
		}
		priority = parsed
	}

	description := r.Form.Get("description")

	actionsJSON, _ := json.Marshal(actions)

	rt := Route{
		RouteID:     generateRouteID(),
		Priority:    priority,
		Description: description,
		Expression:  expression,
		Actions:     string(actionsJSON),
	}

	if err := h.db.Create(&rt).Error; err != nil {
		response.RespondError(w, http.StatusInternalServerError, "Failed to create route")
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Route has been created",
		"route":   rt.toJSON(),
	})
}

// ListRoutes handles GET /v3/routes.
func (h *Handlers) ListRoutes(w http.ResponseWriter, r *http.Request) {
	// Parse limit and skip manually to validate limit > 1000
	limitStr := r.URL.Query().Get("limit")
	skipStr := r.URL.Query().Get("skip")

	limit := 100
	skip := 0

	if limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err != nil {
			response.RespondError(w, http.StatusBadRequest, "Invalid limit value")
			return
		}
		if parsed > 1000 {
			response.RespondError(w, http.StatusBadRequest, "The 'limit' parameter can't be larger than 1000")
			return
		}
		limit = parsed
	}

	if skipStr != "" {
		parsed, err := strconv.Atoi(skipStr)
		if err != nil {
			response.RespondError(w, http.StatusBadRequest, "Invalid skip value")
			return
		}
		skip = parsed
	}

	var totalCount int64
	h.db.Model(&Route{}).Count(&totalCount)

	var routes []Route
	h.db.Order("priority ASC, created_at ASC").Offset(skip).Limit(limit).Find(&routes)

	items := make([]map[string]interface{}, 0, len(routes))
	for i := range routes {
		items = append(items, routes[i].toJSON())
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"total_count": totalCount,
		"items":       items,
	})
}

// GetRoute handles GET /v3/routes/{route_id}.
func (h *Handlers) GetRoute(w http.ResponseWriter, r *http.Request) {
	routeID := chi.URLParam(r, "route_id")

	var rt Route
	if err := h.db.Where("route_id = ?", routeID).First(&rt).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Route not found")
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"route": rt.toJSON(),
	})
}

// UpdateRoute handles PUT /v3/routes/{route_id}.
func (h *Handlers) UpdateRoute(w http.ResponseWriter, r *http.Request) {
	routeID := chi.URLParam(r, "route_id")

	var rt Route
	if err := h.db.Where("route_id = ?", routeID).First(&rt).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Route not found")
		return
	}

	if err := r.ParseForm(); err != nil {
		response.RespondError(w, http.StatusBadRequest, "Failed to parse form")
		return
	}

	// Partial update: only update fields that are present
	if p := r.Form.Get("priority"); p != "" {
		parsed, err := strconv.Atoi(p)
		if err != nil {
			response.RespondError(w, http.StatusBadRequest, "Invalid priority value")
			return
		}
		rt.Priority = parsed
	}

	if _, exists := r.Form["description"]; exists {
		rt.Description = r.Form.Get("description")
	}

	if expr := r.Form.Get("expression"); expr != "" {
		rt.Expression = expr
	}

	if actions := r.Form["action"]; len(actions) > 0 {
		actionsJSON, _ := json.Marshal(actions)
		rt.Actions = string(actionsJSON)
	}

	if err := h.db.Save(&rt).Error; err != nil {
		response.RespondError(w, http.StatusInternalServerError, "Failed to update route")
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Route has been updated",
		"route":   rt.toJSON(),
	})
}

// DeleteRoute handles DELETE /v3/routes/{route_id}.
func (h *Handlers) DeleteRoute(w http.ResponseWriter, r *http.Request) {
	routeID := chi.URLParam(r, "route_id")

	var rt Route
	if err := h.db.Where("route_id = ?", routeID).First(&rt).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Route not found")
		return
	}

	// Hard delete (unscoped to bypass soft delete)
	if err := h.db.Unscoped().Delete(&rt).Error; err != nil {
		response.RespondError(w, http.StatusInternalServerError, "Failed to delete route")
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Route has been deleted",
		"id":      routeID,
	})
}

// MatchRoute handles GET /v3/routes/match.
func (h *Handlers) MatchRoute(w http.ResponseWriter, r *http.Request) {
	address := r.URL.Query().Get("address")
	if address == "" {
		response.RespondError(w, http.StatusBadRequest, "address parameter is required")
		return
	}

	var routes []Route
	h.db.Order("priority ASC, created_at ASC").Find(&routes)

	for _, rt := range routes {
		if evaluateExpression(rt.Expression, address) {
			response.RespondJSON(w, http.StatusOK, map[string]interface{}{
				"route": rt.toJSON(),
			})
			return
		}
	}

	response.RespondError(w, http.StatusNotFound, "Route not found")
}

// SimulateInbound handles POST /mock/inbound/{domain}.
func (h *Handlers) SimulateInbound(w http.ResponseWriter, r *http.Request) {
	// Parse JSON body
	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.RespondError(w, http.StatusBadRequest, "Failed to parse JSON body")
		return
	}

	// Extract recipient address
	recipient, _ := body["recipient"].(string)
	if recipient == "" {
		recipient, _ = body["to"].(string)
	}

	var routes []Route
	h.db.Order("priority ASC, created_at ASC").Find(&routes)

	matchedRoutes := make([]string, 0)
	actionsExecuted := make([]string, 0)
	previousMatched := false

	for _, rt := range routes {
		// For catch_all, only trigger if no preceding routes matched
		isCatchAll := isCatchAllExpression(rt.Expression)

		matched := false
		if isCatchAll {
			if !previousMatched {
				matched = true
			}
		} else {
			matched = evaluateExpression(rt.Expression, recipient)
		}

		if matched {
			previousMatched = true
			matchedRoutes = append(matchedRoutes, rt.RouteID)

			var actions []string
			json.Unmarshal([]byte(rt.Actions), &actions)
			actionsExecuted = append(actionsExecuted, actions...)

			// Check if stop() action is present
			for _, action := range actions {
				if action == "stop()" {
					response.RespondJSON(w, http.StatusOK, map[string]interface{}{
						"message":          "Inbound message processed",
						"matched_routes":   matchedRoutes,
						"actions_executed": actionsExecuted,
					})
					return
				}
			}
		}
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message":          "Inbound message processed",
		"matched_routes":   matchedRoutes,
		"actions_executed": actionsExecuted,
	})
}
