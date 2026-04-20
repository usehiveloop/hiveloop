package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/cache"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/registry"
	"github.com/usehiveloop/hiveloop/internal/trigger/zira"
)

// messageContentMaxBytes caps the first-message content we send to the naming
// model. The naming prompt doesn't need the full webhook payload — the first
// couple KB almost always contain the signal (subject, headline, etc.).
const messageContentMaxBytes = 2048

// ConversationNameHandler generates a short title for a conversation by
// calling the cheapest model available on the conversation's credential
// provider. It's idempotent: if the conversation already has a name, the
// handler returns nil without making any external calls.
type ConversationNameHandler struct {
	db           *gorm.DB
	cacheManager *cache.Manager
}

// NewConversationNameHandler constructs a handler. Returns nil if the cache
// manager is nil — the handler requires credential decryption.
func NewConversationNameHandler(db *gorm.DB, cacheManager *cache.Manager) *ConversationNameHandler {
	if db == nil || cacheManager == nil {
		return nil
	}
	return &ConversationNameHandler{db: db, cacheManager: cacheManager}
}

// Handle runs one naming job. On retryable failures (provider timeouts,
// network) it returns an error so Asynq retries per MaxRetry. On permanent
// failures (missing credential, no supported model) it returns nil so the
// job is dropped — the conversation simply stays unnamed, and the frontend
// falls back to deriving a title from the first message.
func (handler *ConversationNameHandler) Handle(ctx context.Context, task *asynq.Task) error {
	var payload ConversationNamePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	var conv model.AgentConversation
	if err := handler.db.WithContext(ctx).
		Where("id = ?", payload.ConversationID).
		First(&conv).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			slog.Info("conversation naming: conversation gone, skipping",
				"conversation_id", payload.ConversationID)
			return nil
		}
		return fmt.Errorf("load conversation: %w", err)
	}

	if conv.Name != "" {
		return nil
	}
	if conv.CredentialID == nil {
		slog.Info("conversation naming: no credential, skipping",
			"conversation_id", conv.ID)
		return nil
	}

	var credential model.Credential
	if err := handler.db.WithContext(ctx).
		Where("id = ? AND org_id = ?", *conv.CredentialID, conv.OrgID).
		First(&credential).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			slog.Info("conversation naming: credential gone, skipping",
				"conversation_id", conv.ID, "credential_id", *conv.CredentialID)
			return nil
		}
		return fmt.Errorf("load credential: %w", err)
	}

	modelID, supportsTools, ok := pickCheapestModel(credential.ProviderID)
	if !ok {
		slog.Info("conversation naming: no model available for provider, skipping",
			"conversation_id", conv.ID, "provider", credential.ProviderID)
		return nil
	}

	firstMessage, err := loadFirstMessageContent(ctx, handler.db, conv.ID)
	if err != nil {
		return fmt.Errorf("load first message: %w", err)
	}
	if firstMessage == "" {
		slog.Info("conversation naming: first message not yet persisted, retrying",
			"conversation_id", conv.ID)
		// Asynq will retry. Webhook ordering means message_received should be
		// visible by the time this runs, but a redelivery edge case is possible.
		return fmt.Errorf("first message not yet available")
	}

	decrypted, err := handler.cacheManager.GetDecryptedCredential(
		ctx, credential.ID.String(), conv.OrgID)
	if err != nil {
		return fmt.Errorf("decrypt credential: %w", err)
	}

	client := zira.NewCompletionClient(&credential, string(decrypted.APIKey))
	title, err := generateConversationTitle(ctx, client, modelID, supportsTools, firstMessage)
	if err != nil {
		return fmt.Errorf("generate title: %w", err)
	}
	title = cleanTitle(title)
	if title == "" {
		slog.Info("conversation naming: model returned empty title, skipping",
			"conversation_id", conv.ID, "model", modelID)
		return nil
	}

	// Only write if still empty — protects a human rename that raced with us.
	result := handler.db.WithContext(ctx).
		Model(&model.AgentConversation{}).
		Where("id = ? AND (name IS NULL OR name = '')", conv.ID).
		Update("name", title)
	if result.Error != nil {
		return fmt.Errorf("update name: %w", result.Error)
	}

	slog.Info("conversation named",
		"conversation_id", conv.ID,
		"model", modelID,
		"title", title,
		"rows_updated", result.RowsAffected,
	)
	return nil
}

