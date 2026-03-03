package metrics

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bethmaloney/mailgun-mock-api/internal/response"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Read-only view of the events table (avoids importing event package)
// ---------------------------------------------------------------------------

// eventRecord is a read-only view of the events table for metrics queries.
type eventRecord struct {
	DomainName string
	EventType  string
	Timestamp  float64
	Tags       string // JSON array
	Severity   string
	Reason     string
}

func (eventRecord) TableName() string { return "events" }

// ---------------------------------------------------------------------------
// Constants and validation maps
// ---------------------------------------------------------------------------

const rfc2822 = "Mon, 02 Jan 2006 15:04:05 MST"

var validEventTypes = map[string]bool{
	"accepted": true, "delivered": true, "failed": true, "stored": true,
	"opened": true, "clicked": true, "unsubscribed": true, "complained": true,
}

var validResolutions = map[string]bool{
	"hour": true, "day": true, "month": true,
}

// ---------------------------------------------------------------------------
// Time parsing helpers (copied from tag/stats.go to avoid circular imports)
// ---------------------------------------------------------------------------

func parseStatsTimestamp(s string) (time.Time, error) {
	if ts, err := strconv.ParseFloat(s, 64); err == nil {
		sec := int64(ts)
		nsec := int64((ts - float64(sec)) * 1e9)
		return time.Unix(sec, nsec).UTC(), nil
	}
	if t, err := time.Parse(time.RFC1123Z, s); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse(time.RFC1123, s); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse(rfc2822, s); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("unrecognized time format: %q", s)
}

func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	unit := s[len(s)-1]
	numStr := s[:len(s)-1]
	num, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, fmt.Errorf("invalid duration number: %v", err)
	}
	switch unit {
	case 'h':
		return time.Duration(num) * time.Hour, nil
	case 'd':
		return time.Duration(num) * 24 * time.Hour, nil
	case 'm':
		return time.Duration(num) * 30 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unknown duration unit: %c", unit)
	}
}

// ---------------------------------------------------------------------------
// Time bucket helpers (copied from tag/stats.go)
// ---------------------------------------------------------------------------

func truncateTime(t time.Time, resolution string) time.Time {
	t = t.UTC()
	switch resolution {
	case "hour":
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, time.UTC)
	case "month":
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	default:
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	}
}

func nextBucket(t time.Time, resolution string) time.Time {
	switch resolution {
	case "hour":
		return t.Add(time.Hour)
	case "month":
		return t.AddDate(0, 1, 0)
	default:
		return t.AddDate(0, 0, 1)
	}
}

func generateBuckets(start, end time.Time, resolution string) []time.Time {
	start = truncateTime(start, resolution)
	endBucket := truncateTime(end, resolution)
	var buckets []time.Time
	for t := start; !t.After(endBucket); t = nextBucket(t, resolution) {
		buckets = append(buckets, t)
	}
	return buckets
}

// ---------------------------------------------------------------------------
// Stats bucket data structures (copied from tag/stats.go)
// ---------------------------------------------------------------------------

type acceptedStats struct {
	Incoming int `json:"incoming"`
	Outgoing int `json:"outgoing"`
	Total    int `json:"total"`
}

type deliveredStats struct {
	SMTP  int `json:"smtp"`
	HTTP  int `json:"http"`
	Total int `json:"total"`
}

type failedTemporary struct {
	ESPBlock int `json:"espblock"`
}

type failedPermanent struct {
	SuppressBounce      int `json:"suppress-bounce"`
	SuppressUnsubscribe int `json:"suppress-unsubscribe"`
	SuppressComplaint   int `json:"suppress-complaint"`
	Bounce              int `json:"bounce"`
	DelayedBounce       int `json:"delayed-bounce"`
	Total               int `json:"total"`
}

type failedStats struct {
	Temporary failedTemporary `json:"temporary"`
	Permanent failedPermanent `json:"permanent"`
}

type simpleTotalStats struct {
	Total int `json:"total"`
}

type statsBucket struct {
	Time         string           `json:"time"`
	Accepted     acceptedStats    `json:"accepted"`
	Delivered    deliveredStats   `json:"delivered"`
	Failed       failedStats      `json:"failed"`
	Stored       simpleTotalStats `json:"stored"`
	Opened       simpleTotalStats `json:"opened"`
	Clicked      simpleTotalStats `json:"clicked"`
	Unsubscribed simpleTotalStats `json:"unsubscribed"`
	Complained   simpleTotalStats `json:"complained"`
}

