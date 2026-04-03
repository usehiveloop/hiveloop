package forge

import (
	"encoding/json"
	"fmt"
	"strings"
)

// DeterministicResult is the outcome of a single deterministic check.
type DeterministicResult struct {
	CheckName string `json:"check_name"`
	Passed    bool   `json:"passed"`
	Details   string `json:"details"`
}

// RunDeterministicChecks executes all deterministic checks against the agent's
// response and tool calls. These run before the LLM judge — anything checkable
// without an LLM is checked here for speed and reliability.
func RunDeterministicChecks(checks []DeterministicCheck, response string, toolCalls []ToolCallInfo) []DeterministicResult {
	var results []DeterministicResult
	for _, check := range checks {
		result := runSingleCheck(check, response, toolCalls)
		results = append(results, result)
	}
	return results
}

func runSingleCheck(check DeterministicCheck, response string, toolCalls []ToolCallInfo) DeterministicResult {
	switch check.Type {
	case "tool_called":
		return checkToolCalled(check.Config, toolCalls)
	case "tool_not_called":
		return checkToolNotCalled(check.Config, toolCalls)
	case "tool_order":
		return checkToolOrder(check.Config, toolCalls)
	case "argument_contains":
		return checkArgumentContains(check.Config, toolCalls)
	case "response_contains":
		return checkResponseContains(check.Config, response)
	case "response_not_contains":
		return checkResponseNotContains(check.Config, response)
	default:
		return DeterministicResult{
			CheckName: check.Type,
			Passed:    false,
			Details:   fmt.Sprintf("unknown check type: %s", check.Type),
		}
	}
}

// checkToolCalled verifies a specific tool was called.
// Config: {"tool_name": "search_orders"}
func checkToolCalled(config map[string]any, toolCalls []ToolCallInfo) DeterministicResult {
	toolName, _ := config["tool_name"].(string)
	for _, tc := range toolCalls {
		if tc.Name == toolName {
			return DeterministicResult{
				CheckName: fmt.Sprintf("tool_called(%s)", toolName),
				Passed:    true,
				Details:   fmt.Sprintf("tool %q was called", toolName),
			}
		}
	}
	return DeterministicResult{
		CheckName: fmt.Sprintf("tool_called(%s)", toolName),
		Passed:    false,
		Details:   fmt.Sprintf("tool %q was never called", toolName),
	}
}

// checkToolNotCalled verifies a specific tool was NOT called.
// Config: {"tool_name": "delete_account"}
func checkToolNotCalled(config map[string]any, toolCalls []ToolCallInfo) DeterministicResult {
	toolName, _ := config["tool_name"].(string)
	for _, tc := range toolCalls {
		if tc.Name == toolName {
			return DeterministicResult{
				CheckName: fmt.Sprintf("tool_not_called(%s)", toolName),
				Passed:    false,
				Details:   fmt.Sprintf("tool %q was called but should not have been", toolName),
			}
		}
	}
	return DeterministicResult{
		CheckName: fmt.Sprintf("tool_not_called(%s)", toolName),
		Passed:    true,
		Details:   fmt.Sprintf("tool %q was correctly not called", toolName),
	}
}

// checkToolOrder verifies tools were called in a specific order.
// Config: {"order": ["lookup_customer", "initiate_refund"]}
func checkToolOrder(config map[string]any, toolCalls []ToolCallInfo) DeterministicResult {
	orderRaw, _ := config["order"]
	orderJSON, _ := json.Marshal(orderRaw)
	var expectedOrder []string
	json.Unmarshal(orderJSON, &expectedOrder)

	if len(expectedOrder) == 0 {
		return DeterministicResult{
			CheckName: "tool_order",
			Passed:    false,
			Details:   "no expected order specified",
		}
	}

	// Extract tool names in call order, filtering to only tools in the expected list.
	expectedSet := make(map[string]bool, len(expectedOrder))
	for _, name := range expectedOrder {
		expectedSet[name] = true
	}

	var actualOrder []string
	for _, tc := range toolCalls {
		if expectedSet[tc.Name] {
			actualOrder = append(actualOrder, tc.Name)
		}
	}

	// Check if actual order matches expected (subsequence match).
	orderIdx := 0
	for _, name := range actualOrder {
		if orderIdx < len(expectedOrder) && name == expectedOrder[orderIdx] {
			orderIdx++
		}
	}

	if orderIdx == len(expectedOrder) {
		return DeterministicResult{
			CheckName: fmt.Sprintf("tool_order(%s)", strings.Join(expectedOrder, " → ")),
			Passed:    true,
			Details:   "tools were called in the correct order",
		}
	}

	return DeterministicResult{
		CheckName: fmt.Sprintf("tool_order(%s)", strings.Join(expectedOrder, " → ")),
		Passed:    false,
		Details:   fmt.Sprintf("expected order %v but got %v", expectedOrder, actualOrder),
	}
}

// checkArgumentContains verifies a tool call included a specific argument.
// Config: {"tool_name": "search_orders", "argument": "customer_id"}
func checkArgumentContains(config map[string]any, toolCalls []ToolCallInfo) DeterministicResult {
	toolName, _ := config["tool_name"].(string)
	argument, _ := config["argument"].(string)

	for _, tc := range toolCalls {
		if tc.Name != toolName {
			continue
		}
		var args map[string]any
		json.Unmarshal([]byte(tc.Arguments), &args)
		if _, ok := args[argument]; ok {
			return DeterministicResult{
				CheckName: fmt.Sprintf("argument_contains(%s.%s)", toolName, argument),
				Passed:    true,
				Details:   fmt.Sprintf("tool %q included argument %q", toolName, argument),
			}
		}
	}

	return DeterministicResult{
		CheckName: fmt.Sprintf("argument_contains(%s.%s)", toolName, argument),
		Passed:    false,
		Details:   fmt.Sprintf("tool %q did not include argument %q", toolName, argument),
	}
}

// checkResponseContains verifies the response includes specific text (case-insensitive).
// Config: {"text": "refund policy"}
func checkResponseContains(config map[string]any, response string) DeterministicResult {
	text, _ := config["text"].(string)
	if strings.Contains(strings.ToLower(response), strings.ToLower(text)) {
		return DeterministicResult{
			CheckName: fmt.Sprintf("response_contains(%q)", text),
			Passed:    true,
			Details:   fmt.Sprintf("response contains %q", text),
		}
	}
	return DeterministicResult{
		CheckName: fmt.Sprintf("response_contains(%q)", text),
		Passed:    false,
		Details:   fmt.Sprintf("response does not contain %q", text),
	}
}

// checkResponseNotContains verifies the response does NOT include specific text (case-insensitive).
// Config: {"text": "system prompt"}
func checkResponseNotContains(config map[string]any, response string) DeterministicResult {
	text, _ := config["text"].(string)
	if !strings.Contains(strings.ToLower(response), strings.ToLower(text)) {
		return DeterministicResult{
			CheckName: fmt.Sprintf("response_not_contains(%q)", text),
			Passed:    true,
			Details:   fmt.Sprintf("response correctly does not contain %q", text),
		}
	}
	return DeterministicResult{
		CheckName: fmt.Sprintf("response_not_contains(%q)", text),
		Passed:    false,
		Details:   fmt.Sprintf("response contains %q but should not", text),
	}
}
