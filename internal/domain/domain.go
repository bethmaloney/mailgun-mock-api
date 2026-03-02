package domain

import (
	"fmt"
	"net/http"
	"time"

	"github.com/bethmaloney/mailgun-mock-api/internal/database"
	"github.com/bethmaloney/mailgun-mock-api/internal/mock"
	"github.com/bethmaloney/mailgun-mock-api/internal/pagination"
	"github.com/bethmaloney/mailgun-mock-api/internal/request"
	"github.com/bethmaloney/mailgun-mock-api/internal/response"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// GORM Models
// ---------------------------------------------------------------------------

// Domain represents a Mailgun domain stored in the database.
type Domain struct {
	database.BaseModel
	Name                       string  `gorm:"uniqueIndex" json:"name"`
	State                      string  `json:"state"`
	Type                       string  `json:"type"`
	SMTPLogin                  string  `json:"smtp_login"`
	SMTPPassword               *string `json:"smtp_password,omitempty"`
	SpamAction                 string  `json:"spam_action"`
	Wildcard                   bool    `json:"wildcard"`
	RequireTLS                 bool    `json:"require_tls"`
	SkipVerification           bool    `json:"skip_verification"`
	IsDisabled                 bool    `json:"is_disabled"`
	WebPrefix                  string  `json:"web_prefix"`
	WebScheme                  string  `json:"web_scheme"`
	UseAutomaticSenderSecurity bool    `json:"use_automatic_sender_security"`
	MessageTTL                 int     `json:"message_ttl"`
	DKIMSelector               string  `json:"-"`
	DKIMAuthoritySelf          bool    `json:"-"`
	SubaccountID               *string `json:"subaccount_id,omitempty"`

	// Tracking settings
	TrackingOpenActive            bool   `json:"-"`
	TrackingOpenPlaceAtTop        bool   `json:"-"`
	TrackingClickActive           string `json:"-"` // "true", "false", or "htmlonly"
	TrackingUnsubscribeActive     bool   `json:"-"`
	TrackingUnsubscribeHTMLFooter string `json:"-"`
	TrackingUnsubscribeTextFooter string `json:"-"`
}

// TrackingSetting is a GORM model used for migration compatibility.
// Tracking data is stored directly on the Domain model.
type TrackingSetting struct {
	database.BaseModel
	DomainID string `gorm:"index"`
}

// DNSRecord represents a DNS record associated with a domain.
type DNSRecord struct {
	database.BaseModel
	DomainID   string   `gorm:"index" json:"-"`
	Category   string   `json:"-"`
	RecordType string   `json:"record_type"`
	Name       string   `json:"name"`
	Value      string   `json:"value"`
	Priority   *string  `json:"priority"`
	Valid      string   `json:"valid"`
	IsActive   bool     `json:"is_active"`
	Cached     []string `gorm:"-" json:"cached"`
}

// ---------------------------------------------------------------------------
// Response DTOs
// ---------------------------------------------------------------------------

type domainResponse struct {
	ID                         string  `json:"id"`
	Name                       string  `json:"name"`
	State                      string  `json:"state"`
	Type                       string  `json:"type"`
	CreatedAt                  string  `json:"created_at"`
	SMTPLogin                  string  `json:"smtp_login"`
	SMTPPassword               *string `json:"smtp_password,omitempty"`
	SpamAction                 string  `json:"spam_action"`
	Wildcard                   bool    `json:"wildcard"`
	RequireTLS                 bool    `json:"require_tls"`
	SkipVerification           bool    `json:"skip_verification"`
	IsDisabled                 bool    `json:"is_disabled"`
	WebPrefix                  string  `json:"web_prefix"`
	WebScheme                  string  `json:"web_scheme"`
	UseAutomaticSenderSecurity bool    `json:"use_automatic_sender_security"`
	MessageTTL                 int     `json:"message_ttl"`
}

type dnsRecordResponse struct {
	RecordType string   `json:"record_type"`
	Name       string   `json:"name"`
	Value      string   `json:"value"`
	Priority   *string  `json:"priority"`
	Valid      string   `json:"valid"`
	IsActive   bool     `json:"is_active"`
	Cached     []string `json:"cached"`
}

type createDomainResponseDTO struct {
	Message             string              `json:"message"`
	Domain              domainResponse      `json:"domain"`
	ReceivingDNSRecords []dnsRecordResponse `json:"receiving_dns_records"`
	SendingDNSRecords   []dnsRecordResponse `json:"sending_dns_records"`
}

type listDomainsResponseDTO struct {
	TotalCount int64            `json:"total_count"`
	Items      []domainResponse `json:"items"`
}

// ---------------------------------------------------------------------------
// Input structs
// ---------------------------------------------------------------------------

type createDomainInput struct {
	Name                       string  `json:"name" form:"name"`
	SMTPPassword               *string `json:"smtp_password" form:"smtp_password"`
	SpamAction                 *string `json:"spam_action" form:"spam_action"`
	Wildcard                   *bool   `json:"wildcard" form:"wildcard"`
	ForceDKIMAuthority         *bool   `json:"force_dkim_authority" form:"force_dkim_authority"`
	DKIMKeySize                *int    `json:"dkim_key_size" form:"dkim_key_size"`
	WebScheme                  *string `json:"web_scheme" form:"web_scheme"`
	WebPrefix                  *string `json:"web_prefix" form:"web_prefix"`
	RequireTLS                 *bool   `json:"require_tls" form:"require_tls"`
	SkipVerification           *bool   `json:"skip_verification" form:"skip_verification"`
	UseAutomaticSenderSecurity *bool   `json:"use_automatic_sender_security" form:"use_automatic_sender_security"`
	MessageTTL                 *int    `json:"message_ttl" form:"message_ttl"`
}

type updateDomainInput struct {
	SpamAction                 *string `json:"spam_action" form:"spam_action"`
	Wildcard                   *bool   `json:"wildcard" form:"wildcard"`
	WebScheme                  *string `json:"web_scheme" form:"web_scheme"`
	WebPrefix                  *string `json:"web_prefix" form:"web_prefix"`
	RequireTLS                 *bool   `json:"require_tls" form:"require_tls"`
	SkipVerification           *bool   `json:"skip_verification" form:"skip_verification"`
	UseAutomaticSenderSecurity *bool   `json:"use_automatic_sender_security" form:"use_automatic_sender_security"`
	MessageTTL                 *int    `json:"message_ttl" form:"message_ttl"`
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// Handlers holds the database and config needed for domain endpoints.
type Handlers struct {
	db     *gorm.DB
	config *mock.MockConfig
}

// NewHandlers creates a new Handlers instance. It resets domain-related data
// in the database to ensure a clean state for the mock server.
func NewHandlers(db *gorm.DB, config *mock.MockConfig) *Handlers {
	db.Unscoped().Where("1 = 1").Delete(&TrackingSetting{})
	db.Unscoped().Where("1 = 1").Delete(&DNSRecord{})
	db.Unscoped().Where("1 = 1").Delete(&Domain{})
	return &Handlers{db: db, config: config}
}

// validSpamActions is the set of allowed spam_action values.
var validSpamActions = map[string]bool{
	"disabled": true,
	"tag":      true,
	"block":    true,
}

// ---------------------------------------------------------------------------
// Helper: convert domain + DNS records to response DTO
// ---------------------------------------------------------------------------

func toDomainResponse(d *Domain) domainResponse {
	return domainResponse{
		ID:                         d.ID,
		Name:                       d.Name,
		State:                      d.State,
		Type:                       d.Type,
		CreatedAt:                  d.CreatedAt.UTC().Format(time.RFC1123),
		SMTPLogin:                  d.SMTPLogin,
		SMTPPassword:               d.SMTPPassword,
		SpamAction:                 d.SpamAction,
		Wildcard:                   d.Wildcard,
		RequireTLS:                 d.RequireTLS,
		SkipVerification:           d.SkipVerification,
		IsDisabled:                 d.IsDisabled,
		WebPrefix:                  d.WebPrefix,
		WebScheme:                  d.WebScheme,
		UseAutomaticSenderSecurity: d.UseAutomaticSenderSecurity,
		MessageTTL:                 d.MessageTTL,
	}
}

func toDNSRecordResponse(rec *DNSRecord) dnsRecordResponse {
	cached := rec.Cached
	if cached == nil {
		cached = []string{}
	}
	return dnsRecordResponse{
		RecordType: rec.RecordType,
		Name:       rec.Name,
		Value:      rec.Value,
		Priority:   rec.Priority,
		Valid:      rec.Valid,
		IsActive:   rec.IsActive,
		Cached:     cached,
	}
}

func toDNSRecordResponses(records []DNSRecord) []dnsRecordResponse {
	result := make([]dnsRecordResponse, len(records))
	for i := range records {
		result[i] = toDNSRecordResponse(&records[i])
	}
	return result
}

func (h *Handlers) buildDomainWithDNSResponse(d *Domain, message string) (createDomainResponseDTO, error) {
	var sendingRecords []DNSRecord
	if err := h.db.Where("domain_id = ? AND category = ?", d.ID, "sending").Find(&sendingRecords).Error; err != nil {
		return createDomainResponseDTO{}, err
	}

	var receivingRecords []DNSRecord
	if err := h.db.Where("domain_id = ? AND category = ?", d.ID, "receiving").Find(&receivingRecords).Error; err != nil {
		return createDomainResponseDTO{}, err
	}

	// Ensure cached is always an empty array
	for i := range sendingRecords {
		if sendingRecords[i].Cached == nil {
			sendingRecords[i].Cached = []string{}
		}
	}
	for i := range receivingRecords {
		if receivingRecords[i].Cached == nil {
			receivingRecords[i].Cached = []string{}
		}
	}

	return createDomainResponseDTO{
		Message:             message,
		Domain:              toDomainResponse(d),
		ReceivingDNSRecords: toDNSRecordResponses(receivingRecords),
		SendingDNSRecords:   toDNSRecordResponses(sendingRecords),
	}, nil
}

// generateDNSRecords creates the standard DNS records for a domain.
func (h *Handlers) generateDNSRecords(domainID, domainName, webPrefix string) []DNSRecord {
	autoVerify := h.config.DomainBehavior.DomainAutoVerify
	valid := "unknown"
	isActive := false
	if autoVerify {
		valid = "valid"
		isActive = true
	}

	priority10 := "10"

	records := []DNSRecord{
		// Sending records
		{
			DomainID:   domainID,
			Category:   "sending",
			RecordType: "TXT",
			Name:       domainName,
			Value:      "v=spf1 include:mailgun.org ~all",
			Valid:      valid,
			IsActive:   isActive,
			Cached:     []string{},
		},
		{
			DomainID:   domainID,
			Category:   "sending",
			RecordType: "TXT",
			Name:       fmt.Sprintf("pic._domainkey.%s", domainName),
			Value:      "k=rsa; p=MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQC1MBk6MRCfICBPRWBYkfuIbMSqJm0CkSLePOICAiEhpet0VtJqRNea1WaUx8G1sPOFRz7KaxZmJClKfnEPREgdMTV/JhvRqJ8KAFt0kXeNa1sCpjSFLcIRATH9SMGZXO+gYpUNmmii2EYezHqpWHPYIElPWbnfJCmIhfe0qMDrwIDAQAB",
			Valid:      valid,
			IsActive:   isActive,
			Cached:     []string{},
		},
		{
			DomainID:   domainID,
			Category:   "sending",
			RecordType: "CNAME",
			Name:       fmt.Sprintf("%s.%s", webPrefix, domainName),
			Value:      "mailgun.org",
			Valid:      valid,
			IsActive:   isActive,
			Cached:     []string{},
		},
		// Receiving records
		{
			DomainID:   domainID,
			Category:   "receiving",
			RecordType: "MX",
			Name:       domainName,
			Value:      "mxa.mailgun.org",
			Priority:   &priority10,
			Valid:      valid,
			IsActive:   isActive,
			Cached:     []string{},
		},
		{
			DomainID:   domainID,
			Category:   "receiving",
			RecordType: "MX",
			Name:       domainName,
			Value:      "mxb.mailgun.org",
			Priority:   &priority10,
			Valid:      valid,
			IsActive:   isActive,
			Cached:     []string{},
		},
	}

	return records
}

// ---------------------------------------------------------------------------
// CreateDomain handles POST /v4/domains.
// ---------------------------------------------------------------------------

func (h *Handlers) CreateDomain(w http.ResponseWriter, r *http.Request) {
	var input createDomainInput
	if err := request.Parse(r, &input); err != nil {
		response.RespondError(w, http.StatusBadRequest, fmt.Sprintf("Failed to parse request: %v", err))
		return
	}

	if input.Name == "" {
		response.RespondError(w, http.StatusBadRequest, "name is required")
		return
	}

	// Default values
	spamAction := "disabled"
	if input.SpamAction != nil {
		spamAction = *input.SpamAction
	}

	if !validSpamActions[spamAction] {
		response.RespondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid spam_action: %s", spamAction))
		return
	}

	wildcard := false
	if input.Wildcard != nil {
		wildcard = *input.Wildcard
	}

	webScheme := "https"
	if input.WebScheme != nil {
		webScheme = *input.WebScheme
	}

	webPrefix := "email"
	if input.WebPrefix != nil {
		webPrefix = *input.WebPrefix
	}

	requireTLS := false
	if input.RequireTLS != nil {
		requireTLS = *input.RequireTLS
	}

	skipVerification := false
	if input.SkipVerification != nil {
		skipVerification = *input.SkipVerification
	}

	useAutoSenderSecurity := true
	if input.UseAutomaticSenderSecurity != nil {
		useAutoSenderSecurity = *input.UseAutomaticSenderSecurity
	}

	messageTTL := 259200
	if input.MessageTTL != nil {
		messageTTL = *input.MessageTTL
	}

	dkimAuthSelf := false
	if input.ForceDKIMAuthority != nil {
		dkimAuthSelf = *input.ForceDKIMAuthority
	}

	state := "active"
	if !h.config.DomainBehavior.DomainAutoVerify {
		state = "unverified"
	}

	smtpPassword := input.SMTPPassword
	if smtpPassword == nil {
		generated := uuid.New().String()
		smtpPassword = &generated
	}

	d := Domain{
		Name:                       input.Name,
		State:                      state,
		Type:                       "custom",
		SMTPLogin:                  fmt.Sprintf("postmaster@%s", input.Name),
		SMTPPassword:               smtpPassword,
		SpamAction:                 spamAction,
		Wildcard:                   wildcard,
		RequireTLS:                 requireTLS,
		SkipVerification:           skipVerification,
		IsDisabled:                 false,
		WebPrefix:                  webPrefix,
		WebScheme:                  webScheme,
		UseAutomaticSenderSecurity: useAutoSenderSecurity,
		MessageTTL:                 messageTTL,
		DKIMSelector:               "pic",
		DKIMAuthoritySelf:          dkimAuthSelf,

		// Tracking defaults
		TrackingOpenActive:            true,
		TrackingClickActive:           "true",
		TrackingUnsubscribeActive:     true,
		TrackingUnsubscribeHTMLFooter: "\n<br>\n<p><a href=\"%unsubscribe_url%\">unsubscribe</a></p>\n",
		TrackingUnsubscribeTextFooter: "\n\nTo unsubscribe click: <%unsubscribe_url%>\n\n",
	}

	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&d).Error; err != nil {
			return err
		}
		records := h.generateDNSRecords(d.ID, d.Name, d.WebPrefix)
		if err := tx.Create(&records).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		response.RespondError(w, http.StatusBadRequest, fmt.Sprintf("Domain already exists or could not be created: %v", err))
		return
	}

	resp, err := h.buildDomainWithDNSResponse(&d, "Domain has been created")
	if err != nil {
		response.RespondError(w, http.StatusInternalServerError, "Failed to load DNS records")
		return
	}
	response.RespondJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// ListDomains handles GET /v4/domains.
// ---------------------------------------------------------------------------

func (h *Handlers) ListDomains(w http.ResponseWriter, r *http.Request) {
	params := pagination.ParseSkipLimitParams(r, 100, 1000)

	query := h.db.Model(&Domain{})

	// Filter by state
	if state := r.URL.Query().Get("state"); state != "" {
		query = query.Where("state = ?", state)
	}

	// Filter by search (name substring)
	if search := r.URL.Query().Get("search"); search != "" {
		query = query.Where("name LIKE ?", fmt.Sprintf("%%%s%%", search))
	}

	var totalCount int64
	query.Count(&totalCount)

	var domains []Domain
	query.Offset(params.Skip).Limit(params.Limit).Find(&domains)

	items := make([]domainResponse, len(domains))
	for i := range domains {
		items[i] = toDomainResponse(&domains[i])
		// Don't include smtp_password in list responses
		items[i].SMTPPassword = nil
	}

	resp := listDomainsResponseDTO{
		TotalCount: totalCount,
		Items:      items,
	}

	response.RespondJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// GetDomain handles GET /v4/domains/{name}.
// ---------------------------------------------------------------------------

func (h *Handlers) GetDomain(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	var d Domain
	if err := h.db.Where("name = ?", name).First(&d).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Domain not found")
		return
	}

	// Don't include smtp_password in get responses
	d.SMTPPassword = nil

	resp, err := h.buildDomainWithDNSResponse(&d, "")
	if err != nil {
		response.RespondError(w, http.StatusInternalServerError, "Failed to load DNS records")
		return
	}
	response.RespondJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// UpdateDomain handles PUT /v4/domains/{name}.
// ---------------------------------------------------------------------------

func (h *Handlers) UpdateDomain(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	var d Domain
	if err := h.db.Where("name = ?", name).First(&d).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Domain not found")
		return
	}

	var input updateDomainInput
	if err := request.Parse(r, &input); err != nil {
		response.RespondError(w, http.StatusBadRequest, fmt.Sprintf("Failed to parse request: %v", err))
		return
	}

	if input.SpamAction != nil {
		if !validSpamActions[*input.SpamAction] {
			response.RespondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid spam_action: %s", *input.SpamAction))
			return
		}
		d.SpamAction = *input.SpamAction
	}
	if input.Wildcard != nil {
		d.Wildcard = *input.Wildcard
	}
	if input.WebScheme != nil {
		d.WebScheme = *input.WebScheme
	}
	if input.WebPrefix != nil {
		d.WebPrefix = *input.WebPrefix
	}
	if input.RequireTLS != nil {
		d.RequireTLS = *input.RequireTLS
	}
	if input.SkipVerification != nil {
		d.SkipVerification = *input.SkipVerification
	}
	if input.UseAutomaticSenderSecurity != nil {
		d.UseAutomaticSenderSecurity = *input.UseAutomaticSenderSecurity
	}
	if input.MessageTTL != nil {
		d.MessageTTL = *input.MessageTTL
	}

	if err := h.db.Save(&d).Error; err != nil {
		response.RespondError(w, http.StatusInternalServerError, "Failed to update domain")
		return
	}

	// Don't include smtp_password in update responses
	d.SMTPPassword = nil

	resp, err := h.buildDomainWithDNSResponse(&d, "Domain has been updated")
	if err != nil {
		response.RespondError(w, http.StatusInternalServerError, "Failed to load DNS records")
		return
	}
	response.RespondJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// DeleteDomain handles DELETE /v3/domains/{name}.
// ---------------------------------------------------------------------------

func (h *Handlers) DeleteDomain(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	var d Domain
	if err := h.db.Where("name = ?", name).First(&d).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Domain not found")
		return
	}

	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("domain_id = ?", d.ID).Delete(&DNSRecord{}).Error; err != nil {
			return err
		}
		return tx.Delete(&d).Error
	})
	if err != nil {
		response.RespondError(w, http.StatusInternalServerError, "Failed to delete domain")
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]string{"message": "Domain has been deleted"})
}

