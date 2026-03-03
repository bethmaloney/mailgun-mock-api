package tag

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bethmaloney/mailgun-mock-api/internal/response"
	"github.com/go-chi/chi/v5"
)

// ---------------------------------------------------------------------------
// Read-only view of the events table (avoids importing event package)
// ---------------------------------------------------------------------------

// eventRecord is a read-only view of the events table for stats queries.
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
// Time format constant
// ---------------------------------------------------------------------------

const rfc2822 = "Mon, 02 Jan 2006 15:04:05 MST"

// validEventTypes is the set of event types accepted by the stats endpoints.
var validEventTypes = map[string]bool{
	"accepted": true, "delivered": true, "failed": true, "stored": true,
	"opened": true, "clicked": true, "unsubscribed": true, "complained": true,
}

// validResolutions is the set of resolution values accepted by the stats endpoints.
var validResolutions = map[string]bool{
	"hour": true, "day": true, "month": true,
}

// ---------------------------------------------------------------------------
// Time parsing helpers
// ---------------------------------------------------------------------------

// parseStatsTimestamp parses a time string from a query parameter.
// It supports RFC 2822 / RFC 1123 / RFC 1123Z formats and Unix epoch floats.
func parseStatsTimestamp(s string) (time.Time, error) {
	// Try Unix epoch float first
	if ts, err := strconv.ParseFloat(s, 64); err == nil {
		sec := int64(ts)
		nsec := int64((ts - float64(sec)) * 1e9)
		return time.Unix(sec, nsec).UTC(), nil
	}

	// Try RFC 1123Z (e.g. "Mon, 02 Jan 2006 15:04:05 -0700")
	if t, err := time.Parse(time.RFC1123Z, s); err == nil {
		return t.UTC(), nil
	}

	// Try RFC 1123 (e.g. "Mon, 02 Jan 2006 15:04:05 MST")
	if t, err := time.Parse(time.RFC1123, s); err == nil {
		return t.UTC(), nil
	}

	// Try the exact rfc2822 format used in the spec
	if t, err := time.Parse(rfc2822, s); err == nil {
		return t.UTC(), nil
	}

	return time.Time{}, fmt.Errorf("unrecognized time format: %q", s)
}

// parseDuration parses a Mailgun-style duration string like "1m", "7d", "24h".
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
		return time.Duration(num) * 30 * 24 * time.Hour, nil // approximate month
	default:
		return 0, fmt.Errorf("unknown duration unit: %c", unit)
	}
}

// ---------------------------------------------------------------------------
// Time bucket helpers
// ---------------------------------------------------------------------------

// truncateTime truncates a time to the given resolution boundary.
func truncateTime(t time.Time, resolution string) time.Time {
	t = t.UTC()
	switch resolution {
	case "hour":
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, time.UTC)
	case "month":
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	default: // "day"
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	}
}

// nextBucket advances a time bucket by one resolution step.
func nextBucket(t time.Time, resolution string) time.Time {
	switch resolution {
	case "hour":
		return t.Add(time.Hour)
	case "month":
		return t.AddDate(0, 1, 0)
	default: // "day"
		return t.AddDate(0, 0, 1)
	}
}

// generateBuckets creates time bucket start times from start to end (inclusive
// of any bucket that start falls into, up to and including end's bucket).
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
// Stats bucket data structures
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

// newStatsBucket creates an empty stats bucket for a given time.
func newStatsBucket(t time.Time) statsBucket {
	return statsBucket{
		Time: t.UTC().Format(rfc2822),
	}
}

// addEvent increments the appropriate counters for an event.
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
			// permanent
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
// Core stats query logic
// ---------------------------------------------------------------------------

