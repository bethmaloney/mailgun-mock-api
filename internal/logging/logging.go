package logging

import (
	"log/slog"
	"os"
)

// bufferSize is how many log records can be queued before AsyncHandler
// starts dropping. 4096 records at ~500 bytes each is ~2 MB of slack,
// which is plenty to absorb bursts from HTTP handlers without dropping
// anything in normal operation. If records are being dropped, something
// is genuinely wrong with the log sink.
const bufferSize = 4096

// Init configures slog and the legacy log package to route through an
// async handler backed by os.Stderr. Call once at process startup,
// before any HTTP server begins handling requests.
//
// After Init:
//   - slog.Info / slog.Warn / etc. route through the async handler.
//   - log.Printf and related legacy-log-package calls also route
//     through the async handler — slog.SetDefault automatically bridges
//     the global log.Logger to the slog default, which is how chi's
//     middleware.Logger and any stray log.Printf inherit the
//     non-blocking behavior.
//   - Neither path can block the caller on a slow or full stderr pipe.
//
// Returns the handler so callers can inspect its dropped counter.
func Init() *AsyncHandler {
	textHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	async := NewAsyncHandler(textHandler, bufferSize)
	slog.SetDefault(slog.New(async))
	return async
}
