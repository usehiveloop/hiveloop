package e2e

import (
	"testing"

	"github.com/ziraloop/ziraloop/internal/forge"
)

func TestDeterministic_ToolCalled(t *testing.T) {
	toolCalls := []forge.ToolCallInfo{
		{Name: "search_orders", Arguments: `{"order_id": "123"}`},
		{Name: "process_refund", Arguments: `{"order_id": "123"}`},
	}

	t.Run("pass_when_tool_was_called", func(t *testing.T) {
		checks := []forge.DeterministicCheck{
			{Type: "tool_called", Config: map[string]any{"tool_name": "search_orders"}},
		}
		results := forge.RunDeterministicChecks(checks, "", toolCalls)
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if !results[0].Passed {
			t.Errorf("expected tool_called(search_orders) to pass, got: %s", results[0].Details)
		}
	})

	t.Run("fail_when_tool_was_not_called", func(t *testing.T) {
		checks := []forge.DeterministicCheck{
			{Type: "tool_called", Config: map[string]any{"tool_name": "delete_account"}},
		}
		results := forge.RunDeterministicChecks(checks, "", toolCalls)
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if results[0].Passed {
			t.Errorf("expected tool_called(delete_account) to fail, but it passed")
		}
	})
}

func TestDeterministic_ToolNotCalled(t *testing.T) {
	toolCalls := []forge.ToolCallInfo{
		{Name: "search_orders", Arguments: `{}`},
	}

	t.Run("pass_when_tool_was_not_called", func(t *testing.T) {
		checks := []forge.DeterministicCheck{
			{Type: "tool_not_called", Config: map[string]any{"tool_name": "delete_account"}},
		}
		results := forge.RunDeterministicChecks(checks, "", toolCalls)
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if !results[0].Passed {
			t.Errorf("expected tool_not_called(delete_account) to pass, got: %s", results[0].Details)
		}
	})

	t.Run("fail_when_tool_was_called", func(t *testing.T) {
		checks := []forge.DeterministicCheck{
			{Type: "tool_not_called", Config: map[string]any{"tool_name": "search_orders"}},
		}
		results := forge.RunDeterministicChecks(checks, "", toolCalls)
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if results[0].Passed {
			t.Errorf("expected tool_not_called(search_orders) to fail, but it passed")
		}
	})
}

func TestDeterministic_ToolOrder(t *testing.T) {
	toolCalls := []forge.ToolCallInfo{
		{Name: "lookup_customer", Arguments: `{}`},
		{Name: "search_orders", Arguments: `{}`},
		{Name: "initiate_refund", Arguments: `{}`},
	}

	t.Run("pass_subsequence_order", func(t *testing.T) {
		checks := []forge.DeterministicCheck{
			{Type: "tool_order", Config: map[string]any{
				"order": []string{"lookup_customer", "initiate_refund"},
			}},
		}
		results := forge.RunDeterministicChecks(checks, "", toolCalls)
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if !results[0].Passed {
			t.Errorf("expected subsequence order to pass, got: %s", results[0].Details)
		}
	})

	t.Run("fail_wrong_order", func(t *testing.T) {
		checks := []forge.DeterministicCheck{
			{Type: "tool_order", Config: map[string]any{
				"order": []string{"initiate_refund", "lookup_customer"},
			}},
		}
		results := forge.RunDeterministicChecks(checks, "", toolCalls)
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if results[0].Passed {
			t.Errorf("expected wrong order to fail, but it passed")
		}
	})

	t.Run("pass_exact_order", func(t *testing.T) {
		checks := []forge.DeterministicCheck{
			{Type: "tool_order", Config: map[string]any{
				"order": []string{"lookup_customer", "search_orders", "initiate_refund"},
			}},
		}
		results := forge.RunDeterministicChecks(checks, "", toolCalls)
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if !results[0].Passed {
			t.Errorf("expected exact order to pass, got: %s", results[0].Details)
		}
	})
}

