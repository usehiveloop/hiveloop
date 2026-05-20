package hivy

import (
	"encoding/json"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/model"
)

func testConnections() []ConnectionWithActions {
	return []ConnectionWithActions{
		{
			Connection: model.InConnection{ID: uuid.MustParse("cccccccc-0000-0000-0000-000000000001")},
			Provider:   "github-app",
			ReadActions: map[string]catalog.ActionDef{
				"pulls_get": {
					DisplayName:    "Get Pull Request",
					Description:    "Get a single pull request by number",
					Access:         "read",
					Parameters:     json.RawMessage(`{"type":"object","properties":{"owner":{"type":"string","description":"Repository owner"},"repo":{"type":"string","description":"Repository name"},"pull_number":{"type":"integer","description":"PR number"}},"required":["owner","repo","pull_number"]}`),
					ResponseSchema: "pull_request",
				},
				"pulls_get_diff": {
					DisplayName: "Get PR Diff",
					Description: "Get the diff for a pull request",
					Access:      "read",
					Parameters:  json.RawMessage(`{"type":"object","properties":{"owner":{"type":"string","description":"Repository owner"},"repo":{"type":"string","description":"Repository name"},"pull_number":{"type":"integer","description":"PR number"}},"required":["owner","repo","pull_number"]}`),
				},
			},
		},
		{
			Connection: model.InConnection{ID: uuid.MustParse("cccccccc-0000-0000-0000-000000000002")},
			Provider:   "slack",
			ReadActions: map[string]catalog.ActionDef{
				"conversations_replies": {
					DisplayName: "Get Thread Replies",
					Description: "Fetch all replies in a Slack thread",
					Access:      "read",
					Parameters:  json.RawMessage(`{"type":"object","properties":{"channel":{"type":"string","description":"Channel ID"},"ts":{"type":"string","description":"Thread timestamp"}},"required":["channel","ts"]}`),
				},
			},
		},
	}
}

func marshalArgs(args any) json.RawMessage {
	data, _ := json.Marshal(args)
	return data
}
