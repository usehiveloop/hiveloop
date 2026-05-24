package enrichment

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/mcpserver"
	"github.com/usehivy/hivy/internal/trigger/hivy"
)

type fetchResultEntry struct {
	Action string
	Result string
}

func (agent *EnrichmentAgent) newFetchHandler(
	ctx context.Context,
	orgID uuid.UUID,
	connMap map[string]hivy.ConnectionWithActions,
	fetchResults *[]fetchResultEntry,
	fetchCount *int,
	_ *slog.Logger,
) hivy.ToolHandler {
	return func(_ context.Context, _ string, raw json.RawMessage) (string, bool, error) {
		var args struct {
			ConnectionID string         `json:"connection_id"`
			Action       string         `json:"action"`
			Params       map[string]any `json:"params"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return "", false, fmt.Errorf("invalid arguments: %w", err)
		}

		conn, ok := connMap[args.ConnectionID]
		if !ok {
			var available []string
			for connID, connEntry := range connMap {
				available = append(available, fmt.Sprintf("%s (%s)", connID, connEntry.Provider))
			}
			return "", false, fmt.Errorf("connection %q not found. Available: %s", args.ConnectionID, strings.Join(available, ", "))
		}

		actionDef, actionExists := conn.ReadActions[args.Action]
		if !actionExists {
			var available []string
			for actionKey := range conn.ReadActions {
				available = append(available, actionKey)
			}
			return "", false, fmt.Errorf("action %q not found for %s. Available: %s", args.Action, conn.Provider, strings.Join(available, ", "))
		}

		providerCfgKey := fmt.Sprintf("%s_%s", orgID.String(), conn.Connection.Integration.UniqueKey)
		nangoConnID := conn.Connection.NangoConnectionID

		result, err := mcpserver.ExecuteAction(
			ctx,
			agent.nangoClient,
			conn.Provider,
			providerCfgKey,
			nangoConnID,
			&actionDef,
			args.Params,
			nil,
		)
		if err != nil {
			return fmt.Sprintf("Fetch failed: %s", err.Error()), false, nil
		}

		resultJSON, _ := json.Marshal(result)
		resultStr := truncateString(string(resultJSON), 4000)

		*fetchResults = append(*fetchResults, fetchResultEntry{Action: args.Action, Result: resultStr})
		*fetchCount++

		return resultStr, false, nil
	}
}
