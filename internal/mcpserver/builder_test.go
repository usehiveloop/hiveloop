package mcpserver

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/model"
)

func TestBuildServerRegistersSpecialistToolsOnlyForEmployeeRuntimeTokens(t *testing.T) {
	for _, tc := range []struct {
		name      string
		meta      model.JSON
		wantCalls int
	}{
		{
			name: "employee runtime",
			meta: model.JSON{
				model.TokenMetaType:        model.TokenTypeEmployeeProxy,
				model.TokenMetaRuntimeMode: model.TokenRuntimeModeEmployee,
			},
			wantCalls: 1,
		},
		{
			name: "specialist runtime",
			meta: model.JSON{
				model.TokenMetaType:        model.TokenTypeEmployeeProxy,
				model.TokenMetaRuntimeMode: model.TokenRuntimeModeSpecialist,
			},
			wantCalls: 0,
		},
		{
			name: "missing runtime mode",
			meta: model.JSON{
				model.TokenMetaType: model.TokenTypeEmployeeProxy,
			},
			wantCalls: 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			_, err := BuildServer(
				context.Background(),
				&model.Token{Meta: tc.meta},
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				func(server *mcp.Server, token *model.Token) {
					calls++
				},
			)
			if err != nil {
				t.Fatalf("build server: %v", err)
			}
			if calls != tc.wantCalls {
				t.Fatalf("specialist tool registrations = %d, want %d", calls, tc.wantCalls)
			}
		})
	}
}
