package tasks

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/pkoukk/tiktoken-go"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/qdrant"
)

// 8192 - 2048 safety margin. Same encoding (cl100k_base) for every run,
// so a doc of N tokens always splits into the same K parts at the same
// boundaries; reconciliation can compare part counts deterministically.
const embedTokenLimit = 6000

func flushBatch(
	ctx context.Context,
	deps *Deps,
	src *ragmodel.RAGSource,
	attempt *ragmodel.RAGIndexAttempt,
	docs []interfaces.Document,
) error {
	if len(docs) == 0 {
		return nil
	}
	parts := make([]docPart, 0, len(docs))
	docIDs := make([]string, 0, len(docs))
	seen := make(map[string]struct{}, len(docs))
	for i := range docs {
		ps, err := splitDocument(&docs[i])
		if err != nil {
			return fmt.Errorf("ingest: split %s: %w", docs[i].DocID, err)
		}
		parts = append(parts, ps...)
		if _, ok := seen[docs[i].DocID]; !ok {
			seen[docs[i].DocID] = struct{}{}
			docIDs = append(docIDs, docs[i].DocID)
		}
	}
	if len(parts) == 0 {
		return nil
	}

	if err := deps.Qdrant.DeleteByDocIDs(ctx, deps.Collection, src.ID.String(), docIDs); err != nil {
		return fmt.Errorf("ingest: clear stale parts (%d docs): %w", len(docIDs), err)
	}

	inputs := make([]string, len(parts))
	for i := range parts {
		inputs[i] = parts[i].content
	}
	vectors, err := deps.Embedder.Embed(ctx, inputs)
	if err != nil {
		return fmt.Errorf("ingest: embed (%d parts from %d docs): %w", len(parts), len(docs), err)
	}

	points := make([]qdrant.Point, 0, len(parts))
	orgID := src.OrgIDValue.String()
	sourceID := src.ID.String()
	for i := range parts {
		points = append(points, qdrant.Point{
			ID:      qdrant.PointID(orgID, sourceID, parts[i].pointDocID),
			Vector:  vectors[i],
			Payload: buildPayload(src, parts[i].doc, parts[i].content, parts[i].partIndex),
		})
	}
	if err := deps.Qdrant.Upsert(ctx, deps.Collection, points, true); err != nil {
		return fmt.Errorf("ingest: qdrant upsert (%d points): %w", len(points), err)
	}
	return nil
}

type docPart struct {
	doc        *interfaces.Document
	pointDocID string // <doc.DocID>-part-N — always suffixed for stability across re-runs
	content    string
	partIndex  int
}

func splitDocument(d *interfaces.Document) ([]docPart, error) {
	full := renderContent(d)
	if full == "" {
		return nil, nil
	}
	chunks, err := splitByTokens(full, embedTokenLimit)
	if err != nil {
		return nil, err
	}
	out := make([]docPart, len(chunks))
	for i, c := range chunks {
		out[i] = docPart{
			doc:        d,
			pointDocID: fmt.Sprintf("%s-part-%d", d.DocID, i),
			content:    c,
			partIndex:  i,
		}
	}
	return out, nil
}

var (
	tokenizerOnce sync.Once
	tokenizer     *tiktoken.Tiktoken
	tokenizerErr  error
)

// getTokenizer returns the cl100k_base encoder used by every recent
// OpenAI embedding model. Lazy + cached because BPE table init is
// non-trivial; fixed encoding makes splits deterministic.
func getTokenizer() (*tiktoken.Tiktoken, error) {
	tokenizerOnce.Do(func() {
		tokenizer, tokenizerErr = tiktoken.GetEncoding("cl100k_base")
	})
	return tokenizer, tokenizerErr
}

func splitByTokens(s string, maxTokens int) ([]string, error) {
	tk, err := getTokenizer()
	if err != nil {
		return nil, fmt.Errorf("tiktoken init: %w", err)
	}
	tokens := tk.Encode(s, nil, nil)
	if len(tokens) <= maxTokens {
		return []string{s}, nil
	}
	out := make([]string, 0, (len(tokens)+maxTokens-1)/maxTokens)
	for i := 0; i < len(tokens); i += maxTokens {
		end := i + maxTokens
		if end > len(tokens) {
			end = len(tokens)
		}
		out = append(out, tk.Decode(tokens[i:end]))
	}
	return out, nil
}

func renderContent(d *interfaces.Document) string {
	parts := make([]string, 0, len(d.Sections)+1)
	if d.SemanticID != "" {
		parts = append(parts, d.SemanticID)
	}
	for i := range d.Sections {
		s := &d.Sections[i]
		if s.Title != "" {
			parts = append(parts, s.Title)
		}
		if s.Text != "" {
			parts = append(parts, s.Text)
		}
	}
	return strings.Join(parts, "\n\n")
}

func buildPayload(src *ragmodel.RAGSource, d *interfaces.Document, content string, partIndex int) map[string]any {
	pl := map[string]any{
		"org_id":         src.OrgIDValue.String(),
		"rag_source_id":  src.ID.String(),
		"doc_id":         d.DocID,
		"part_index":     partIndex,
		"semantic_id":    d.SemanticID,
		"link":           d.Link,
		"acl":            append([]string(nil), d.ACL...),
		"is_public":      d.IsPublic,
		"content":        content,
		"primary_owners": append([]string(nil), d.PrimaryOwners...),
	}
	if d.DocUpdatedAt != nil {
		pl["doc_updated_at"] = d.DocUpdatedAt.Unix()
	}
	if d.Metadata != nil {
		pl["metadata"] = d.Metadata
	}
	return pl
}