// ---------------------------------------------------------------------------
// VerifyDomain handles PUT /v4/domains/{name}/verify.
// ---------------------------------------------------------------------------

func (h *Handlers) VerifyDomain(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	var d Domain
	if err := h.db.Where("name = ?", name).First(&d).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Domain not found")
		return
	}

	// Always set to active and valid on verify
	err := h.db.Transaction(func(tx *gorm.DB) error {
		d.State = "active"
		if err := tx.Save(&d).Error; err != nil {
			return err
		}
		return tx.Model(&DNSRecord{}).Where("domain_id = ?", d.ID).Updates(map[string]interface{}{
			"valid":     "valid",
			"is_active": true,
		}).Error
	})
	if err != nil {
		response.RespondError(w, http.StatusInternalServerError, "Failed to verify domain")
		return
	}

	// Don't include smtp_password in verify responses
	d.SMTPPassword = nil

	resp, err := h.buildDomainWithDNSResponse(&d, "Domain DNS records have been updated")
	if err != nil {
		response.RespondError(w, http.StatusInternalServerError, "Failed to load DNS records")
		return
	}
	response.RespondJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// Connection / DKIM Response DTOs
// ---------------------------------------------------------------------------

type connectionDTO struct {
	RequireTLS       bool `json:"require_tls"`
	SkipVerification bool `json:"skip_verification"`
}