func newStatsBucket(t time.Time) statsBucket {
	return statsBucket{
		Time: t.UTC().Format(rfc2822),
	}
}

func (b *statsBucket) addEvent(ev eventRecord) {
	switch ev.EventType {
	case "accepted":
		b.Accepted.Outgoing++
		b.Accepted.Total++
	case "delivered":
		b.Delivered.HTTP++
		b.Delivered.Total++
	case "failed":
		if ev.Severity == "temporary" {
			b.Failed.Temporary.ESPBlock++
		} else {
			switch ev.Reason {
			case "suppress-bounce":
				b.Failed.Permanent.SuppressBounce++
			case "suppress-unsubscribe":
				b.Failed.Permanent.SuppressUnsubscribe++
			case "suppress-complaint":
				b.Failed.Permanent.SuppressComplaint++
			case "bounce":
				b.Failed.Permanent.Bounce++
			case "delayed-bounce":
				b.Failed.Permanent.DelayedBounce++
			default:
				b.Failed.Permanent.Bounce++
			}
			b.Failed.Permanent.Total++
		}
	case "stored":
		b.Stored.Total++
	case "opened":
		b.Opened.Total++
	case "clicked":
		b.Clicked.Total++
	case "unsubscribed":
		b.Unsubscribed.Total++
	case "complained":
		b.Complained.Total++
	}
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// Handlers holds the database connection for metrics endpoints.
type Handlers struct {
	db *gorm.DB
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(db *gorm.DB) *Handlers {
	return &Handlers{db: db}
}

// ---------------------------------------------------------------------------
// Shared: account-wide stats query (no domain filter)
// ---------------------------------------------------------------------------

func (h *Handlers) queryAccountStats(r *http.Request, eventTypes []string) ([]statsBucket, time.Time, time.Time, string, error) {
	q := r.URL.Query()

	resolution := q.Get("resolution")
	if resolution == "" {
		resolution = "day"
	}
	if !validResolutions[resolution] {
		return nil, time.Time{}, time.Time{}, "", fmt.Errorf("invalid resolution")
	}

	now := time.Now().UTC()
	start := now.Add(-7 * 24 * time.Hour)
	end := now

	if v := q.Get("duration"); v != "" {
		dur, err := parseDuration(v)
		if err != nil {
			return nil, time.Time{}, time.Time{}, "", fmt.Errorf("invalid duration: %v", err)
		}
		start = end.Add(-dur)
	} else if v := q.Get("start"); v != "" {
		t, err := parseStatsTimestamp(v)
		if err != nil {
			return nil, time.Time{}, time.Time{}, "", fmt.Errorf("invalid start: %v", err)
		}
		start = t
	}

	if v := q.Get("end"); v != "" {
		t, err := parseStatsTimestamp(v)
		if err != nil {
			return nil, time.Time{}, time.Time{}, "", fmt.Errorf("invalid end: %v", err)
		}
		end = t
	}

	buckets := generateBuckets(start, end, resolution)
	bucketMap := make(map[string]int)
	results := make([]statsBucket, len(buckets))
	for i, bt := range buckets {
		results[i] = newStatsBucket(bt)
		bucketMap[bt.UTC().Format(rfc2822)] = i
	}

	startTS := float64(start.UnixMicro()) / 1e6
	endTS := float64(end.UnixMicro()) / 1e6

	query := h.db.Model(&eventRecord{}).
		Where("event_type IN ?", eventTypes).
		Where("timestamp >= ? AND timestamp <= ?", startTS, endTS)

	var events []eventRecord
	query.Find(&events)

	for _, ev := range events {
		evTime := time.Unix(int64(ev.Timestamp), int64((ev.Timestamp-float64(int64(ev.Timestamp)))*1e9)).UTC()
		bucketTime := truncateTime(evTime, resolution)
		key := bucketTime.Format(rfc2822)
		if idx, ok := bucketMap[key]; ok {
			results[idx].addEvent(ev)
		}
	}

	return results, start, end, resolution, nil
}

// ---------------------------------------------------------------------------
// Shared: validate event params for v3 stats endpoints
// ---------------------------------------------------------------------------

func validateEventParams(r *http.Request) ([]string, error) {
	eventTypes := r.URL.Query()["event"]
	if len(eventTypes) == 0 {
		return nil, fmt.Errorf("event is required")
	}
	for _, et := range eventTypes {
		if !validEventTypes[et] {
			return nil, fmt.Errorf("invalid event type")
		}
	}
	return eventTypes, nil
}

// ---------------------------------------------------------------------------
// GET /v3/stats/total -- Account-wide aggregated stats
// ---------------------------------------------------------------------------

// GetAccountStats handles GET /v3/stats/total.
func (h *Handlers) GetAccountStats(w http.ResponseWriter, r *http.Request) {
	eventTypes, err := validateEventParams(r)
	if err != nil {
		response.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	stats, start, end, resolution, err := h.queryAccountStats(r, eventTypes)
	if err != nil {
		response.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"start":      start.UTC().Format(rfc2822),
		"end":        end.UTC().Format(rfc2822),
		"resolution": resolution,
		"stats":      stats,
	})
}

// ---------------------------------------------------------------------------
// GET /v3/stats/filter -- Filtered/grouped account stats
// ---------------------------------------------------------------------------

// GetFilteredStats handles GET /v3/stats/filter.
func (h *Handlers) GetFilteredStats(w http.ResponseWriter, r *http.Request) {
	// Same as GetAccountStats -- filter and group params are accepted but ignored
	eventTypes, err := validateEventParams(r)
	if err != nil {
		response.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	stats, start, end, resolution, err := h.queryAccountStats(r, eventTypes)
	if err != nil {
		response.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"start":      start.UTC().Format(rfc2822),
		"end":        end.UTC().Format(rfc2822),
		"resolution": resolution,
		"stats":      stats,
	})
}

// ---------------------------------------------------------------------------
// GET /v3/stats/total/domains -- Per-domain stats snapshot
// ---------------------------------------------------------------------------

// GetDomainStatsSnapshot handles GET /v3/stats/total/domains.
func (h *Handlers) GetDomainStatsSnapshot(w http.ResponseWriter, r *http.Request) {
	eventTypes, err := validateEventParams(r)
	if err != nil {
		response.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Use the same account-wide query logic
	stats, start, end, resolution, err := h.queryAccountStats(r, eventTypes)
	if err != nil {
		response.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"start":      start.UTC().Format(rfc2822),
		"end":        end.UTC().Format(rfc2822),
		"resolution": resolution,
		"stats":      stats,
	})
}

// ---------------------------------------------------------------------------
// GET /v3/{domain_name}/aggregates/providers -- Per-provider breakdown
// ---------------------------------------------------------------------------

// GetDomainAggregateProviders handles GET /v3/{domain_name}/aggregates/providers.
func (h *Handlers) GetDomainAggregateProviders(w http.ResponseWriter, r *http.Request) {
	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"items": map[string]interface{}{},
	})
}

