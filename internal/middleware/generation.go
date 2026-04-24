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
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/goroutine"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/observe"
	"github.com/usehiveloop/hiveloop/internal/registry"
)

const generationBatchSize = 50

// GenerationWriter is a buffered generation log writer that never blocks the request hot path.
type GenerationWriter struct {
	db            *gorm.DB
	reg           *registry.Registry
	entries       chan model.Generation
	wg            sync.WaitGroup
	flushInterval time.Duration
}

// NewGenerationWriter creates a GenerationWriter with the given buffer size and starts
// background flushing. Call Shutdown to flush remaining entries on exit.
// An optional flushInterval controls how often partial batches are flushed
// (default 500ms).
func NewGenerationWriter(db *gorm.DB, reg *registry.Registry, bufferSize int, flushInterval ...time.Duration) *GenerationWriter {
	interval := 500 * time.Millisecond
	if len(flushInterval) > 0 {
		interval = flushInterval[0]
	}
	gw := &GenerationWriter{
		db:            db,
		reg:           reg,
		entries:       make(chan model.Generation, bufferSize),
		flushInterval: interval,
	}
	gw.wg.Add(1)
	go gw.drain()
	return gw
}

func (gw *GenerationWriter) drain() {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("generation drain panicked",
				"panic", r,
				"stack", string(debug.Stack()),
			)
		}
		gw.wg.Done()
	}()

	batch := make([]model.Generation, 0, generationBatchSize)
	timer := time.NewTimer(gw.flushInterval)
	defer timer.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := gw.db.CreateInBatches(batch, generationBatchSize).Error; err != nil {
			slog.Error("generation batch write failed", "error", err, "count", len(batch))
		}
		batch = batch[:0]
	}

	for {
		select {
		case gen, ok := <-gw.entries:
			if !ok {
				flush()
				return
			}
			batch = append(batch, gen)
			if len(batch) >= generationBatchSize {
				flush()
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(gw.flushInterval)
			}
		case <-timer.C:
			flush()
			timer.Reset(gw.flushInterval)
		}
	}
}

// Write queues a generation entry. It never blocks — if the buffer is full, the
// entry is dropped and a warning is logged.
func (gw *GenerationWriter) Write(gen model.Generation) {
	select {
	case gw.entries <- gen:
	default:
		slog.Warn("generation buffer full, dropping entry", "id", gen.ID)
	}
}

// Shutdown closes the channel and waits for all queued entries to be flushed.
func (gw *GenerationWriter) Shutdown(ctx context.Context) {
	close(gw.entries)

	done := make(chan struct{})
	goroutine.Go(ctx, func(context.Context) {
		gw.wg.Wait()
		close(done)
	})

	select {
	case <-done:
	case <-ctx.Done():
		slog.Warn("generation shutdown timed out, some entries may be lost")
	}
}

// Generation returns middleware that captures observability data for proxy requests.
// It sets up the CapturedData on the request context before the proxy runs,
// then after the response, builds a Generation record and queues it for writing.
func Generation(gw *GenerationWriter, db *gorm.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsFromContext(r.Context())
			if !ok {
				next.ServeHTTP(w, r)
				return
			}

			// Look up credential's provider_id
			providerID := lookupProviderID(db, claims.CredentialID)

			// Set up captured data on context for the CaptureTransport to fill
			captured := &observe.CapturedData{
				ProviderID: providerID,
			}
			ctx := observe.WithCapturedData(r.Context(), captured)
			r = r.WithContext(ctx)

			next.ServeHTTP(w, r)

			// After response: build and queue generation record
			gen := buildGeneration(r, claims, captured, providerID, gw.reg, db)
			gw.Write(gen)
		})
	}
}

func buildGeneration(r *http.Request, claims *TokenClaims, captured *observe.CapturedData, providerID string, reg *registry.Registry, db *gorm.DB) model.Generation {
	genID := "gen_" + ulid.Make().String()

	orgID, _ := uuid.Parse(claims.OrgID)
	credID, _ := uuid.Parse(claims.CredentialID)

	gen := model.Generation{
		ID:           genID,
		OrgID:        orgID,
		CredentialID: credID,
		TokenJTI:     claims.JTI,
		ProviderID:   providerID,
		Model:        captured.Model,
		RequestPath:  r.URL.Path,
		IsStreaming:  captured.IsStreaming,

		InputTokens:     captured.Usage.InputTokens,
		OutputTokens:    captured.Usage.OutputTokens,
		CachedTokens:    captured.Usage.CachedTokens,
		ReasoningTokens: captured.Usage.ReasoningTokens,

		TTFBMs:         &captured.TTFBMs,
		TotalMs:        captured.TotalMs,
		UpstreamStatus: captured.UpstreamStatus,

		ErrorType:    captured.ErrorType,
		ErrorMessage: truncate(captured.ErrorMessage, 1000),

		CreatedAt: time.Now().UTC(),
	}

	// Cost calculation using observe usage data
	gen.Cost = calculateCost(reg, providerID, captured.Model, captured.Usage)

	// Identity from context
	// Attribution from token meta
	extractAttribution(db, claims.JTI, &gen)

	// IP address
	if ip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		gen.IPAddress = &ip
	} else {
		addr := r.RemoteAddr
		gen.IPAddress = &addr
	}

	return gen
}

// calculateCost computes cost using the registry, wrapping the observe.UsageData.
func calculateCost(reg *registry.Registry, providerID, modelID string, usage observe.UsageData) float64 {
	if reg == nil || providerID == "" || modelID == "" {
		return 0
	}

	provider, ok := reg.GetProvider(providerID)
	if !ok {
		return 0
	}

	model, ok := provider.Models[modelID]
	if !ok {
		return 0
	}

	if model.Cost == nil {
		return 0
	}

	inputPrice := model.Cost.Input
	outputPrice := model.Cost.Output

	nonCachedInput := usage.InputTokens - usage.CachedTokens
	if nonCachedInput < 0 {
		nonCachedInput = 0
	}
	inputCost := float64(nonCachedInput) * inputPrice / 1_000_000

	discount := cachedTokenDiscount[providerID]
	if discount == 0 && usage.CachedTokens > 0 {
		discount = 1.0
	}
	cachedCost := float64(usage.CachedTokens) * inputPrice * discount / 1_000_000
	outputCost := float64(usage.OutputTokens) * outputPrice / 1_000_000

	return inputCost + cachedCost + outputCost
}

var cachedTokenDiscount = map[string]float64{
	"anthropic":     0.10,
	"openai":        0.50,
	"google":        0.25,
	"google-vertex": 0.25,
}

// extractAttribution reads token.meta to extract user_id and tags.
func extractAttribution(db *gorm.DB, jti string, gen *model.Generation) {
	var token model.Token
	if err := db.Select("meta").Where("jti = ?", jti).First(&token).Error; err != nil {
		return
	}

	if token.Meta == nil {
		return
	}

	if user, ok := token.Meta["user"].(string); ok {
		gen.UserID = user
	}

	if tags, ok := token.Meta["tags"].([]any); ok {
		for _, t := range tags {
			if s, ok := t.(string); ok {
				gen.Tags = append(gen.Tags, s)
			}
		}
	}
}

// lookupProviderID fetches the credential's provider_id from the database.
func lookupProviderID(db *gorm.DB, credentialID string) string {
	var providerID string
	db.Model(&model.Credential{}).Select("provider_id").Where("id = ?", credentialID).Scan(&providerID)
	return providerID
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
