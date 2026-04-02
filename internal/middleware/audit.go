package middleware

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/llmvault/llmvault/internal/goroutine"
	"github.com/llmvault/llmvault/internal/model"
)

const auditBatchSize = 50

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
	defer func() {
		if r := recover(); r != nil {
			slog.Error("audit drain panicked",
				"panic", r,
				"stack", string(debug.Stack()),
			)
		}
		aw.wg.Done()
	}()

	batch := make([]model.AuditEntry, 0, auditBatchSize)
	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := aw.db.CreateInBatches(batch, auditBatchSize).Error; err != nil {
			slog.Error("audit batch write failed", "error", err, "count", len(batch))
		}
		batch = batch[:0]
	}

	for {
		select {
		case entry, ok := <-aw.entries:
			if !ok {
				flush()
				return
			}
			batch = append(batch, entry)
			if len(batch) >= auditBatchSize {
				flush()
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(500 * time.Millisecond)
			}
		case <-timer.C:
			flush()
			timer.Reset(500 * time.Millisecond)
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
	goroutine.Go(func() {
		aw.wg.Wait()
		close(done)
	})

	select {
	case <-done:
	case <-ctx.Done():
		slog.Warn("audit shutdown timed out, some entries may be lost")
	}
}

// Audit returns middleware that logs each request to the audit log.
// The action parameter distinguishes between proxy and management requests.
func Audit(aw *AuditWriter, action ...string) func(http.Handler) http.Handler {
	a := "api.request"
	if len(action) > 0 {
		a = action[0]
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)

			// Build audit entry after handler completes
			entry := model.AuditEntry{
				Action:   a,
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
				// For proxy routes, org may not be in context — extract from claims
				if entry.OrgID == uuid.Nil {
					if orgID, err := uuid.Parse(claims.OrgID); err == nil {
						entry.OrgID = orgID
					}
				}
			}
			if identID, ok := CredentialIdentityIDFromContext(r.Context()); ok && identID != nil {
				entry.IdentityID = identID
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

// Unwrap returns the underlying ResponseWriter (supports http.ResponseController).
func (sw *statusWriter) Unwrap() http.ResponseWriter {
	return sw.ResponseWriter
}
