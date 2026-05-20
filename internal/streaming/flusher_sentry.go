package streaming

import (
	"context"
	"fmt"

	"github.com/usehivy/hivy/internal/logging"
)

func captureTimelineFlush(ctx context.Context, stage string, convID string, err error, fields map[string]any) {
	if err == nil {
		return
	}
	if fields == nil {
		fields = make(map[string]any)
	}
	fields["stage"] = stage
	if convID != "" {
		fields["conversation_id"] = convID
	}
	logging.CaptureWithFields(ctx, fmt.Errorf("timeline flusher %s: %w", stage, err), fields)
}
