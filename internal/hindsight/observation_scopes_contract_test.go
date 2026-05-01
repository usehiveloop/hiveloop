package hindsight

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// configurableFields is the authoritative whitelist copied verbatim from
// hindsight-api-slim/hindsight_api/config.py:1165-1210 (`_CONFIGURABLE_FIELDS`).
// If hindsight ever drifts and we miss a sync, the bank-config tests below
// will catch it the moment they run against a real hindsight.
var configurableFields = map[string]struct{}{
	"mcp_enabled_tools":                                     {},
	"retain_chunk_size":                                     {},
	"retain_extraction_mode":                                {},
	"retain_mission":                                        {},
	"retain_custom_instructions":                            {},
	"retain_default_strategy":                               {},
	"retain_strategies":                                     {},
	"retain_chunk_batch_size":                               {},
	"entity_labels":                                         {},
	"entities_allow_free_form":                              {},
	"enable_observations":                                   {},
	"consolidation_llm_batch_size":                          {},
	"consolidation_max_memories_per_round":                  {},
	"consolidation_source_facts_max_tokens":                 {},
	"consolidation_source_facts_max_tokens_per_observation": {},
	"observations_mission":                                  {},
	"max_observations_per_scope":                            {},
	"reflect_mission":                                       {},
	"reflect_source_facts_max_tokens":                       {},
	"recall_include_chunks":                                 {},
	"recall_max_tokens":                                     {},
	"recall_chunks_max_tokens":                              {},
	"recall_budget_function":                                {},
	"recall_budget_fixed_low":                               {},
	"recall_budget_fixed_mid":                               {},
	"recall_budget_fixed_high":                              {},
	"recall_budget_adaptive_low":                            {},
	"recall_budget_adaptive_mid":                            {},
	"recall_budget_adaptive_high":                           {},
	"recall_budget_min":                                     {},
	"recall_budget_max":                                     {},
	"disposition_skepticism":                                {},
	"disposition_literalism":                                {},
	"disposition_empathy":                                   {},
	"llm_gemini_safety_settings":                            {},
}

// fakeHindsightServer mimics the parts of the hindsight API we exercise here.
// Records every request body for assertions and rejects bank-config payloads
// the same way the real config_resolver does.
type fakeHindsightServer struct {
	configPayloads []map[string]any
	retainPayloads []RetainRequest
	configRejects  []string
}

func newFakeHindsight(t *testing.T) (*httptest.Server, *fakeHindsightServer) {
	t.Helper()
	state := &fakeHindsightServer{}
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/default/banks/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch {
		case r.Method == http.MethodPatch && strings.HasSuffix(r.URL.Path, "/config"):
			var req struct {
				Updates map[string]any `json:"updates"`
			}
			if err := json.Unmarshal(body, &req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			var invalid []string
			for k := range req.Updates {
				if _, ok := configurableFields[k]; !ok {
					invalid = append(invalid, k)
				}
			}
			if len(invalid) > 0 {
				state.configRejects = append(state.configRejects, invalid...)
				http.Error(w,
					`{"detail":"Unknown configuration fields: `+joinSorted(invalid)+`"}`,
					http.StatusBadRequest)
				return
			}
			state.configPayloads = append(state.configPayloads, req.Updates)
			_, _ = w.Write([]byte(`{"bank_id":"x","config":{},"overrides":{}}`))

		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/memories"):
			var req RetainRequest
			if err := json.Unmarshal(body, &req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			state.retainPayloads = append(state.retainPayloads, req)
			_, _ = w.Write([]byte(`{"success":true,"bank_id":"x","items_count":1,"async":true}`))

		default:
			http.NotFound(w, r)
		}
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, state
}

func joinSorted(xs []string) string {
	b, _ := json.Marshal(xs)
	return string(b)
}

func TestObsScopes_BankConfigPayloadOnlyIncludesAllowlistedFields(t *testing.T) {
	srv, state := newFakeHindsight(t)
	client := NewClient(srv.URL)

	cfg := DefaultMemoryConfig().ToBankConfigUpdate()
	if err := client.ConfigureBank(context.Background(), "org-test", cfg); err != nil {
		t.Fatalf("ConfigureBank: %v", err)
	}

	if len(state.configRejects) > 0 {
		t.Fatalf("hindsight rejected fields that should be allow-listed: %v", state.configRejects)
	}
	if len(state.configPayloads) != 1 {
		t.Fatalf("expected 1 config payload, got %d", len(state.configPayloads))
	}

	payload := state.configPayloads[0]
	if _, present := payload["observation_scopes"]; present {
		t.Errorf("bank-config payload still includes observation_scopes — should live on RetainItem instead")
	}
	for k := range payload {
		if _, ok := configurableFields[k]; !ok {
			t.Errorf("bank-config payload includes non-allowlisted field %q", k)
		}
	}
}

func TestObsScopes_RetainItemCarriesObservationScopes(t *testing.T) {
	srv, state := newFakeHindsight(t)
	client := NewClient(srv.URL)

	want := [][]string{{"agent:abc-123"}}
	_, err := client.Retain(context.Background(), "org-test", &RetainRequest{
		Items: []RetainItem{{
			Content:           "hello",
			Tags:              []string{"agent:abc-123"},
			ObservationScopes: want,
		}},
	})
	if err != nil {
		t.Fatalf("Retain: %v", err)
	}

	if len(state.retainPayloads) != 1 {
		t.Fatalf("expected 1 retain payload, got %d", len(state.retainPayloads))
	}
	got := state.retainPayloads[0].Items[0].ObservationScopes
	if len(got) != 1 || len(got[0]) != 1 || got[0][0] != "agent:abc-123" {
		t.Errorf("observation_scopes round-trip: got %v, want %v", got, want)
	}
}

func TestObsScopes_RetainItemWithoutScopesOmitsField(t *testing.T) {
	// Verifies the json:"omitempty" tag — RetainItem callers without per-agent
	// scoping (e.g. legacy test fixtures) shouldn't emit a null field.
	body, err := json.Marshal(RetainItem{Content: "x"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(body), "observation_scopes") {
		t.Errorf("empty ObservationScopes should be omitted, got %s", body)
	}
}

func TestObsScopes_BankConfigSendsAllExpectedFields(t *testing.T) {
	srv, state := newFakeHindsight(t)
	client := NewClient(srv.URL)

	if err := client.ConfigureBank(context.Background(), "org-test", DefaultMemoryConfig().ToBankConfigUpdate()); err != nil {
		t.Fatalf("ConfigureBank: %v", err)
	}

	must := []string{
		"retain_mission",
		"reflect_mission",
		"observations_mission",
		"disposition_skepticism",
		"disposition_literalism",
		"disposition_empathy",
	}
	payload := state.configPayloads[0]
	for _, k := range must {
		if _, ok := payload[k]; !ok {
			t.Errorf("bank-config payload missing %q", k)
		}
	}
}
