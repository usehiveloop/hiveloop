package hindsight

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

type memoryForgetToolResponse struct {
	BankID             string `json:"bank_id"`
	DocumentID         string `json:"document_id"`
	Deleted            bool   `json:"deleted"`
	MemoryUnitsDeleted int    `json:"memory_units_deleted"`
	Message            string `json:"message"`
}

func addForgetTool(server *mcp.Server, agent *model.Agent, client *Client, db *gorm.DB, bankID string, refresh MemoryRefreshFunc) {
	server.AddTool(
		&mcp.Tool{
			Name:        "memory_forget",
			Description: "Delete exactly one long-term memory document by exact document_id. This is immediate, document-level deletion and cannot be undone. Use only when the user explicitly asks to forget a specific retained memory and you have the exact document_id.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"document_id": map[string]any{
						"type":        "string",
						"description": "The exact Hindsight document_id to delete, such as a document_id returned by memory_retain or memory_recall.",
					},
					"reason": map[string]any{
						"type":        "string",
						"description": "Optional brief reason for audit metadata. Raw reason text is not stored.",
					},
				},
				"required": []string{"document_id"},
			},
		},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var params struct {
				DocumentID string `json:"document_id"`
				Reason     string `json:"reason"`
			}
			if req.Params.Arguments != nil {
				_ = json.Unmarshal(req.Params.Arguments, &params)
			}
			params.DocumentID = strings.TrimSpace(params.DocumentID)
			if params.DocumentID == "" {
				return toolError("document_id is required"), nil
			}

			if doc, err := client.GetDocument(ctx, bankID, params.DocumentID); err == nil {
				if !documentAllowedForAgent(doc, agent) {
					return toolError("document is outside this agent's memory scope"), nil
				}
			}

			deleted, err := client.DeleteDocument(ctx, bankID, params.DocumentID)
			if err != nil {
				return toolError("memory forget failed: " + err.Error()), nil
			}
			writeMemoryForgetAudit(ctx, db, agent, bankID, params.DocumentID, params.Reason, deleted)
			if refresh != nil && deleted.Deleted {
				refresh(ctx, agent)
			}

			return toolJSON(memoryForgetResponse(bankID, params.DocumentID, deleted))
		},
	)
}

func documentAllowedForAgent(doc *DocumentResponse, agent *model.Agent) bool {
	if doc == nil || agent == nil || agent.OrgID == nil || len(doc.Tags) == 0 {
		return true
	}
	tags := map[string]struct{}{}
	for _, tag := range doc.Tags {
		tags[tag] = struct{}{}
	}
	if _, ok := tags["company:"+agent.OrgID.String()]; !ok {
		return false
	}
	return true
}

func writeMemoryForgetAudit(ctx context.Context, db *gorm.DB, agent *model.Agent, bankID, documentID, reason string, deleted *DeleteDocumentResponse) {
	if db == nil || agent == nil || agent.OrgID == nil {
		return
	}
	meta := model.JSON{
		"agent_id":             agent.ID.String(),
		"bank_id":              bankID,
		"document_id":          documentID,
		"deleted":              deleted != nil && deleted.Deleted,
		"memory_units_deleted": 0,
	}
	if deleted != nil {
		meta["memory_units_deleted"] = deleted.MemoryUnitsDeleted
	}
	if strings.TrimSpace(reason) != "" {
		meta["reason_sha256"] = sha256Hex(reason)
	}
	_ = db.WithContext(ctx).Create(&model.AuditEntry{
		OrgID:     *agent.OrgID,
		Action:    "memory.forget",
		Metadata:  meta,
		CreatedAt: time.Now().UTC(),
	}).Error
}

func memoryForgetResponse(bankID, documentID string, result *DeleteDocumentResponse) memoryForgetToolResponse {
	out := memoryForgetToolResponse{
		BankID:     bankID,
		DocumentID: documentID,
		Deleted:    true,
		Message:    "Memory document deleted.",
	}
	if result == nil {
		return out
	}
	out.Deleted = result.Deleted
	out.MemoryUnitsDeleted = result.MemoryUnitsDeleted
	if result.BankID != "" {
		out.BankID = result.BankID
	}
	if result.DocumentID != "" {
		out.DocumentID = result.DocumentID
	}
	if !out.Deleted {
		out.Message = "Memory document delete request completed, but Hindsight did not report deletion."
	}
	return out
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}
