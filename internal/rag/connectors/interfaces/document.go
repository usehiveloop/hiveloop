package interfaces

import "time"

// Document is the neutral shape every connector produces. JSON tags are
// load-bearing — these travel across checkpoint storage and test fixtures.
type Document struct {
	DocID         string            `json:"doc_id"`
	SemanticID    string            `json:"semantic_id"`
	Link          string            `json:"link"`
	Sections      []Section         `json:"sections"`
	ACL           []string          `json:"acl,omitempty"`
	IsPublic      bool              `json:"is_public"`
	DocUpdatedAt  *time.Time        `json:"doc_updated_at,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	PrimaryOwners []string          `json:"primary_owners,omitempty"`
	SecondaryOwners []string        `json:"secondary_owners,omitempty"`
}

// Section is one content block. The chunker MUST skip empty sections.
type Section struct {
	Text  string `json:"text"`
	Link  string `json:"link,omitempty"`
	Title string `json:"title,omitempty"`
}

// SlimDocument is the minimal shape produced by SlimConnector.ListAllSlim,
// used by the prune loop to diff against the indexed set.
type SlimDocument struct {
	DocID          string          `json:"doc_id"`
	ExternalAccess *ExternalAccess `json:"external_access,omitempty"`
}
