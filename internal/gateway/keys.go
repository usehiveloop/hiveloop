package gateway

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func stableConversationID(routeID uuid.UUID, threadKey string) string {
	sum := sha256.Sum256([]byte(routeID.String() + ":" + threadKey))
	return "gateway-" + hex.EncodeToString(sum[:])[:32]
}

func runtimeSessionID(conversationID string) string {
	return "http-" + conversationID
}

func outboundDedupeKey(response AgentResponse) string {
	if response.TurnID != "" {
		return response.RuntimeSessionID + ":" + response.TurnID
	}
	sum := sha256.Sum256([]byte(response.RuntimeSessionID + ":" + response.Text))
	return response.RuntimeSessionID + ":" + hex.EncodeToString(sum[:])[:16]
}

func rawJSON(value any, fallback string) model.RawJSON {
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) == 0 {
		return model.RawJSON(fallback)
	}
	return model.RawJSON(encoded)
}

func handlesJSON(handles []MessageHandle) model.RawJSON {
	if handles == nil {
		handles = []MessageHandle{}
	}
	return rawJSON(handles, "[]")
}