// ---------------------------------------------------------------------------
// GET /v3/{domain_name}/aggregates/devices -- Per-device breakdown
// ---------------------------------------------------------------------------

// GetDomainAggregateDevices handles GET /v3/{domain_name}/aggregates/devices.
func (h *Handlers) GetDomainAggregateDevices(w http.ResponseWriter, r *http.Request) {
	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"items": map[string]interface{}{},
	})
}

// ---------------------------------------------------------------------------
// GET /v3/{domain_name}/aggregates/countries -- Per-country breakdown
// ---------------------------------------------------------------------------

// GetDomainAggregateCountries handles GET /v3/{domain_name}/aggregates/countries.
func (h *Handlers) GetDomainAggregateCountries(w http.ResponseWriter, r *http.Request) {
	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"items": map[string]interface{}{},
	})
}

// ---------------------------------------------------------------------------
// POST /v1/analytics/metrics -- Multi-dimensional query
// ---------------------------------------------------------------------------

// metricsRequest represents the JSON body for POST /v1/analytics/metrics.
type metricsRequest struct {
	Start             string                 `json:"start"`
	End               string                 `json:"end"`
	Resolution        string                 `json:"resolution"`
	Duration          string                 `json:"duration"`
	Dimensions        []string               `json:"dimensions"`
	Metrics           []string               `json:"metrics"`
	Filter            map[string]interface{} `json:"filter"`
	IncludeSubaccts   bool                   `json:"include_subaccounts"`
	IncludeAggregates bool                   `json:"include_aggregates"`
	Pagination        *paginationRequest     `json:"pagination"`
}