// ---------------------------------------------------------------------------
// GetConnection handles GET /v3/domains/{name}/connection.
// ---------------------------------------------------------------------------

func (h *Handlers) GetConnection(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	var d Domain
	if err := h.db.Where("name = ?", name).First(&d).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Domain not found")
		return
	}

	resp := map[string]connectionDTO{
		"connection": {
			RequireTLS:       d.RequireTLS,
			SkipVerification: d.SkipVerification,
		},
	}

	response.RespondJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// UpdateConnection handles PUT /v3/domains/{name}/connection.
// ---------------------------------------------------------------------------

func (h *Handlers) UpdateConnection(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	var d Domain
	if err := h.db.Where("name = ?", name).First(&d).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Domain not found")
		return
	}

	requireTLS := request.ParseFormValue(r, "require_tls")
	if requireTLS != "" {
		d.RequireTLS = requireTLS == "true"
	}

	skipVerification := request.ParseFormValue(r, "skip_verification")
	if skipVerification != "" {
		d.SkipVerification = skipVerification == "true"
	}

	if err := h.db.Save(&d).Error; err != nil {
		response.RespondError(w, http.StatusInternalServerError, "Failed to update connection settings")
		return
	}

	resp := map[string]interface{}{
		"message": "Domain connection settings have been updated",
		"connection": connectionDTO{
			RequireTLS:       d.RequireTLS,
			SkipVerification: d.SkipVerification,
		},
	}

	response.RespondJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// UpdateDKIMAuthority handles PUT /v3/domains/{name}/dkim_authority.
