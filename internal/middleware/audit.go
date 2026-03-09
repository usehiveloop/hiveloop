package middleware

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/useportal/llmvault/internal/model"
)

// AuditWriter is a buffered audit log writer that never blocks the request hot path.
// Entries are queued via a channel and flushed in a background goroutine.
type AuditWriter struct {
	db      *gorm.DB
	entries chan model.AuditEntry
	wg      sync.WaitGroup
}

// NewAuditWriter creates an AuditWriter with the given buffer size and starts
// background flushing. Call Shutdown to flush remaining entries on exit.
func NewAuditWriter(db *gorm.DB, bufferSize int) *AuditWriter {
	aw := &AuditWriter{
		db:      db,
		entries: make(chan model.AuditEntry, bufferSize),
	}
	aw.wg.Add(1)
	go aw.drain()
	return aw
}

func (aw *AuditWriter) drain() {
	defer aw.wg.Done()
	for entry := range aw.entries {
		if err := aw.db.Create(&entry).Error; err != nil {
			slog.Error("audit write failed", "error", err, "action", entry.Action)
		}
	}
}

// Write queues an audit entry. It never blocks — if the buffer is full, the
// entry is dropped and a warning is logged.
func (aw *AuditWriter) Write(entry model.AuditEntry) {
	select {
	case aw.entries <- entry:
	default:
		slog.Warn("audit buffer full, dropping entry", "action", entry.Action)
	}
}

// Shutdown closes the channel and waits for all queued entries to be flushed.
func (aw *AuditWriter) Shutdown(ctx context.Context) {
	close(aw.entries)

	done := make(chan struct{})
	go func() {
		aw.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		slog.Warn("audit shutdown timed out, some entries may be lost")
	}
}

// Audit returns middleware that logs each request to the audit log.
func Audit(aw *AuditWriter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)

			// Build audit entry after handler completes
			entry := model.AuditEntry{
				Action:   "proxy.request",
				Metadata: model.JSON{"method": r.Method, "path": r.URL.Path, "status": sw.status, "latency_ms": time.Since(start).Milliseconds()},
			}

			if ip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
				entry.IPAddress = &ip
			} else {
				addr := r.RemoteAddr
				entry.IPAddress = &addr
			}

			if org, ok := OrgFromContext(r.Context()); ok {
				entry.OrgID = org.ID
			}
			if claims, ok := ClaimsFromContext(r.Context()); ok {
				if credID, err := uuid.Parse(claims.CredentialID); err == nil {
					entry.CredentialID = &credID
				}
			}

			aw.Write(entry)
		})
	}
}

// statusWriter wraps http.ResponseWriter to capture the status code.
type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (sw *statusWriter) WriteHeader(code int) {
	if !sw.wroteHeader {
		sw.status = code
		sw.wroteHeader = true
	}
	sw.ResponseWriter.WriteHeader(code)
}
