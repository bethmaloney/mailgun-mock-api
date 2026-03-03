package ip

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/bethmaloney/mailgun-mock-api/internal/database"
	"github.com/bethmaloney/mailgun-mock-api/internal/domain"
	"github.com/bethmaloney/mailgun-mock-api/internal/response"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// Models

type IP struct {
	database.BaseModel
	Address   string `gorm:"uniqueIndex" json:"ip"`
	RDNS      string `json:"rdns"`
	Dedicated bool   `json:"dedicated"`
}

type IPPool struct {
	database.BaseModel
	PoolID      string `gorm:"uniqueIndex;size:24" json:"pool_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type IPPoolIP struct {
	database.BaseModel
	PoolID    string `gorm:"index;uniqueIndex:idx_pool_ip"`
	IPAddress string `gorm:"uniqueIndex:idx_pool_ip"`
}

type DomainIP struct {
	database.BaseModel
	DomainName string `gorm:"index;uniqueIndex:idx_domain_ip"`
	IPAddress  string `gorm:"uniqueIndex:idx_domain_ip"`
}

type DomainPool struct {
	database.BaseModel
	DomainName string `gorm:"uniqueIndex"`
	PoolID     string
}

// Handlers

type Handlers struct {
	db *gorm.DB
}

func generatePoolID() string {
	b := make([]byte, 12) // 12 bytes = 24 hex chars
	rand.Read(b)
	return hex.EncodeToString(b)
}

// parseForm parses both multipart and URL-encoded form data.
func parseForm(r *http.Request) error {
	_ = r.ParseMultipartForm(32 << 20)
	return r.ParseForm()
}

func NewHandlers(db *gorm.DB) *Handlers {
	// Seed default IPs
	db.Where(IP{Address: "127.0.0.1"}).FirstOrCreate(&IP{
		Address:   "127.0.0.1",
		RDNS:      "mock-1.mailgun.net",
		Dedicated: true,
	})
	db.Where(IP{Address: "10.0.0.1"}).FirstOrCreate(&IP{
		Address:   "10.0.0.1",
		RDNS:      "mock-2.mailgun.net",
		Dedicated: true,
	})

	return &Handlers{db: db}
}

// ListIPs handles GET /v3/ips
func (h *Handlers) ListIPs(w http.ResponseWriter, r *http.Request) {
	dedicatedParam := r.URL.Query().Get("dedicated")
	// "enabled" param is accepted but ignored in mock

	var ips []IP
	query := h.db
	if dedicatedParam == "true" {
		query = query.Where("dedicated = ?", true)
	} else if dedicatedParam == "false" {
		query = query.Where("dedicated = ?", false)
	}
	query.Find(&ips)

	items := make([]string, 0, len(ips))
	assignable := make([]string, 0, len(ips))
	for _, ip := range ips {
		items = append(items, ip.Address)
		assignable = append(assignable, ip.Address)
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"items":               items,
		"total_count":         len(items),
		"assignable_to_pools": assignable,
	})
}

// GetIP handles GET /v3/ips/{ip}
func (h *Handlers) GetIP(w http.ResponseWriter, r *http.Request) {
	ipAddr := chi.URLParam(r, "ip")

	var ipRecord IP
	if err := h.db.Where("address = ?", ipAddr).First(&ipRecord).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "IP not found")
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"ip":        ipRecord.Address,
		"rdns":      ipRecord.RDNS,
		"dedicated": ipRecord.Dedicated,
	})
}

// ListDomainIPs handles GET /v3/domains/{name}/ips
func (h *Handlers) ListDomainIPs(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	var domainIPs []DomainIP
	h.db.Where("domain_name = ?", name).Find(&domainIPs)

	type ipItem struct {
		IP        string `json:"ip"`
		RDNS      string `json:"rdns"`
		Dedicated bool   `json:"dedicated"`
	}

	items := make([]ipItem, 0, len(domainIPs))
	for _, di := range domainIPs {
		var ipRecord IP
		if err := h.db.Where("address = ?", di.IPAddress).First(&ipRecord).Error; err == nil {
			items = append(items, ipItem{
				IP:        ipRecord.Address,
				RDNS:      ipRecord.RDNS,
				Dedicated: ipRecord.Dedicated,
			})
		}
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"items":       items,
		"total_count": len(items),
	})
}

// AssignDomainIP handles POST /v3/domains/{name}/ips
func (h *Handlers) AssignDomainIP(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	// Validate domain exists
	var d domain.Domain
	if err := h.db.Where("name = ?", name).First(&d).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Domain not found")
		return
	}

	if err := parseForm(r); err != nil {
		response.RespondError(w, http.StatusBadRequest, "Failed to parse form")
		return
	}

	ipAddr := r.Form.Get("ip")
	poolID := r.Form.Get("pool_id")

	if ipAddr == "" && poolID == "" {
		response.RespondError(w, http.StatusBadRequest, "ip or pool_id is required")
		return
	}

	if ipAddr != "" {
		h.db.Create(&DomainIP{
			DomainName: name,
			IPAddress:  ipAddr,
		})
	} else if poolID != "" {
		h.db.Create(&DomainPool{
			DomainName: name,
			PoolID:     poolID,
		})
	}

	response.RespondSuccess(w, "success")
}

// UnassignDomainIP handles DELETE /v3/domains/{name}/ips/{ip}
func (h *Handlers) UnassignDomainIP(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	ipParam := chi.URLParam(r, "ip")

	switch ipParam {
	case "all":
		h.db.Unscoped().Where("domain_name = ?", name).Delete(&DomainIP{})
	case "ip_pool":
		h.db.Unscoped().Where("domain_name = ?", name).Delete(&DomainPool{})
	default:
		h.db.Unscoped().Where("domain_name = ? AND ip_address = ?", name, ipParam).Delete(&DomainIP{})
	}

	response.RespondSuccess(w, "success")
}

// ListPools handles GET /v1/ip_pools (and /v3/ip_pools)
func (h *Handlers) ListPools(w http.ResponseWriter, r *http.Request) {
	var pools []IPPool
	h.db.Find(&pools)

	type poolItem struct {
		PoolID      string   `json:"pool_id"`
		Name        string   `json:"name"`
		Description string   `json:"description"`
		IPs         []string `json:"ips"`
		IsLinked    bool     `json:"is_linked"`
		IsInherited bool     `json:"is_inherited"`
	}

	items := make([]poolItem, 0, len(pools))
	for _, p := range pools {
		// Get IPs for this pool
		var poolIPs []IPPoolIP
		h.db.Where("pool_id = ?", p.PoolID).Find(&poolIPs)
		ips := make([]string, 0, len(poolIPs))
		for _, pip := range poolIPs {
			ips = append(ips, pip.IPAddress)
		}

		// Check if linked to any domain
		var count int64
		h.db.Model(&DomainPool{}).Where("pool_id = ?", p.PoolID).Count(&count)

		items = append(items, poolItem{
			PoolID:      p.PoolID,
			Name:        p.Name,
			Description: p.Description,
			IPs:         ips,
			IsLinked:    count > 0,
			IsInherited: false,
		})
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"ip_pools": items,
		"message":  "success",
	})
}

// CreatePool handles POST /v1/ip_pools
func (h *Handlers) CreatePool(w http.ResponseWriter, r *http.Request) {
	if err := parseForm(r); err != nil {
		response.RespondError(w, http.StatusBadRequest, "Failed to parse form")
		return
	}

	name := r.Form.Get("name")
	if name == "" {
		response.RespondError(w, http.StatusBadRequest, "Name is required")
		return
	}

	description := r.Form.Get("description")
	ips := r.Form["ip"]

	poolID := generatePoolID()

	pool := IPPool{
		PoolID:      poolID,
		Name:        name,
		Description: description,
	}
	h.db.Create(&pool)

	// Create IPPoolIP records for any provided IPs
	for _, ipAddr := range ips {
		h.db.Create(&IPPoolIP{
			PoolID:    poolID,
			IPAddress: ipAddr,
		})
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "success",
		"pool_id": poolID,
	})
}

// GetPool handles GET /v1/ip_pools/{pool_id}
func (h *Handlers) GetPool(w http.ResponseWriter, r *http.Request) {
	poolID := chi.URLParam(r, "pool_id")

	var pool IPPool
	if err := h.db.Where("pool_id = ?", poolID).First(&pool).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Pool not found")
		return
	}

	// Get IPs for this pool
	var poolIPs []IPPoolIP
	h.db.Where("pool_id = ?", poolID).Find(&poolIPs)
	ips := make([]string, 0, len(poolIPs))
	for _, pip := range poolIPs {
		ips = append(ips, pip.IPAddress)
	}

	// Get linked domains
	var domainPools []DomainPool
	h.db.Where("pool_id = ?", poolID).Find(&domainPools)

	type linkedDomainItem struct {
		Name string `json:"name"`
	}
	linkedDomains := make([]linkedDomainItem, 0, len(domainPools))
	for _, dp := range domainPools {
		linkedDomains = append(linkedDomains, linkedDomainItem{Name: dp.DomainName})
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"pool_id":        pool.PoolID,
		"name":           pool.Name,
		"description":    pool.Description,
		"ips":            ips,
		"is_linked":      len(domainPools) > 0,
		"is_inherited":   false,
		"linked_domains": linkedDomains,
		"message":        "success",
	})
}

// UpdatePool handles PATCH /v1/ip_pools/{pool_id}
func (h *Handlers) UpdatePool(w http.ResponseWriter, r *http.Request) {
	poolID := chi.URLParam(r, "pool_id")

	var pool IPPool
	if err := h.db.Where("pool_id = ?", poolID).First(&pool).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Pool not found")
		return
	}

	if err := parseForm(r); err != nil {
		response.RespondError(w, http.StatusBadRequest, "Failed to parse form")
		return
	}

	// Update name if provided
	if name := r.Form.Get("name"); name != "" {
		pool.Name = name
	}
	// Update description if provided
	if desc := r.Form.Get("description"); desc != "" {
		pool.Description = desc
	}
	h.db.Save(&pool)

	// Add IPs
	if addIP := r.Form.Get("add_ip"); addIP != "" {
		h.db.Create(&IPPoolIP{
			PoolID:    poolID,
			IPAddress: addIP,
		})
	}

	// Remove IPs
	if removeIP := r.Form.Get("remove_ip"); removeIP != "" {
		h.db.Unscoped().Where("pool_id = ? AND ip_address = ?", poolID, removeIP).Delete(&IPPoolIP{})
	}

	response.RespondSuccess(w, "success")
}

// DeletePool handles DELETE /v1/ip_pools/{pool_id}
func (h *Handlers) DeletePool(w http.ResponseWriter, r *http.Request) {
	poolID := chi.URLParam(r, "pool_id")

	var pool IPPool
	if err := h.db.Where("pool_id = ?", poolID).First(&pool).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Pool not found")
		return
	}

	// Delete associated IPPoolIP records
	h.db.Unscoped().Where("pool_id = ?", poolID).Delete(&IPPoolIP{})

	// Delete the pool itself
	h.db.Unscoped().Delete(&pool)

	response.RespondSuccess(w, "started")
}
