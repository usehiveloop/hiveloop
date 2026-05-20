package handler_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

func TestPromptWriter_RendersResolvedSkillsIntoUpstreamRequest(t *testing.T) {
	h := newPromptWriterHarness(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatCompletionResponse("## Role\nYou are deploy-watcher.\n", 10, 5)))
	})

	skill := seedSkill(t, h.db, &h.org.ID,
		"fetch-railway-logs",
		"pulls the last N lines of deployment logs from Railway",
		model.SkillStatusDraft,
	)

	stream := false
	rr := h.post(t, map[string]any{
		"stream": stream,
		"args": map[string]any{
			"name":         "deploy-watcher",
			"category":     "ops",
			"instructions": "Watch for failed Railway deployments and triage them.",
			"skill_ids":    []string{skill.ID.String()},
		},
	})

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	upstream := *h.upstreamBody
	if upstream == "" {
		t.Fatal("upstream never received the forwarded request")
	}
	if !strings.Contains(upstream, "fetch-railway-logs") {
		t.Errorf("upstream request missing skill name; body:\n%s", upstream)
	}
	if !strings.Contains(upstream, "pulls the last N lines of deployment logs from Railway") {
		t.Errorf("upstream request missing skill description; body:\n%s", upstream)
	}
	if strings.Contains(upstream, skill.ID.String()) {
		t.Errorf("raw skill UUID leaked into prompt:\n%s", upstream)
	}
}

func TestPromptWriter_ForeignOrgSkill_Returns400UnknownSkill(t *testing.T) {
	h := newPromptWriterHarness(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream must not be hit when resolution fails")
	})

	foreign := seedSkill(t, h.db, &h.otherOrg.ID,
		"private-skill",
		"belongs to a different org",
		model.SkillStatusPublished,
	)

	rr := h.post(t, map[string]any{
		"stream": false,
		"args": map[string]any{
			"name":         "deploy-watcher",
			"instructions": "irrelevant",
			"skill_ids":    []string{foreign.ID.String()},
		},
	})

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var envelope struct {
		Error     string `json:"error"`
		ErrorCode string `json:"error_code"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode envelope: %v body=%s", err, rr.Body.String())
	}
	if envelope.ErrorCode != "unknown_skill" {
		t.Errorf("error_code=%q want %q (body=%s)", envelope.ErrorCode, "unknown_skill", rr.Body.String())
	}
	if got := atomic.LoadInt32(h.hits); got != 0 {
		t.Errorf("upstream was hit %d times despite resolution failure", got)
	}
}

func TestPromptWriter_PublicPublishedSkill_VisibleAcrossOrgs(t *testing.T) {
	h := newPromptWriterHarness(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatCompletionResponse("## Role\nYou are an agent.\n", 5, 3)))
	})

	publicSkill := seedSkill(t, h.db, nil,
		"public-library-skill",
		"installed by anyone",
		model.SkillStatusPublished,
	)

	rr := h.post(t, map[string]any{
		"stream": false,
		"args": map[string]any{
			"name":         "any-agent",
			"instructions": "use the public skill",
			"skill_ids":    []string{publicSkill.ID.String()},
		},
	})

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(*h.upstreamBody, "public-library-skill") {
		t.Errorf("upstream request missing public skill name; body:\n%s", *h.upstreamBody)
	}
}

func TestPromptWriter_PublicDraftSkill_NotVisible(t *testing.T) {
	h := newPromptWriterHarness(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream must not be hit when resolution fails")
	})

	draft := seedSkill(t, h.db, nil,
		"public-draft",
		"not yet published",
		model.SkillStatusDraft,
	)

	rr := h.post(t, map[string]any{
		"stream": false,
		"args": map[string]any{
			"name":         "any-agent",
			"instructions": "irrelevant",
			"skill_ids":    []string{draft.ID.String()},
		},
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var envelope struct {
		ErrorCode string `json:"error_code"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &envelope)
	if envelope.ErrorCode != "unknown_skill" {
		t.Errorf("error_code=%q want unknown_skill", envelope.ErrorCode)
	}
}
