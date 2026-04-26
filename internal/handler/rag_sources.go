package handler

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/model"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
)

type RAGSourceHandler struct {
	db   *gorm.DB
	enq  enqueue.TaskEnqueuer
	caps RAGCapabilityCheck
}

// RAGCapabilityCheck answers "can a connector of this kind perform
// permission sync?". Production wires scheduler.HasPermSyncCapability;
// tests inject a fake whose answer is fixed per-kind so the test
// doesn't depend on a real connector being registered.
type RAGCapabilityCheck func(kind string) bool

func NewRAGSourceHandler(db *gorm.DB, enq enqueue.TaskEnqueuer, caps RAGCapabilityCheck) *RAGSourceHandler {
	return &RAGSourceHandler{db: db, enq: enq, caps: caps}
}

type ragSourceResponse struct {
	ID                      string          `json:"id"`
	OrgID                   string          `json:"org_id"`
	Kind                    string          `json:"kind"`
	Name                    string          `json:"name"`
	Status                  string          `json:"status"`
	Enabled                 bool            `json:"enabled"`
	InConnectionID          *string         `json:"in_connection_id,omitempty"`
	AccessType              string          `json:"access_type"`
	Config                  json.RawMessage `json:"config"`
	IndexingStart           *time.Time      `json:"indexing_start,omitempty"`
	LastSuccessfulIndexTime *time.Time      `json:"last_successful_index_time,omitempty"`
	LastTimePermSync        *time.Time      `json:"last_time_perm_sync,omitempty"`
	LastPruned              *time.Time      `json:"last_pruned,omitempty"`
	RefreshFreqSeconds      *int            `json:"refresh_freq_seconds,omitempty"`
	PruneFreqSeconds        *int            `json:"prune_freq_seconds,omitempty"`
	PermSyncFreqSeconds     *int            `json:"perm_sync_freq_seconds,omitempty"`
	TotalDocsIndexed        int             `json:"total_docs_indexed"`
	InRepeatedErrorState    bool            `json:"in_repeated_error_state"`
	CreatedAt               time.Time       `json:"created_at"`
	UpdatedAt               time.Time       `json:"updated_at"`
}

type ragSourceDetailResponse struct {
	ragSourceResponse
	RecentAttempts []ragIndexAttemptResponse `json:"recent_attempts"`
}

type ragIndexAttemptResponse struct {
	ID                   string     `json:"id"`
	Status               string     `json:"status"`
	FromBeginning        bool       `json:"from_beginning"`
	NewDocsIndexed       *int       `json:"new_docs_indexed,omitempty"`
	TotalDocsIndexed     *int       `json:"total_docs_indexed,omitempty"`
	DocsRemovedFromIndex *int       `json:"docs_removed_from_index,omitempty"`
	ErrorMsg             *string    `json:"error_msg,omitempty"`
	PollRangeStart       *time.Time `json:"poll_range_start,omitempty"`
	PollRangeEnd         *time.Time `json:"poll_range_end,omitempty"`
	TimeStarted          *time.Time `json:"time_started,omitempty"`
	TimeCreated          time.Time  `json:"time_created"`
	TimeUpdated          time.Time  `json:"time_updated"`
}

type ragAttemptDetailResponse struct {
	ragIndexAttemptResponse
	FullExceptionTrace *string                  `json:"full_exception_trace,omitempty"`
	Errors             []ragAttemptErrorPayload `json:"errors"`
	ErrorCount         int                      `json:"error_count"`
}

type ragAttemptErrorPayload struct {
	ID                   string     `json:"id"`
	DocumentID           *string    `json:"document_id,omitempty"`
	DocumentLink         *string    `json:"document_link,omitempty"`
	EntityID             *string    `json:"entity_id,omitempty"`
	FailedTimeRangeStart *time.Time `json:"failed_time_range_start,omitempty"`
	FailedTimeRangeEnd   *time.Time `json:"failed_time_range_end,omitempty"`
	FailureMessage       string     `json:"failure_message"`
	IsResolved           bool       `json:"is_resolved"`
	ErrorType            *string    `json:"error_type,omitempty"`
	TimeCreated          time.Time  `json:"time_created"`
}

type ragIntegrationResponse struct {
	ID          string `json:"id"`
	UniqueKey   string `json:"unique_key"`
	Provider    string `json:"provider"`
	DisplayName string `json:"display_name"`
}

type ragListResponse struct {
	Data  []ragSourceResponse `json:"data"`
	Total int64               `json:"total"`
	Page  int                 `json:"page"`
	Size  int                 `json:"page_size"`
}

type ragAttemptsListResponse struct {
	Data  []ragIndexAttemptResponse `json:"data"`
	Total int64                     `json:"total"`
	Page  int                       `json:"page"`
	Size  int                       `json:"page_size"`
}

func toRAGSourceResponse(s *ragmodel.RAGSource) ragSourceResponse {
	resp := ragSourceResponse{
		ID:                      s.ID.String(),
		OrgID:                   s.OrgIDValue.String(),
		Kind:                    string(s.KindValue),
		Name:                    s.Name,
		Status:                  string(s.Status),
		Enabled:                 s.Enabled,
		AccessType:              string(s.AccessType),
		Config:                  s.Config(),
		IndexingStart:           s.IndexingStart,
		LastSuccessfulIndexTime: s.LastSuccessfulIndexTime,
		LastTimePermSync:        s.LastTimePermSync,
		LastPruned:              s.LastPruned,
		RefreshFreqSeconds:      s.RefreshFreqSeconds,
		PruneFreqSeconds:        s.PruneFreqSeconds,
		PermSyncFreqSeconds:     s.PermSyncFreqSeconds,
		TotalDocsIndexed:        s.TotalDocsIndexed,
		InRepeatedErrorState:    s.InRepeatedErrorState,
		CreatedAt:               s.CreatedAt,
		UpdatedAt:               s.UpdatedAt,
	}
	if s.InConnectionID != nil {
		v := s.InConnectionID.String()
		resp.InConnectionID = &v
	}
	return resp
}

func toRAGAttemptResponse(a *ragmodel.RAGIndexAttempt) ragIndexAttemptResponse {
	return ragIndexAttemptResponse{
		ID:                   a.ID.String(),
		Status:               string(a.Status),
		FromBeginning:        a.FromBeginning,
		NewDocsIndexed:       a.NewDocsIndexed,
		TotalDocsIndexed:     a.TotalDocsIndexed,
		DocsRemovedFromIndex: a.DocsRemovedFromIndex,
		ErrorMsg:             a.ErrorMsg,
		PollRangeStart:       a.PollRangeStart,
		PollRangeEnd:         a.PollRangeEnd,
		TimeStarted:          a.TimeStarted,
		TimeCreated:          a.TimeCreated,
		TimeUpdated:          a.TimeUpdated,
	}
}

func toRAGIntegrationResponse(i *model.InIntegration) ragIntegrationResponse {
	return ragIntegrationResponse{
		ID:          i.ID.String(),
		UniqueKey:   i.UniqueKey,
		Provider:    i.Provider,
		DisplayName: i.DisplayName,
	}
}

func parseSourceID(s string) (uuid.UUID, bool) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}
