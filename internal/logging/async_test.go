package logging

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// recordingHandler is a test slog.Handler that stores received records
// in a slice and optionally sleeps in Handle to simulate a slow sink.
type recordingHandler struct {
	mu      sync.Mutex
	records []string
	delay   time.Duration
}

func (h *recordingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	if h.delay > 0 {
		time.Sleep(h.delay)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Message)
	return nil
}

func (h *recordingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *recordingHandler) get() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, len(h.records))
	copy(out, h.records)
	return out
}

// waitFor polls until cond returns true or the deadline expires.
func waitFor(t *testing.T, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("condition not met within %v", d)
}

func TestAsyncHandler_PassesRecordsThrough(t *testing.T) {
	rec := &recordingHandler{}
	async := NewAsyncHandler(rec, 10)
	logger := slog.New(async)

	logger.Info("hello")
	logger.Info("world")

	waitFor(t, 500*time.Millisecond, func() bool {
		got := rec.get()
		return len(got) == 2 && got[0] == "hello" && got[1] == "world"
	})
}

func TestAsyncHandler_NeverBlocksOnHandle(t *testing.T) {
	// If Handle blocked on a slow sink, this test would take 10+ seconds.
	rec := &recordingHandler{delay: 10 * time.Second}
	async := NewAsyncHandler(rec, 1)
	logger := slog.New(async)

	start := time.Now()
	for i := 0; i < 1000; i++ {
		logger.Info("msg")
	}
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Errorf("Handle appears to block: 1000 calls took %v (expected <500ms)", elapsed)
	}
}

func TestAsyncHandler_DropsOnFullBuffer(t *testing.T) {
	// Slow sink + tiny buffer guarantees overflow.
	rec := &recordingHandler{delay: 100 * time.Millisecond}
	async := NewAsyncHandler(rec, 2)
	logger := slog.New(async)

	for i := 0; i < 100; i++ {
		logger.Info("msg")
	}

	if async.Dropped() == 0 {
		t.Fatalf("expected some records to be dropped; got 0")
	}
}

func TestAsyncHandler_WithAttrsPreservesAttrs(t *testing.T) {
	// Regression: WithAttrs must produce a child whose records actually
	// carry the added attrs, even though the drain goroutine only sees
	// one shared channel. The asyncRecord.handler field carries the
	// decorated handler per-record to make this work.
	rec := newAttrCapturingHandler()
	async := NewAsyncHandler(rec, 10)

	parent := slog.New(async)
	child := parent.With("request_id", "abc123")
	child.Info("child message")
	parent.Info("parent message")

	waitFor(t, 500*time.Millisecond, func() bool {
		return rec.count() == 2
	})

	// The child's record should carry request_id; the parent's should not.
	got := rec.snapshot()
	var childAttrs, parentAttrs map[string]string
	for _, r := range got {
		if r.msg == "child message" {
			childAttrs = r.attrs
		}
		if r.msg == "parent message" {
			parentAttrs = r.attrs
		}
	}

	if childAttrs == nil {
		t.Fatal("expected to find child message")
	}
	if childAttrs["request_id"] != "abc123" {
		t.Errorf("child message missing request_id attr: got %v", childAttrs)
	}
	if _, ok := parentAttrs["request_id"]; ok {
		t.Errorf("parent message should NOT have request_id attr: got %v", parentAttrs)
	}
}

// attrCapturingHandler is a test handler that records every slog.Record
// it sees along with the attrs it has accumulated via WithAttrs. The
// captured slice is shared across all handlers derived via WithAttrs so
// the test can inspect records from any child from the root reference.
type attrCapturingHandler struct {
	shared *attrCapturingShared
	attrs  []slog.Attr
}

type attrCapturingShared struct {
	mu       sync.Mutex
	captured []capturedRecord
}

type capturedRecord struct {
	msg   string
	attrs map[string]string
}

func newAttrCapturingHandler() *attrCapturingHandler {
	return &attrCapturingHandler{shared: &attrCapturingShared{}}
}

func (h *attrCapturingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *attrCapturingHandler) Handle(_ context.Context, r slog.Record) error {
	attrs := make(map[string]string)
	for _, a := range h.attrs {
		attrs[a.Key] = a.Value.String()
	}
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.String()
		return true
	})
	h.shared.mu.Lock()
	defer h.shared.mu.Unlock()
	h.shared.captured = append(h.shared.captured, capturedRecord{msg: r.Message, attrs: attrs})
	return nil
}

func (h *attrCapturingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	combined := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	combined = append(combined, h.attrs...)
	combined = append(combined, attrs...)
	return &attrCapturingHandler{
		shared: h.shared, // shared across all derivatives
		attrs:  combined,
	}
}

func (h *attrCapturingHandler) WithGroup(_ string) slog.Handler { return h }

func (h *attrCapturingHandler) count() int {
	h.shared.mu.Lock()
	defer h.shared.mu.Unlock()
	return len(h.shared.captured)
}

func (h *attrCapturingHandler) snapshot() []capturedRecord {
	h.shared.mu.Lock()
	defer h.shared.mu.Unlock()
	out := make([]capturedRecord, len(h.shared.captured))
	copy(out, h.shared.captured)
	return out
}
