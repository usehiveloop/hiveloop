package evals

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const DefaultJudgeModel = "gpt-4o-mini"

type BehaviorJudgement struct {
	Behavior   string  `json:"behavior"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
	Model      string  `json:"model,omitempty"`
}

type Judge struct {
	client *http.Client
	model  string
}

func NewJudge(model string) *Judge {
	return &Judge{
		client: &http.Client{Timeout: 20 * time.Second},
		model:  judgeModel(model),
	}
}

func judgeModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return DefaultJudgeModel
	}
	return model
}

func (j *Judge) ClassifyFinalText(ctx context.Context, proxyBaseURL, proxyToken string, c Case, finalText string) (*BehaviorJudgement, error) {
	if j == nil {
		return nil, nil
	}
	proxyBaseURL = strings.TrimRight(strings.TrimSpace(proxyBaseURL), "/")
	proxyToken = strings.TrimSpace(proxyToken)
	finalText = strings.TrimSpace(finalText)
	if proxyBaseURL == "" || proxyToken == "" || finalText == "" {
		return nil, nil
	}
	body, err := json.Marshal(map[string]any{
		"model":       j.model,
		"temperature": 0,
		"max_tokens":  120,
		"messages": []map[string]string{
			{
				"role": "system",
				"content": strings.Join([]string{
					"You are an eval judge.",
					"Classify the assistant's final response as direct or clarify.",
					"direct means it answers or proceeds without needing more user input.",
					"clarify means it asks for missing information before it can proceed.",
					"Return only JSON that matches the schema.",
				}, " "),
			},
			{
				"role": "user",
				"content": fmt.Sprintf("User message:\n%s\n\nExpected behavior:\n%s\n\nAssistant final response:\n%s",
					strings.TrimSpace(c.Message),
					strings.TrimSpace(c.ExpectedBehavior),
					finalText,
				),
			},
		},
		"response_format": map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "eval_behavior_judgement",
				"strict": true,
				"schema": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"behavior", "confidence", "reason"},
					"properties": map[string]any{
						"behavior":   map[string]any{"type": "string", "enum": []string{BehaviorDirect, BehaviorClarify}},
						"confidence": map[string]any{"type": "number", "minimum": 0, "maximum": 1},
						"reason":     map[string]any{"type": "string"},
					},
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, proxyBaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+proxyToken)
	resp, err := j.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("judge request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var payload map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&payload)
		return nil, fmt.Errorf("judge status %d: %v", resp.StatusCode, payload)
	}
	var completion struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&completion); err != nil {
		return nil, fmt.Errorf("decode judge completion: %w", err)
	}
	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf("judge returned no choices")
	}
	content := strings.TrimSpace(completion.Choices[0].Message.Content)
	if content == "" {
		return nil, fmt.Errorf("judge returned empty content for model %q", j.model)
	}
	var judgement BehaviorJudgement
	if err := json.Unmarshal([]byte(content), &judgement); err != nil {
		return nil, fmt.Errorf("decode judge JSON: %w", err)
	}
	if judgement.Behavior != BehaviorDirect && judgement.Behavior != BehaviorClarify {
		return nil, fmt.Errorf("judge returned invalid behavior %q", judgement.Behavior)
	}
	judgement.Model = j.model
	return &judgement, nil
}

func (j *Judge) GenerateFollowUp(ctx context.Context, proxyBaseURL, proxyToken string, c Case, assistantText string) (string, error) {
	if j == nil || c.FollowUp == nil {
		return "", nil
	}
	proxyBaseURL = strings.TrimRight(strings.TrimSpace(proxyBaseURL), "/")
	proxyToken = strings.TrimSpace(proxyToken)
	if proxyBaseURL == "" || proxyToken == "" {
		return "", nil
	}
	contextText := strings.TrimSpace(c.FollowUp.Context)
	if strings.TrimSpace(c.FollowUp.Mode) == "static" {
		return contextText, nil
	}
	body, err := json.Marshal(map[string]any{
		"model":       j.model,
		"temperature": 0,
		"max_tokens":  180,
		"messages": []map[string]string{
			{
				"role": "system",
				"content": strings.Join([]string{
					"You are simulating the human user in an eval.",
					"Reply naturally to the assistant's clarification request.",
					"Use only the provided user context.",
					"Give enough detail for the assistant to proceed.",
					"Return only JSON that matches the schema.",
				}, " "),
			},
			{
				"role": "user",
				"content": fmt.Sprintf("Original user message:\n%s\n\nAssistant clarification:\n%s\n\nUser context to include:\n%s",
					strings.TrimSpace(c.Message),
					strings.TrimSpace(assistantText),
					contextText,
				),
			},
		},
		"response_format": map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "eval_follow_up",
				"strict": true,
				"schema": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"reply"},
					"properties": map[string]any{
						"reply": map[string]any{"type": "string"},
					},
				},
			},
		},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, proxyBaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+proxyToken)
	resp, err := j.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("follow-up judge request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var payload map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&payload)
		return "", fmt.Errorf("follow-up judge status %d: %v", resp.StatusCode, payload)
	}
	var completion struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&completion); err != nil {
		return "", fmt.Errorf("decode follow-up completion: %w", err)
	}
	if len(completion.Choices) == 0 {
		return "", fmt.Errorf("follow-up judge returned no choices")
	}
	var payload struct {
		Reply string `json:"reply"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(completion.Choices[0].Message.Content)), &payload); err != nil {
		return "", fmt.Errorf("decode follow-up JSON: %w", err)
	}
	if strings.TrimSpace(payload.Reply) == "" {
		return "", fmt.Errorf("follow-up judge returned empty reply")
	}
	return strings.TrimSpace(payload.Reply), nil
}

func proxyBaseURL(apiURL string) string {
	apiURL = strings.TrimRight(strings.TrimSpace(apiURL), "/")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}
	return apiURL + "/v1/proxy/v1"
}
