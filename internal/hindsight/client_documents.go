package hindsight

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// DocumentResponse is the response from GET /documents/{documentID}.
type DocumentResponse struct {
	DocumentID string         `json:"document_id,omitempty"`
	ID         string         `json:"id,omitempty"`
	Tags       []string       `json:"tags,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	Raw        map[string]any `json:"-"`
}

// DeleteDocumentResponse is the response from DELETE /documents/{documentID}.
type DeleteDocumentResponse struct {
	BankID             string         `json:"bank_id,omitempty"`
	DocumentID         string         `json:"document_id,omitempty"`
	Deleted            bool           `json:"deleted"`
	MemoryUnitsDeleted int            `json:"memory_units_deleted"`
	Raw                map[string]any `json:"-"`
}

// GetDocument returns document metadata and source content from a bank.
func (c *Client) GetDocument(ctx context.Context, bankID, documentID string) (*DocumentResponse, error) {
	resp, err := c.do(ctx, http.MethodGet, documentPath(bankID, documentID), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("hindsight get document: status %d: %s", resp.StatusCode, string(body))
	}

	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("hindsight get document: decoding response: %w", err)
	}
	return documentResponseFromRaw(raw), nil
}

// DeleteDocument deletes a document and its associated memory units from a bank.
func (c *Client) DeleteDocument(ctx context.Context, bankID, documentID string) (*DeleteDocumentResponse, error) {
	resp, err := c.do(ctx, http.MethodDelete, documentPath(bankID, documentID), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hindsight delete document: status %d: %s", resp.StatusCode, string(body))
	}
	result := deleteDocumentResponseFromRaw(nil, bankID, documentID)
	if len(bytes.TrimSpace(body)) == 0 {
		return result, nil
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("hindsight delete document: decoding response: %w", err)
	}
	return deleteDocumentResponseFromRaw(raw, bankID, documentID), nil
}

func documentPath(bankID, documentID string) string {
	return fmt.Sprintf("/v1/default/banks/%s/documents/%s", url.PathEscape(bankID), url.PathEscape(documentID))
}

func documentResponseFromRaw(raw map[string]any) *DocumentResponse {
	docRaw := raw
	if nested, ok := raw["document"].(map[string]any); ok {
		docRaw = nested
	}
	doc := &DocumentResponse{
		DocumentID: stringFromMap(docRaw, "document_id"),
		ID:         stringFromMap(docRaw, "id"),
		Tags:       stringsFromAny(docRaw["tags"]),
		Metadata:   mapFromAny(docRaw["metadata"]),
		Raw:        raw,
	}
	if doc.DocumentID == "" {
		doc.DocumentID = stringFromMap(raw, "document_id")
	}
	if len(doc.Tags) == 0 {
		doc.Tags = stringsFromAny(raw["tags"])
	}
	if doc.Metadata == nil {
		doc.Metadata = mapFromAny(raw["metadata"])
	}
	return doc
}

func deleteDocumentResponseFromRaw(raw map[string]any, bankID, documentID string) *DeleteDocumentResponse {
	resp := &DeleteDocumentResponse{BankID: bankID, DocumentID: documentID, Deleted: true, Raw: raw}
	if raw == nil {
		return resp
	}
	if value := stringFromMap(raw, "bank_id"); value != "" {
		resp.BankID = value
	}
	if value := stringFromMap(raw, "document_id"); value != "" {
		resp.DocumentID = value
	}
	if value, ok := raw["deleted"].(bool); ok {
		resp.Deleted = value
	} else if value, ok := raw["success"].(bool); ok {
		resp.Deleted = value
	}
	resp.MemoryUnitsDeleted = intFromMap(raw, "memory_units_deleted", "deleted_memory_units", "deleted_memories", "memory_units_count")
	return resp
}

func stringFromMap(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, _ := values[key].(string)
	return value
}

func intFromMap(values map[string]any, keys ...string) int {
	for _, key := range keys {
		switch value := values[key].(type) {
		case float64:
			return int(value)
		case int:
			return value
		case json.Number:
			i, _ := value.Int64()
			return int(i)
		}
	}
	return 0
}

func stringsFromAny(value any) []string {
	if tags, ok := value.([]any); ok {
		out := make([]string, 0, len(tags))
		for _, tag := range tags {
			if s, ok := tag.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	tags, _ := value.([]string)
	return tags
}

func mapFromAny(value any) map[string]any {
	m, _ := value.(map[string]any)
	return m
}
