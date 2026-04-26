package db

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
)

const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// ListOptions controls org-scoped pagination + filtering for
// ListSourcesForOrg. Page is zero-indexed (page 0 is the first page),
// matching Onyx's `cc_pair.py:82` convention.
type ListOptions struct {
	Page         int
	PageSize     int
	StatusFilter *ragmodel.RAGSourceStatus
	KindFilter   *ragmodel.RAGSourceKind
}

func (o ListOptions) normalized() ListOptions {
	if o.Page < 0 {
		o.Page = 0
	}
	if o.PageSize <= 0 {
		o.PageSize = DefaultPageSize
	}
	if o.PageSize > MaxPageSize {
		o.PageSize = MaxPageSize
	}
	return o
}

func ListSourcesForOrg(db *gorm.DB, orgID uuid.UUID, opts ListOptions) ([]ragmodel.RAGSource, int64, error) {
	o := opts.normalized()

	q := db.Model(&ragmodel.RAGSource{}).Where("org_id = ?", orgID)
	if o.StatusFilter != nil {
		q = q.Where("status = ?", *o.StatusFilter)
	}
	if o.KindFilter != nil {
		q = q.Where("kind = ?", *o.KindFilter)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []ragmodel.RAGSource
	if err := q.Order("created_at DESC, id DESC").
		Offset(o.Page * o.PageSize).
		Limit(o.PageSize).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func GetSourceForOrg(db *gorm.DB, orgID, sourceID uuid.UUID) (*ragmodel.RAGSource, error) {
	var src ragmodel.RAGSource
	if err := db.Where("id = ? AND org_id = ?", sourceID, orgID).First(&src).Error; err != nil {
		return nil, err
	}
	return &src, nil
}

// ListAttemptsForSource returns paginated attempts ordered newest first.
// Matches Onyx's `cc_pair.py:82` paginated index-attempts endpoint.
func ListAttemptsForSource(
	db *gorm.DB,
	orgID, sourceID uuid.UUID,
	page, pageSize int,
) ([]ragmodel.RAGIndexAttempt, int64, error) {
	if page < 0 {
		page = 0
	}
	if pageSize <= 0 {
		pageSize = DefaultPageSize
	}
	if pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}

	q := db.Model(&ragmodel.RAGIndexAttempt{}).
		Where("rag_source_id = ? AND org_id = ?", sourceID, orgID)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []ragmodel.RAGIndexAttempt
	if err := q.Order("time_created DESC, id DESC").
		Offset(page * pageSize).
		Limit(pageSize).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func GetAttemptForSource(
	db *gorm.DB,
	orgID, sourceID, attemptID uuid.UUID,
) (*ragmodel.RAGIndexAttempt, error) {
	var attempt ragmodel.RAGIndexAttempt
	if err := db.Where(
		"id = ? AND rag_source_id = ? AND org_id = ?",
		attemptID, sourceID, orgID,
	).First(&attempt).Error; err != nil {
		return nil, err
	}
	return &attempt, nil
}

// LatestAttemptsBySource returns the most-recent RAGIndexAttempt for
// every source ID in the input set, keyed by RAGSourceID. Sources with
// zero attempts are absent from the result.
//
// Uses Postgres DISTINCT ON to fetch one row per source in a single
// query — cheap even for the org-wide List response.
func LatestAttemptsBySource(
	db *gorm.DB,
	orgID uuid.UUID,
	sourceIDs []uuid.UUID,
) (map[uuid.UUID]ragmodel.RAGIndexAttempt, error) {
	out := map[uuid.UUID]ragmodel.RAGIndexAttempt{}
	if len(sourceIDs) == 0 {
		return out, nil
	}
	var rows []ragmodel.RAGIndexAttempt
	if err := db.Raw(`
		SELECT DISTINCT ON (rag_source_id) *
		FROM rag_index_attempts
		WHERE org_id = ? AND rag_source_id IN ?
		ORDER BY rag_source_id, time_created DESC, id DESC
	`, orgID, sourceIDs).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for i := range rows {
		out[rows[i].RAGSourceID] = rows[i]
	}
	return out, nil
}

// ListRecentAttempts returns the n most-recent attempts for a source,
// org-scoped. Used by the source-detail response to inline the last
// few attempts without a second round trip.
func ListRecentAttempts(
	db *gorm.DB,
	orgID, sourceID uuid.UUID,
	n int,
) ([]ragmodel.RAGIndexAttempt, error) {
	if n <= 0 {
		n = 5
	}
	var rows []ragmodel.RAGIndexAttempt
	if err := db.Where("rag_source_id = ? AND org_id = ?", sourceID, orgID).
		Order("time_created DESC, id DESC").
		Limit(n).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func ListAttemptErrors(
	db *gorm.DB,
	attemptID uuid.UUID,
	page, pageSize int,
) ([]ragmodel.RAGIndexAttemptError, int64, error) {
	if page < 0 {
		page = 0
	}
	if pageSize <= 0 {
		pageSize = DefaultPageSize
	}
	if pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}

	q := db.Model(&ragmodel.RAGIndexAttemptError{}).Where("index_attempt_id = ?", attemptID)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []ragmodel.RAGIndexAttemptError
	if err := q.Order("time_created DESC, id DESC").
		Offset(page * pageSize).
		Limit(pageSize).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func ListSupportedIntegrations(db *gorm.DB) ([]model.InIntegration, error) {
	var rows []model.InIntegration
	if err := db.Where("supports_rag_source = ? AND deleted_at IS NULL", true).
		Order("display_name ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