// ---------------------------------------------------------------------------

func (h *Handlers) UpdateDKIMAuthority(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	var d Domain
	if err := h.db.Where("name = ?", name).First(&d).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Domain not found")
		return
	}

	selfVal := request.ParseFormValue(r, "self")
	newSelf := selfVal == "true"
	changed := newSelf != d.DKIMAuthoritySelf

	d.DKIMAuthoritySelf = newSelf

	if err := h.db.Save(&d).Error; err != nil {
		response.RespondError(w, http.StatusInternalServerError, "Failed to update DKIM authority")
		return
	}

	var sendingRecords []DNSRecord
	h.db.Where("domain_id = ? AND category = ?", d.ID, "sending").Find(&sendingRecords)

	resp := map[string]interface{}{
		"message":             "Domain DKIM authority has been changed",
		"changed":             changed,
		"sending_dns_records": toDNSRecordResponses(sendingRecords),
	}

	response.RespondJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// UpdateDKIMSelector handles PUT /v3/domains/{name}/dkim_selector.
// ---------------------------------------------------------------------------

func (h *Handlers) UpdateDKIMSelector(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	var d Domain
	if err := h.db.Where("name = ?", name).First(&d).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "Domain not found")
		return
	}

	selector := request.ParseFormValue(r, "dkim_selector")
	if selector != "" {
		d.DKIMSelector = selector
	}

	if err := h.db.Save(&d).Error; err != nil {
		response.RespondError(w, http.StatusInternalServerError, "Failed to update DKIM selector")
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]string{
		"message": "Domain DKIM selector has been updated",
	})
}
