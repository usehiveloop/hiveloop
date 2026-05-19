package hindsight

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// MemoryRefreshFunc is called after a destructive memory change so employee
// runtimes can reload their precomputed memory context.
type MemoryRefreshFunc func(ctx context.Context, agent *model.Agent)

// NewMemoryToolsFunc returns a callback compatible with mcpserver.MemoryToolsFunc.
// Designed to be passed to mcpserver.BuildServer to avoid import cycles.
func NewMemoryToolsFunc(client *Client, refreshFns ...MemoryRefreshFunc) func(server *mcp.Server, agentID string, db *gorm.DB) {
	var refresh MemoryRefreshFunc
	if len(refreshFns) > 0 {
		refresh = refreshFns[0]
	}
	return func(server *mcp.Server, agentID string, db *gorm.DB) {
		var agent model.Agent
		if err := db.Where("id = ?", agentID).First(&agent).Error; err != nil {
			return
		}
		AddMemoryTools(server, &agent, client, db, refresh)
	}
}

// AddMemoryTools registers memory tools on an existing MCP server. Memory is
// scoped per org.
func AddMemoryTools(server *mcp.Server, agent *model.Agent, client *Client, db *gorm.DB, refresh MemoryRefreshFunc) {
	if agent.OrgID == nil || client == nil {
		return
	}
	bankID := OrgBankID(*agent.OrgID)
	memoryTags := baseMemoryTags(agent, "manual")
	tagGroups := recallTagGroups(agent)

	addRecallTool(server, client, bankID, tagGroups)
	addRetainTool(server, agent, client, bankID, memoryTags)
	addForgetTool(server, agent, client, db, bankID, refresh)
	addReflectTool(server, client, bankID, tagGroups)
}

func baseMemoryTags(agent *model.Agent, source string) []string {
	if agent == nil || agent.OrgID == nil {
		return nil
	}
	if source == "" {
		source = "manual"
	}
	tags := []string{
		"company:" + agent.OrgID.String(),
		"source:" + source,
		"visibility:company",
	}
	return tags
}

func recallTagGroups(agent *model.Agent) []any {
	if agent == nil || agent.OrgID == nil {
		return nil
	}
	tags := []string{"company:" + agent.OrgID.String()}
	return []any{map[string]any{"tags": tags, "match": "all_strict"}}
}

func toolError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: %s", msg)}},
		IsError: true,
	}
}

func toolJSON(v any) (*mcp.CallToolResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return toolError("failed to serialize response"), nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(b)}},
	}, nil
}
