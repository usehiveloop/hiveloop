package enrichment

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/mcpserver"
	"github.com/usehiveloop/hiveloop/internal/trigger/hiveloop"
)

type fetchResultEntry struct {
	Action string
	Result string
}

func (agent *EnrichmentAgent) newFetchHandler(
	ctx context.Context,
	orgID uuid.UUID,
	connMap map[string]hiveloop.ConnectionWithActions,
	fetchResults *[]fetchResultEntry,
	fetchCount *int,
	logger *slog.Logger,
) hiveloop.ToolHandler {
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
			logger.Warn("enrichment: fetch connection not found",
				"requested_conn_id", args.ConnectionID,
				"available", strings.Join(available, ", "),
			)
			return "", false, fmt.Errorf("connection %q not found. Available: %s", args.ConnectionID, strings.Join(available, ", "))
		}

		actionDef, actionExists := conn.ReadActions[args.Action]
		if !actionExists {
			var available []string
			for actionKey := range conn.ReadActions {
				available = append(available, actionKey)
			}
			logger.Warn("enrichment: fetch action not found",
				"provider", conn.Provider,
				"requested_action", args.Action,
				"available", strings.Join(available, ", "),
			)
			return "", false, fmt.Errorf("action %q not found for %s. Available: %s", args.Action, conn.Provider, strings.Join(available, ", "))
		}

		paramsJSON, _ := json.Marshal(args.Params)
		logger.Info("enrichment: fetch executing",
			"provider", conn.Provider,
			"action", args.Action,
			"conn_id", args.ConnectionID,
			"params", string(paramsJSON),
		)

		providerCfgKey := fmt.Sprintf("%s_%s", orgID.String(), conn.Connection.InIntegration.UniqueKey)
		nangoConnID := conn.Connection.NangoConnectionID

		fetchStart := time.Now()
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
		fetchLatency := time.Since(fetchStart).Milliseconds()

		if err != nil {
			logger.Warn("enrichment: fetch failed",
				"provider", conn.Provider,
				"action", args.Action,
				"error", err,
				"fetch_latency_ms", fetchLatency,
			)
			return fmt.Sprintf("Fetch failed: %s", err.Error()), false, nil
		}

		resultJSON, _ := json.Marshal(result)
		resultStr := truncateString(string(resultJSON), 4000)

		logger.Info("enrichment: fetch success",
			"provider", conn.Provider,
			"action", args.Action,
			"response_bytes", len(resultJSON),
			"truncated_bytes", len(resultStr),
			"fetch_latency_ms", fetchLatency,
		)

		*fetchResults = append(*fetchResults, fetchResultEntry{Action: args.Action, Result: resultStr})
		*fetchCount++

		return resultStr, false, nil
	}
}