// pickCheapestModel returns the cheapest input-cost model from the given
// provider's curated catalog, along with whether that model supports tool
// calls (used for structured output forcing). Models without cost metadata
// are skipped — we can't reason about them.
func pickCheapestModel(providerID string) (string, bool, bool) {
	provider, ok := registry.Global().GetProvider(providerID)
	if !ok {
		return "", false, false
	}

	var cheapestID string
	var cheapestCost float64 = -1
	var cheapestSupportsTools bool

	for id, m := range provider.Models {
		if m.Cost == nil {
			continue
		}
		if m.Status == "deprecated" || m.Status == "retired" {
			continue
		}
		if cheapestCost < 0 || m.Cost.Input < cheapestCost {
			cheapestID = id
			cheapestCost = m.Cost.Input
			cheapestSupportsTools = m.ToolCall
		}
	}
	if cheapestID == "" {
		return "", false, false
	}
	return cheapestID, cheapestSupportsTools, true
}

// loadFirstMessageContent returns the `content` field of the earliest
// `message_received` event for the conversation, truncated.
func loadFirstMessageContent(ctx context.Context, db *gorm.DB, conversationID any) (string, error) {
	var event model.ConversationEvent
	err := db.WithContext(ctx).
		Where("conversation_id = ? AND event_type = ?", conversationID, "message_received").
		Order("sequence_number ASC").
		Limit(1).
		First(&event).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", nil
		}
		return "", err
	}

	var data struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return "", fmt.Errorf("decode event data: %w", err)
	}
	content := data.Content
	if len(content) > messageContentMaxBytes {
		content = content[:messageContentMaxBytes]
	}
	return content, nil
}

// generateConversationTitle calls the LLM and extracts a short title.
// When the model supports tool calls, we force a `submit_title` tool call
// for structured output. Otherwise we fall back to free-form text and parse.
func generateConversationTitle(
	ctx context.Context,
	client zira.CompletionClient,
	modelID string,
	supportsTools bool,
	firstMessage string,
) (string, error) {
	systemPrompt := "You generate concise conversation titles. Given the user's first message, return a 3–6 word title summarising what the conversation is about. Use title case. No punctuation at the end. Do not wrap in quotes."
	userPrompt := "First message:\n\n" + firstMessage

	req := zira.CompletionRequest{
		Model: modelID,
		Messages: []zira.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		MaxTokens: 48,
	}

	if supportsTools {
		req.Tools = []zira.ToolDef{{
			Name:        "submit_title",
			Description: "Submit the final conversation title.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"title": {
						"type": "string",
						"description": "A 3–6 word title in title case, no trailing punctuation."
					}
				},
				"required": ["title"]
			}`),
		}}
		req.ToolChoice = "required"
	}

	resp, err := client.ChatCompletion(ctx, req)
	if err != nil {
		return "", err
	}

	if supportsTools && len(resp.Message.ToolCalls) > 0 {
		var args struct {
			Title string `json:"title"`
		}
		if err := json.Unmarshal([]byte(resp.Message.ToolCalls[0].Arguments), &args); err != nil {
			return "", fmt.Errorf("decode tool args: %w", err)
		}
		return args.Title, nil
	}
	return resp.Message.Content, nil
}

// cleanTitle strips quotes, leading/trailing whitespace, and trailing
// punctuation that models commonly append.
func cleanTitle(raw string) string {
	title := strings.TrimSpace(raw)
	title = strings.Trim(title, `"'“”‘’`)
	title = strings.TrimSpace(title)
	title = strings.TrimRight(title, ".!?,:;")
	// Collapse newlines — the model is supposed to return a single line, but
	// fall-through to the first non-empty line if it didn't.
	if idx := strings.IndexByte(title, '\n'); idx >= 0 {
		title = strings.TrimSpace(title[:idx])
	}
	// Guard against runaway output.
	const maxTitleLen = 120
	if len(title) > maxTitleLen {
		title = title[:maxTitleLen]
	}
	return title
}