// queryStats performs the common stats query logic shared by tag stats and
// domain stats. If tagFilter is non-empty, events are filtered to those
// containing the tag name in their JSON tags field.
func (h *Handlers) queryStats(r *http.Request, domainName string, tagFilter string, eventTypes []string) ([]statsBucket, time.Time, time.Time, string, error) {
	q := r.URL.Query()

	// Parse resolution (default: "day")
	resolution := q.Get("resolution")
	if resolution == "" {
		resolution = "day"
	}
	if !validResolutions[resolution] {
		return nil, time.Time{}, time.Time{}, "", fmt.Errorf("invalid resolution")
	}

	// Parse start/end/duration
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

	// Build time buckets
	buckets := generateBuckets(start, end, resolution)
	bucketMap := make(map[string]int) // time string -> index in results
	results := make([]statsBucket, len(buckets))
	for i, bt := range buckets {
		results[i] = newStatsBucket(bt)
		bucketMap[bt.UTC().Format(rfc2822)] = i
	}

	// Query events — use microsecond precision to match event timestamps
	startTS := float64(start.UnixMicro()) / 1e6
	endTS := float64(end.UnixMicro()) / 1e6

	query := h.db.Model(&eventRecord{}).
		Where("domain_name = ?", domainName).
		Where("event_type IN ?", eventTypes).
		Where("timestamp >= ? AND timestamp <= ?", startTS, endTS)

	if tagFilter != "" {
		query = query.Where("tags LIKE ?", fmt.Sprintf("%%\"%s\"%%", tagFilter))
	}

	var events []eventRecord
	query.Find(&events)

	// Distribute events into buckets
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
// Handler: GetTagStats — GET /v3/{domain_name}/tags/{tag}/stats
// ---------------------------------------------------------------------------

func (h *Handlers) GetTagStats(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	tagName := strings.ToLower(chi.URLParam(r, "tag"))

	// Validate event param
	eventTypes := r.URL.Query()["event"]
	if len(eventTypes) == 0 {
		response.RespondError(w, http.StatusBadRequest, "event is required")
		return
	}
	for _, et := range eventTypes {
		if !validEventTypes[et] {
			response.RespondError(w, http.StatusBadRequest, "invalid event type")
			return
		}
	}

	// Look up the tag
	var t Tag
	if err := h.db.Where("domain_name = ? AND tag = ?", domainName, tagName).First(&t).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "tag not found")
		return
	}

	stats, start, end, resolution, err := h.queryStats(r, domainName, tagName, eventTypes)
	if err != nil {
		response.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"tag":         t.Tag,
		"description": t.Description,
		"start":       start.UTC().Format(rfc2822),
		"end":         end.UTC().Format(rfc2822),
		"resolution":  resolution,
		"stats":       stats,
	})
}

// ---------------------------------------------------------------------------
// Handler: GetDomainStats — GET /v3/{domain_name}/stats/total
// ---------------------------------------------------------------------------

func (h *Handlers) GetDomainStats(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")

	// Validate event param
	eventTypes := r.URL.Query()["event"]
	if len(eventTypes) == 0 {
		response.RespondError(w, http.StatusBadRequest, "event is required")
		return
	}
	for _, et := range eventTypes {
		if !validEventTypes[et] {
			response.RespondError(w, http.StatusBadRequest, "invalid event type")
			return
		}
	}

	stats, start, end, resolution, err := h.queryStats(r, domainName, "", eventTypes)
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
// Handler: GetTagStatsCountries — GET /v3/{domain_name}/tags/{tag}/stats/aggregates/countries
// ---------------------------------------------------------------------------

func (h *Handlers) GetTagStatsCountries(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	tagName := strings.ToLower(chi.URLParam(r, "tag"))

	var t Tag
	if err := h.db.Where("domain_name = ? AND tag = ?", domainName, tagName).First(&t).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "tag not found")
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"tag":       t.Tag,
		"countries": map[string]interface{}{},
	})
}

// ---------------------------------------------------------------------------
// Handler: GetTagStatsProviders — GET /v3/{domain_name}/tags/{tag}/stats/aggregates/providers
// ---------------------------------------------------------------------------

func (h *Handlers) GetTagStatsProviders(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	tagName := strings.ToLower(chi.URLParam(r, "tag"))

	var t Tag
	if err := h.db.Where("domain_name = ? AND tag = ?", domainName, tagName).First(&t).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "tag not found")
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"tag":       t.Tag,
		"providers": map[string]interface{}{},
	})
}

// ---------------------------------------------------------------------------
// Handler: GetTagStatsDevices — GET /v3/{domain_name}/tags/{tag}/stats/aggregates/devices
// ---------------------------------------------------------------------------

func (h *Handlers) GetTagStatsDevices(w http.ResponseWriter, r *http.Request) {
	domainName := chi.URLParam(r, "domain_name")
	tagName := strings.ToLower(chi.URLParam(r, "tag"))

	var t Tag
	if err := h.db.Where("domain_name = ? AND tag = ?", domainName, tagName).First(&t).Error; err != nil {
		response.RespondError(w, http.StatusNotFound, "tag not found")
		return
	}

	response.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"tag":     t.Tag,
		"devices": map[string]interface{}{},
	})
}
