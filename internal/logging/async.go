// Package logging provides a non-blocking slog.Handler for use in HTTP
// servers where a slow or stalled log sink would otherwise wedge the
// request path.
//
// The motivation is specific: Go's log package and slog's stock
// handlers call io.Writer.Write directly, which for a piped stderr
// blocks inside the kernel write syscall once the pipe buffer fills —
// while holding the file descriptor's write mutex. Every subsequent
// caller contends on that mutex. In a server that logs from inside
// HTTP handlers, this means one slow log reader can wedge every
// request. See commit e9232ec for the incident that motivated this
// package.
package logging

import (
	"context"
	"log/slog"
	"sync/atomic"
)

// AsyncHandler wraps another slog.Handler and processes records on a
// background goroutine. Handle never blocks the caller:
//
//   - If the internal buffer has room, the record is queued and Handle
//     returns immediately.
//   - If the buffer is full, the record is dropped and a counter is
//     incremented (see Dropped).
//
// The background goroutine runs for the entire process lifetime; there
// is deliberately no Close method. Records still in the buffer on
// process exit are lost, which is acceptable for the dev/test use case
// this package targets.
type AsyncHandler struct {
	state      *asyncState // shared across WithAttrs/WithGroup derivatives
	underlying slog.Handler
}

// asyncState is the shared state across the original AsyncHandler and
// every child created via WithAttrs/WithGroup. It owns the channel,
// drain goroutine, and dropped counter.
type asyncState struct {
	records chan asyncRecord
	dropped atomic.Uint64
}

// asyncRecord pairs a record with the fully-decorated handler that
// should process it. Carrying the handler per-record lets child
// handlers (from WithAttrs / WithGroup) honor their attrs even though
// the single drain goroutine only sees one channel.
type asyncRecord struct {
	handler slog.Handler
	record  slog.Record
}

// NewAsyncHandler wraps underlying and starts the drain goroutine.
// bufferSize controls how many records can be queued before Handle
// starts dropping.
func NewAsyncHandler(underlying slog.Handler, bufferSize int) *AsyncHandler {
	state := &asyncState{
		records: make(chan asyncRecord, bufferSize),
	}
	go state.run()
	return &AsyncHandler{
		state:      state,
		underlying: underlying,
	}
}

func (s *asyncState) run() {
	ctx := context.Background()
	for ar := range s.records {
		_ = ar.handler.Handle(ctx, ar.record)
	}
}

// Enabled delegates to the underlying handler.
func (h *AsyncHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.underlying.Enabled(ctx, level)
}

// Handle queues r for asynchronous processing. It never blocks.
// On a full buffer the record is dropped and Dropped is incremented.
func (h *AsyncHandler) Handle(_ context.Context, r slog.Record) error {
	select {
	case h.state.records <- asyncRecord{handler: h.underlying, record: r}:
	default:
		h.state.dropped.Add(1)
	}
	return nil
}

// WithAttrs returns a new handler whose records carry the given attrs,
// sharing the same underlying channel, drain goroutine, and drop counter.
func (h *AsyncHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &AsyncHandler{
		state:      h.state,
		underlying: h.underlying.WithAttrs(attrs),
	}
}

// WithGroup returns a new handler that nests records inside the given
// group, sharing state with its parent.
func (h *AsyncHandler) WithGroup(name string) slog.Handler {
	return &AsyncHandler{
		state:      h.state,
		underlying: h.underlying.WithGroup(name),
	}
}

// Dropped returns the cumulative number of records dropped due to a
// full buffer since the handler was created.
func (h *AsyncHandler) Dropped() uint64 {
	return h.state.dropped.Load()
}