type paginationRequest struct {
	Sort  string `json:"sort"`
	Skip  int    `json:"skip"`
	Limit int    `json:"limit"`
}

type dimensionValue struct {
	Dimension    string `json:"dimension"`
	Value        string `json:"value"`
	DisplayValue string `json:"display_value"`
}

type metricsItem struct {
	Dimensions []dimensionValue       `json:"dimensions"`
	Metrics    map[string]interface{} `json:"metrics"`
}

type paginationResponse struct {
	Sort  string `json:"sort"`
	Skip  int    `json:"skip"`
	Limit int    `json:"limit"`
	Total int    `json:"total"`
}

// countMetricMapping maps metric names to their event-type based counting logic.
// Returns the event count from a slice of events for a given metric name.
func countMetric(metricName string, events []eventRecord) float64 {
	var count float64
	for _, ev := range events {
		switch metricName {
		case "accepted_count":
			if ev.EventType == "accepted" {
				count++
			}
		case "delivered_count":
			if ev.EventType == "delivered" {
				count++
			}
		case "opened_count":
			if ev.EventType == "opened" {
				count++
			}
		case "clicked_count":
			if ev.EventType == "clicked" {
				count++
			}
		case "unsubscribed_count":
			if ev.EventType == "unsubscribed" {
				count++
			}
		case "complained_count":
			if ev.EventType == "complained" {
				count++
			}
		case "stored_count":
			if ev.EventType == "stored" {
				count++
			}
		case "failed_count":
			if ev.EventType == "failed" {
				count++
			}
		case "permanent_failed_count":
			if ev.EventType == "failed" && ev.Severity == "permanent" {
				count++
			}
		case "temporary_failed_count":
			if ev.EventType == "failed" && ev.Severity == "temporary" {
				count++
			}
		case "bounced_count":
			if ev.EventType == "failed" && strings.Contains(ev.Reason, "bounce") {
				count++
			}
		}
	}
	return count
}

func isRateMetric(name string) bool {
	return strings.HasSuffix(name, "_rate")
}

func computeRate(metricName string, events []eventRecord) string {
	var numerator, denominator float64

	switch metricName {
	case "delivered_rate":
		numerator = countMetric("delivered_count", events)
		denominator = countMetric("delivered_count", events) + countMetric("permanent_failed_count", events)
	case "opened_rate":
		numerator = countMetric("opened_count", events)
		denominator = countMetric("delivered_count", events)
	case "clicked_rate":
		numerator = countMetric("clicked_count", events)
		denominator = countMetric("delivered_count", events)
	case "unsubscribed_rate":
		numerator = countMetric("unsubscribed_count", events)
		denominator = countMetric("delivered_count", events)
	case "complained_rate":
		numerator = countMetric("complained_count", events)
		denominator = countMetric("delivered_count", events)
	case "bounced_rate":
		numerator = countMetric("bounced_count", events)
		denominator = countMetric("delivered_count", events)
	default:
		return "0.00"
	}

	if denominator == 0 {
		return "0.00"
	}
	rate := (numerator / denominator) * 100
	return fmt.Sprintf("%.2f", rate)
}

func computeMetricValue(metricName string, events []eventRecord) interface{} {
	if isRateMetric(metricName) {
		return computeRate(metricName, events)
	}
	return countMetric(metricName, events)
}

// extractDomainFilter extracts domain values from the filter object if present.
func extractDomainFilter(filter map[string]interface{}) []string {
	if filter == nil {
		return nil
	}
	andClauses, ok := filter["AND"]
	if !ok {
		return nil
	}

	// andClauses can be []interface{} (from JSON decode)
	clauses, ok := andClauses.([]interface{})
	if !ok {
		return nil
	}

	for _, clause := range clauses {
		clauseMap, ok := clause.(map[string]interface{})
		if !ok {
			continue
		}
		attr, _ := clauseMap["attribute"].(string)
		if attr != "domain" {
			continue
		}
		valuesRaw, ok := clauseMap["values"].([]interface{})
		if !ok {
			continue
		}
		var domains []string
		for _, v := range valuesRaw {
			vm, ok := v.(map[string]interface{})
			if !ok {
				continue
			}
			if val, ok := vm["value"].(string); ok {
				domains = append(domains, val)
			}
		}
		return domains
	}
	return nil
}

