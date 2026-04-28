package main

import "strings"

type githubWebhookReq struct {
	ConnectionID      string `json:"connection_id"`
	ProviderConfigKey string `json:"provider_config_key"`
	EventType         string `json:"event_type"`
	Action            string `json:"action,omitempty"`
	Payload           any    `json:"payload,omitempty"`
	Target            string `json:"target,omitempty"`
}

func buildGitHubForward(req githubWebhookReq) (forwardWebhook, map[string]string) {
	payload := req.Payload
	if payload == nil {
		payload = defaultGitHubPayload(req.EventType, req.Action)
	}
	if m, ok := payload.(map[string]any); ok && req.Action != "" {
		if _, has := m["action"]; !has {
			m["action"] = req.Action
		}
	}
	body := forwardWebhook{
		From:              "github",
		Type:              "forward",
		ConnectionID:      req.ConnectionID,
		ProviderConfigKey: req.ProviderConfigKey,
		Payload:           payload,
	}
	headers := map[string]string{
		"X-GitHub-Event":    req.EventType,
		"X-GitHub-Delivery": newID(),
	}
	return body, headers
}

func defaultGitHubPayload(eventType, action string) map[string]any {
	repo := map[string]any{
		"id":        1,
		"name":      "test-repo",
		"full_name": "agent/test-repo",
		"private":   false,
	}
	sender := map[string]any{"login": "agent-bot", "id": 1}
	base := map[string]any{
		"action":     action,
		"repository": repo,
		"sender":     sender,
	}
	switch strings.ToLower(eventType) {
	case "pull_request":
		base["pull_request"] = map[string]any{
			"id": 1, "number": 1, "state": "open",
			"title": "Fake PR",
			"head":  map[string]any{"ref": "feature", "sha": "deadbeef"},
			"base":  map[string]any{"ref": "main", "sha": "cafef00d"},
		}
	case "issues":
		base["issue"] = map[string]any{"id": 1, "number": 1, "title": "Fake issue", "state": "open"}
	case "issue_comment":
		base["issue"] = map[string]any{"id": 1, "number": 1}
		base["comment"] = map[string]any{"id": 1, "body": "fake comment"}
	case "push":
		base["ref"] = "refs/heads/main"
		base["before"] = "0000000000000000000000000000000000000000"
		base["after"] = "1111111111111111111111111111111111111111"
		delete(base, "action")
	case "installation":
		base["installation"] = map[string]any{"id": 42, "account": map[string]any{"login": "agent"}}
	case "workflow_run":
		base["workflow_run"] = map[string]any{"id": 1, "name": "ci", "status": "completed", "conclusion": "success"}
	}
	return base
}
