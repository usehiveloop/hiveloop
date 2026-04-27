package tasks

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/qdrant"
)

type aclUpdate struct {
	docID    string
	acl      []string
	isPublic bool
}

func (d *Deps) HandlePermSync(ctx context.Context, t *asynq.Task) error {
	deps := d.withDefaults()
	payload, err := UnmarshalPermSync(t.Payload())
	if err != nil {
		return err
	}
	src, err := loadSource(ctx, deps.DB, payload.RAGSourceID)
	if err != nil {
		return err
	}
	conn, err := buildConnector(src, deps)
	if err != nil {
		return err
	}
	syncer, ok := conn.(interfaces.PermSyncConnector)
	if !ok {
		return fmt.Errorf("perm_sync %s: connector kind %q does not implement PermSyncConnector",
			src.ID, src.KindValue)
	}

	stream, err := syncer.SyncDocPermissions(ctx, src)
	if err != nil {
		return fmt.Errorf("perm_sync %s: SyncDocPermissions: %w", src.ID, err)
	}
	if err := drainPermSyncStream(ctx, deps, src, stream); err != nil {
		return err
	}

	now := time.Now()
	if err := deps.DB.WithContext(ctx).
		Model(&ragmodel.RAGSource{}).
		Where("id = ?", src.ID).
		Updates(map[string]any{
			"last_time_perm_sync": now,
			"updated_at":          now,
		}).Error; err != nil {
		return fmt.Errorf("perm_sync %s: advance last_time_perm_sync: %w", src.ID, err)
	}
	return nil
}

func drainPermSyncStream(
	ctx context.Context,
	deps *Deps,
	src *ragmodel.RAGSource,
	stream <-chan interfaces.DocExternalAccessOrFailure,
) error {
	const batchSize = 200
	batch := make([]aclUpdate, 0, batchSize)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := pushACLToQdrant(ctx, deps, src, batch); err != nil {
			return err
		}
		if err := persistACLLocal(ctx, deps.DB, batch); err != nil {
			return err
		}
		batch = batch[:0]
		return nil
	}
	for item := range stream {
		if item.Failure != nil {
			slog.Warn("perm_sync per-doc failure",
				"source_id", src.ID, "msg", item.Failure.FailureMessage)
			continue
		}
		if item.Access == nil || item.Access.ExternalAccess == nil {
			continue
		}
		batch = append(batch, aclUpdate{
			docID:    item.Access.DocID,
			acl:      append([]string(nil), item.Access.ExternalAccess.ExternalUserEmails...),
			isPublic: item.Access.ExternalAccess.IsPublic,
		})
		if len(batch) >= batchSize {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	return flush()
}

// Qdrant set_payload accepts a single payload object applied to many points.
// We group updates by ACL+is_public signature so docs sharing perms are
// updated in one round-trip, instead of one call per doc.
func pushACLToQdrant(ctx context.Context, deps *Deps, src *ragmodel.RAGSource, batch []aclUpdate) error {
	groups := map[string][]string{}
	specs := map[string]aclUpdate{}
	for _, u := range batch {
		key := fmt.Sprintf("%v|%s", u.isPublic, joinSorted(u.acl))
		groups[key] = append(groups[key], qdrant.PointID(src.OrgIDValue.String(), u.docID))
		specs[key] = u
	}
	for key, ids := range groups {
		spec := specs[key]
		payload := map[string]any{
			"acl":       append([]string(nil), spec.acl...),
			"is_public": spec.isPublic,
		}
		if err := deps.Qdrant.SetPayload(ctx, deps.Collection, ids, payload); err != nil {
			return fmt.Errorf("set_payload (%d docs): %w", len(ids), err)
		}
	}
	return nil
}

func joinSorted(ss []string) string {
	cp := append([]string(nil), ss...)
	for i := 1; i < len(cp); i++ {
		for j := i; j > 0 && cp[j] < cp[j-1]; j-- {
			cp[j], cp[j-1] = cp[j-1], cp[j]
		}
	}
	out := ""
	for _, s := range cp {
		out += s + ","
	}
	return out
}

func persistACLLocal(ctx context.Context, db *gorm.DB, batch []aclUpdate) error {
	if len(batch) == 0 {
		return nil
	}
	now := time.Now()
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, u := range batch {
			if err := tx.Model(&ragmodel.RAGDocument{}).
				Where("id = ?", u.docID).
				Updates(map[string]any{
					"external_user_emails": pq.StringArray(u.acl),
					"is_public":            u.isPublic,
					"last_modified":        now,
					"last_synced":          now,
				}).Error; err != nil {
				return fmt.Errorf("update rag_documents %s acl: %w", u.docID, err)
			}
		}
		return nil
	})
}