// QueryMetrics handles POST /v1/analytics/metrics.
func (h *Handlers) QueryMetrics(w http.ResponseWriter, r *http.Request) {
	var req metricsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.RespondError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Validate dimensions
	if len(req.Dimensions) > 3 {
		response.RespondError(w, http.StatusBadRequest, "too many dimensions")
		return
	}

	// Validate metrics
	if len(req.Metrics) > 10 {
		response.RespondError(w, http.StatusBadRequest, "too many metrics")
		return
	}

	// Parse start/end/duration
	now := time.Now().UTC()
	start := now.Add(-7 * 24 * time.Hour)
	end := now
	resolution := req.Resolution
	if resolution == "" {
		resolution = "day"
	}

	if req.Duration != "" {
		dur, err := parseDuration(req.Duration)
		if err != nil {
			response.RespondError(w, http.StatusBadRequest, "invalid duration")
			return
		}
		if req.End != "" {
			t, err := parseStatsTimestamp(req.End)
			if err != nil {
				response.RespondError(w, http.StatusBadRequest, "invalid end time")
				return
			}
			end = t
		}
		start = end.Add(-dur)
	} else {
		if req.Start != "" {
			t, err := parseStatsTimestamp(req.Start)
			if err != nil {
				response.RespondError(w, http.StatusBadRequest, "invalid start time")
				return
			}
			start = t
		}
		if req.End != "" {
			t, err := parseStatsTimestamp(req.End)
			if err != nil {
				response.RespondError(w, http.StatusBadRequest, "invalid end time")
				return
			}
			end = t
		}
	}

	// Extract domain filter if present
	domainFilter := extractDomainFilter(req.Filter)

	// Query events — if end has no sub-second precision (from RFC 2822 format),
	// bump it to include the full second so events within that second are captured.
	queryEnd := end
	if queryEnd.Nanosecond() == 0 {
		queryEnd = queryEnd.Add(time.Second - time.Nanosecond)
	}
	startTS := float64(start.UnixMicro()) / 1e6
	endTS := float64(queryEnd.UnixMicro()) / 1e6

	query := h.db.Model(&eventRecord{}).
		Where("timestamp >= ? AND timestamp <= ?", startTS, endTS)

	if len(domainFilter) > 0 {
		query = query.Where("domain_name IN ?", domainFilter)
	}

	var events []eventRecord
	query.Find(&events)

	// Build items based on dimensions
	var items []metricsItem

	hasDimTime := false
	for _, d := range req.Dimensions {
		if d == "time" {
			hasDimTime = true
			break
		}
	}

	if hasDimTime {
		// Group events by time buckets
		buckets := generateBuckets(start, end, resolution)

		for _, bt := range buckets {
			bucketStart := bt
			bucketEnd := nextBucket(bt, resolution)

			// Collect events in this bucket
			var bucketEvents []eventRecord
			for _, ev := range events {
				evTime := time.Unix(int64(ev.Timestamp), int64((ev.Timestamp-float64(int64(ev.Timestamp)))*1e9)).UTC()
				evBucket := truncateTime(evTime, resolution)
				if evBucket.Equal(bucketStart) && evTime.Before(bucketEnd) {
					bucketEvents = append(bucketEvents, ev)
				}
			}

			metricsMap := make(map[string]interface{})
			for _, m := range req.Metrics {
				metricsMap[m] = computeMetricValue(m, bucketEvents)
			}

			item := metricsItem{
				Dimensions: []dimensionValue{
					{
						Dimension:    "time",
						Value:        bt.UTC().Format(rfc2822),
						DisplayValue: bt.UTC().Format(rfc2822),
					},
				},
				Metrics: metricsMap,
			}
			items = append(items, item)
		}
	} else {
		// No time dimension -- single item with all events
		metricsMap := make(map[string]interface{})
		for _, m := range req.Metrics {
			metricsMap[m] = computeMetricValue(m, events)
		}
		items = append(items, metricsItem{
			Dimensions: []dimensionValue{},
			Metrics:    metricsMap,
		})
	}

	// Total before pagination
	total := len(items)

	// Apply pagination
	pag := paginationResponse{
		Sort:  "time:asc",
		Skip:  0,
		Limit: total,
		Total: total,
	}
	if req.Pagination != nil {
		if req.Pagination.Sort != "" {
			pag.Sort = req.Pagination.Sort
		}
		pag.Skip = req.Pagination.Skip
		if req.Pagination.Limit > 0 {
			pag.Limit = req.Pagination.Limit
		}

		// Apply skip
		if pag.Skip < len(items) {
			items = items[pag.Skip:]
		} else {
			items = nil
		}

		// Apply limit
		if pag.Limit > 0 && pag.Limit < len(items) {
			items = items[:pag.Limit]
		}
	}

	// Build response
	resp := map[string]interface{}{
		"start":      start.UTC().Format(rfc2822),
		"end":        end.UTC().Format(rfc2822),
		"resolution": resolution,
		"dimensions": req.Dimensions,
		"items":      items,
		"pagination": pag,
	}

	if req.Duration != "" {
		resp["duration"] = req.Duration
	}

	// Include aggregates if requested
	if req.IncludeAggregates {
		aggMetrics := make(map[string]interface{})
		for _, m := range req.Metrics {
			aggMetrics[m] = computeMetricValue(m, events)
		}
		resp["aggregates"] = map[string]interface{}{
			"metrics": aggMetrics,
		}
	}

	response.RespondJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// POST /v1/analytics/usage/metrics -- Usage data (stub)
// ---------------------------------------------------------------------------

// QueryUsageMetrics handles POST /v1/analytics/usage/metrics.
func (h *Handlers) QueryUsageMetrics(w http.ResponseWriter, r *http.Request) {
	var req metricsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.RespondError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	now := time.Now().UTC()
	start := now.Add(-7 * 24 * time.Hour)
	end := now
	resolution := req.Resolution
	if resolution == "" {
		resolution = "day"
	}

	if req.Start != "" {
		if t, err := parseStatsTimestamp(req.Start); err == nil {
			start = t
		}
	}
	if req.End != "" {
		if t, err := parseStatsTimestamp(req.End); err == nil {
			end = t
		}
	}

	resp := map[string]interface{}{
		"start":      start.UTC().Format(rfc2822),
		"end":        end.UTC().Format(rfc2822),
		"resolution": resolution,
		"dimensions": req.Dimensions,
		"items":      []interface{}{},
	}

	// Include pagination if requested
	if req.Pagination != nil {
		resp["pagination"] = paginationResponse{
			Sort:  req.Pagination.Sort,
			Skip:  req.Pagination.Skip,
			Limit: req.Pagination.Limit,
			Total: 0,
		}
	}

	response.RespondJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// POST /v2/bounce-classification/metrics -- Bounce breakdown (stub)
// ---------------------------------------------------------------------------

// QueryBounceClassification handles POST /v2/bounce-classification/metrics.
func (h *Handlers) QueryBounceClassification(w http.ResponseWriter, r *http.Request) {
	var req metricsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.RespondError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	now := time.Now().UTC()
	start := now.Add(-7 * 24 * time.Hour)
	end := now
	resolution := req.Resolution
	if resolution == "" {
		resolution = "day"
	}

	if req.Start != "" {
		if t, err := parseStatsTimestamp(req.Start); err == nil {
			start = t
		}
	}
	if req.End != "" {
		if t, err := parseStatsTimestamp(req.End); err == nil {
			end = t
		}
	}

	resp := map[string]interface{}{
		"start":      start.UTC().Format(rfc2822),
		"end":        end.UTC().Format(rfc2822),
		"resolution": resolution,
		"dimensions": req.Dimensions,
		"items":      []interface{}{},
	}

	// Include pagination if requested
	if req.Pagination != nil {
		resp["pagination"] = paginationResponse{
			Sort:  req.Pagination.Sort,
			Skip:  req.Pagination.Skip,
			Limit: req.Pagination.Limit,
			Total: 0,
		}
	}

	response.RespondJSON(w, http.StatusOK, resp)
}
