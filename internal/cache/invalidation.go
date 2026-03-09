package cache

import (
	"context"
	"log/slog"
	"sync"

	"github.com/redis/go-redis/v9"
)

const (
	CredentialChannel = "llmvault:invalidate:credential"
	TokenChannel      = "llmvault:invalidate:token"
)

// Invalidator handles cross-instance cache invalidation via Redis pub/sub.
// When a credential or token is revoked on one instance, all other instances
// are notified to purge their L1 caches.
type Invalidator struct {
	client   *redis.Client
	memCache *MemoryCache
	dekCache *DEKCache

	// In-memory set of recently revoked JTIs (populated by pub/sub).
	revokedMu  sync.RWMutex
	revokedSet map[string]struct{}
}

// NewInvalidator creates a new cross-instance invalidator.
func NewInvalidator(client *redis.Client, memCache *MemoryCache, dekCache *DEKCache) *Invalidator {
	return &Invalidator{
		client:     client,
		memCache:   memCache,
		dekCache:   dekCache,
		revokedSet: make(map[string]struct{}),
	}
}

// PublishCredentialInvalidation notifies all instances to evict a credential.
func (inv *Invalidator) PublishCredentialInvalidation(ctx context.Context, credentialID string) error {
	return inv.client.Publish(ctx, CredentialChannel, credentialID).Err()
}

// PublishTokenRevocation notifies all instances that a token JTI was revoked.
func (inv *Invalidator) PublishTokenRevocation(ctx context.Context, jti string) error {
	return inv.client.Publish(ctx, TokenChannel, jti).Err()
}

// IsTokenLocallyRevoked checks the in-memory revoked set (populated by pub/sub).
func (inv *Invalidator) IsTokenLocallyRevoked(jti string) bool {
	inv.revokedMu.RLock()
	defer inv.revokedMu.RUnlock()
	_, ok := inv.revokedSet[jti]
	return ok
}

// Subscribe listens for invalidation messages. Blocks until ctx is cancelled.
// Run this in a goroutine.
func (inv *Invalidator) Subscribe(ctx context.Context) error {
	pubsub := inv.client.Subscribe(ctx, CredentialChannel, TokenChannel)
	defer pubsub.Close()

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				return nil
			}
			switch msg.Channel {
			case CredentialChannel:
				inv.memCache.Invalidate(msg.Payload)
				inv.dekCache.Invalidate(msg.Payload)
			case TokenChannel:
				inv.revokedMu.Lock()
				inv.revokedSet[msg.Payload] = struct{}{}
				inv.revokedMu.Unlock()
			default:
				slog.Warn("unknown invalidation channel", "channel", msg.Channel)
			}
		}
	}
}