func TestDeterministic_ArgumentContains(t *testing.T) {
	toolCalls := []forge.ToolCallInfo{
		{Name: "search_orders", Arguments: `{"customer_id": "123", "status": "active"}`},
	}

	t.Run("pass_when_argument_present", func(t *testing.T) {
		checks := []forge.DeterministicCheck{
			{Type: "argument_contains", Config: map[string]any{
				"tool_name": "search_orders",
				"argument":  "customer_id",
			}},
		}
		results := forge.RunDeterministicChecks(checks, "", toolCalls)
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if !results[0].Passed {
			t.Errorf("expected argument_contains(customer_id) to pass, got: %s", results[0].Details)
		}
	})

	t.Run("fail_when_argument_missing", func(t *testing.T) {
		checks := []forge.DeterministicCheck{
			{Type: "argument_contains", Config: map[string]any{
				"tool_name": "search_orders",
				"argument":  "email",
			}},
		}
		results := forge.RunDeterministicChecks(checks, "", toolCalls)
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if results[0].Passed {
			t.Errorf("expected argument_contains(email) to fail, but it passed")
		}
	})
}

func TestDeterministic_ResponseContains(t *testing.T) {
	response := "Your refund of $49.99 has been processed per our refund policy."

	t.Run("pass_when_text_present", func(t *testing.T) {
		checks := []forge.DeterministicCheck{
			{Type: "response_contains", Config: map[string]any{"text": "refund policy"}},
		}
		results := forge.RunDeterministicChecks(checks, response, nil)
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if !results[0].Passed {
			t.Errorf("expected response_contains(refund policy) to pass, got: %s", results[0].Details)
		}
	})

	t.Run("fail_when_text_absent", func(t *testing.T) {
		checks := []forge.DeterministicCheck{
			{Type: "response_contains", Config: map[string]any{"text": "system prompt"}},
		}
		results := forge.RunDeterministicChecks(checks, response, nil)
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if results[0].Passed {
			t.Errorf("expected response_contains(system prompt) to fail, but it passed")
		}
	})

	t.Run("pass_case_insensitive", func(t *testing.T) {
		checks := []forge.DeterministicCheck{
			{Type: "response_contains", Config: map[string]any{"text": "REFUND POLICY"}},
		}
		results := forge.RunDeterministicChecks(checks, response, nil)
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if !results[0].Passed {
			t.Errorf("expected case-insensitive response_contains to pass, got: %s", results[0].Details)
		}
	})
}

func TestDeterministic_ResponseNotContains(t *testing.T) {
	response := "I can help you with that order."

	t.Run("pass_when_text_absent", func(t *testing.T) {
		checks := []forge.DeterministicCheck{
			{Type: "response_not_contains", Config: map[string]any{"text": "system prompt"}},
		}
		results := forge.RunDeterministicChecks(checks, response, nil)
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if !results[0].Passed {
			t.Errorf("expected response_not_contains(system prompt) to pass, got: %s", results[0].Details)
		}
	})

	t.Run("fail_when_text_present", func(t *testing.T) {
		checks := []forge.DeterministicCheck{
			{Type: "response_not_contains", Config: map[string]any{"text": "help you"}},
		}
		results := forge.RunDeterministicChecks(checks, response, nil)
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if results[0].Passed {
			t.Errorf("expected response_not_contains(help you) to fail, but it passed")
		}
	})
}

func TestDeterministic_MultipleChecks(t *testing.T) {
	response := "Your refund has been processed."
	toolCalls := []forge.ToolCallInfo{
		{Name: "search_orders", Arguments: `{"order_id": "123"}`},
		{Name: "process_refund", Arguments: `{"order_id": "123", "amount": "49.99"}`},
	}

	checks := []forge.DeterministicCheck{
		{Type: "tool_called", Config: map[string]any{"tool_name": "search_orders"}},
		{Type: "tool_called", Config: map[string]any{"tool_name": "process_refund"}},
		{Type: "tool_not_called", Config: map[string]any{"tool_name": "delete_account"}},
		{Type: "tool_order", Config: map[string]any{
			"order": []string{"search_orders", "process_refund"},
		}},
		{Type: "argument_contains", Config: map[string]any{
			"tool_name": "process_refund",
			"argument":  "amount",
		}},
		{Type: "response_contains", Config: map[string]any{"text": "refund"}},
		{Type: "response_not_contains", Config: map[string]any{"text": "error"}},
	}

	results := forge.RunDeterministicChecks(checks, response, toolCalls)

	if len(results) != len(checks) {
		t.Fatalf("expected %d results, got %d", len(checks), len(results))
	}

	for i, r := range results {
		if !r.Passed {
			t.Errorf("check %d (%s) failed unexpectedly: %s", i, r.CheckName, r.Details)
		}
	}
}
