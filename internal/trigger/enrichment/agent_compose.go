package enrichment

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/usehiveloop/hiveloop/internal/trigger/hiveloop"
)

func newComposeHandler(composedMessage *string, _ *slog.Logger) hiveloop.ToolHandler {
	return func(_ context.Context, _ string, raw json.RawMessage) (string, bool, error) {
		var args struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return "", false, fmt.Errorf("invalid arguments: %w", err)
		}
		if args.Message == "" {
			return "", false, fmt.Errorf("message is required")
		}
		*composedMessage = args.Message

		return "Message composed.", true, nil
	}
}
