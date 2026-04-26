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
	"github.com/usehiveloop/hiveloop/internal/rag/ragpb"
)

// HandlePermSync is the asynq handler for TypeRagPermSync. It drives
// the source's PermSyncConnector and pushes ACL updates to the
// rag-engine via UpdateACL — no re-embed, no chunk rewrite. On clean
// completion the source's last_time_perm_sync is advanced.
//
// Port of try_creating_permissions_sync_task at
// backend/ee/onyx/background/celery/tasks/doc_permission_syncing/tasks.py:230-235.
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

// drainPermSyncStream consumes the per-doc access channel, batches
// updates, and ships each batch through ragclient.UpdateACL.
func drainPermSyncStream(
	ctx context.Context,
	deps *Deps,
	src *ragmodel.RAGSource,
	stream <-chan interfaces.DocExternalAccessOrFailure,
) error {
	const batchSize = 200
	batch := make([]*ragpb.UpdateACLEntry, 0, batchSize)
	batchID := 0
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		batchID++
		if _, err := deps.RagClient.UpdateACL(ctx, &ragpb.UpdateACLRequest{
			DatasetName:    deps.DatasetName,
			OrgId:          src.OrgIDValue.String(),
			Entries:        batch,
			IdempotencyKey: fmt.Sprintf("perm-sync-%s-%d", src.ID, batchID),
		}); err != nil {
			return fmt.Errorf("UpdateACL (%d docs): %w", len(batch), err)
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
		batch = append(batch, &ragpb.UpdateACLEntry{
			DocId:    item.Access.DocID,
			Acl:      append([]string(nil), item.Access.ExternalAccess.ExternalUserEmails...),
			IsPublic: item.Access.ExternalAccess.IsPublic,
		})
		if len(batch) >= batchSize {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	return flush()
}

// persistACLLocal mirrors the ACL update into rag_documents so search
// can ACL-filter without round-tripping to Rust on every query.
func persistACLLocal(ctx context.Context, db *gorm.DB, batch []*ragpb.UpdateACLEntry) error {
	if len(batch) == 0 {
		return nil
	}
	now := time.Now()
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, u := range batch {
			if err := tx.Model(&ragmodel.RAGDocument{}).
				Where("id = ?", u.DocId).
				Updates(map[string]any{
					"external_user_emails": pq.StringArray(u.Acl),
					"is_public":            u.IsPublic,
					"last_modified":        now,
					"last_synced":          now,
				}).Error; err != nil {
				return fmt.Errorf("update rag_documents %s acl: %w", u.DocId, err)
			}
		}
		return nil
	})
}

