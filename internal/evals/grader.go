package evals

import (
	"fmt"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/model"
)

type Decision struct {
	Behavior           string             `json:"behavior"`
	ExpectedBehavior   string             `json:"expected_behavior"`
	SpecialistSlug     string             `json:"specialist_slug,omitempty"`
	ExpectedSpecialist string             `json:"expected_specialist,omitempty"`
	ToolCallsBefore    []ToolCall         `json:"tool_calls_before_delegate,omitempty"`
	FinalText          string             `json:"final_text,omitempty"`
	Judgement          *BehaviorJudgement `json:"judgement,omitempty"`
	DecidedAt          time.Time          `json:"decided_at,omitempty"`
}

func GradeCase(c Case, ev Evidence) (bool, string, Decision) {
	return GradeCaseWithJudgement(c, ev, nil)
}

func GradeCaseWithJudgement(c Case, ev Evidence, judgement *BehaviorJudgement) (bool, string, Decision) {
	decision := Decision{
		ExpectedBehavior:   c.ExpectedBehavior,
		ExpectedSpecialist: c.ExpectedSpecialist,
		FinalText:          ev.FinalText,
		Judgement:          judgement,
	}
	if len(ev.Tasks) > 0 {
		task := ev.Tasks[0]
		decision.Behavior = BehaviorDelegate
		decision.SpecialistSlug = task.SpecialistSlug
		decision.DecidedAt = task.CreatedAt
		decision.ToolCallsBefore = toolCallsBefore(ev.ToolCalls, task.CreatedAt)
		return gradeDelegate(c, ev, decision)
	}
	if strings.TrimSpace(ev.FinalText) != "" {
		decision.DecidedAt = ev.FinalEventAt
		if judgement != nil && strings.TrimSpace(judgement.Behavior) != "" {
			decision.Behavior = judgement.Behavior
		} else {
			decision.Behavior = BehaviorDirect
		}
		return gradeNonDelegate(c, ev, decision)
	}
	decision.Behavior = "pending"
	return false, "no delegation or final response observed", decision
}

func IsTerminal(c Case, ev Evidence) bool {
	if len(ev.Tasks) > 0 {
		return true
	}
	return strings.TrimSpace(ev.FinalText) != ""
}

func gradeDelegate(c Case, ev Evidence, d Decision) (bool, string, Decision) {
	if c.ExpectedBehavior != BehaviorDelegate {
		return false, "launched specialist for non-delegation case", d
	}
	if d.SpecialistSlug != c.ExpectedSpecialist {
		return false, fmt.Sprintf("wrong specialist: got %q want %q", d.SpecialistSlug, c.ExpectedSpecialist), d
	}
	if passed, reason := gradeAssertions(c, ev); !passed {
		return false, reason, d
	}
	return true, "correct specialist delegation", d
}

func gradeNonDelegate(c Case, ev Evidence, d Decision) (bool, string, Decision) {
	switch c.ExpectedBehavior {
	case BehaviorDirect:
		if d.Behavior == BehaviorDirect {
			if passed, reason := gradeAssertions(c, ev); !passed {
				return false, reason, d
			}
			return true, "answered directly", d
		}
		return false, "asked for clarification when direct answer was expected", d
	case BehaviorClarify:
		if d.Behavior == BehaviorClarify {
			if passed, reason := gradeAssertions(c, ev); !passed {
				return false, reason, d
			}
			return true, "asked for clarification", d
		}
		return false, "answered directly when clarification was expected", d
	default:
		return false, "missed expected specialist delegation", d
	}
}

func gradeAssertions(c Case, ev Evidence) (bool, string) {
	for _, name := range c.Assertions.RequiredToolCalls {
		if !hasToolCall(ev.ToolCalls, name) {
			return false, fmt.Sprintf("missing required tool call %q", name)
		}
	}
	for _, name := range c.Assertions.ForbiddenToolCalls {
		if hasToolCall(ev.ToolCalls, name) {
			return false, fmt.Sprintf("forbidden tool call %q was used", name)
		}
	}
	for _, needle := range c.Assertions.RequiredFinalText {
		if !containsFold(ev.FinalText, needle) {
			return false, fmt.Sprintf("final text missing %q", needle)
		}
	}
	for _, needle := range c.Assertions.ForbiddenFinalText {
		if containsFold(ev.FinalText, needle) {
			return false, fmt.Sprintf("final text contained forbidden text %q", needle)
		}
	}
	for _, needle := range c.Assertions.RequiredBriefContains {
		if !tasksContainBrief(ev.Tasks, needle) {
			return false, fmt.Sprintf("specialist brief missing %q", needle)
		}
	}
	for _, needle := range c.Assertions.ForbiddenBriefContains {
		if tasksContainBrief(ev.Tasks, needle) {
			return false, fmt.Sprintf("specialist brief contained forbidden text %q", needle)
		}
	}
	if c.Assertions.MaxStatusChecksBeforeWake > 0 {
		count := statusChecksBeforeWake(ev.ToolCalls)
		if count > c.Assertions.MaxStatusChecksBeforeWake {
			return false, fmt.Sprintf("too many specialist status checks before wake: got %d want <= %d", count, c.Assertions.MaxStatusChecksBeforeWake)
		}
	}
	return true, ""
}

func hasToolCall(calls []ToolCall, name string) bool {
	for _, call := range calls {
		if toolNameMatches(call.Name, name) {
			return true
		}
	}
	return false
}

func toolNameMatches(got, want string) bool {
	got = strings.TrimSpace(strings.ToLower(got))
	want = strings.TrimSpace(strings.ToLower(want))
	return got == want || strings.HasSuffix(got, "_"+want) || strings.HasSuffix(got, "."+want)
}

func containsFold(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(strings.TrimSpace(needle)))
}

func tasksContainBrief(tasks []model.SpecialistTask, needle string) bool {
	for _, task := range tasks {
		if containsFold(task.Brief, needle) {
			return true
		}
	}
	return false
}

func statusChecksBeforeWake(calls []ToolCall) int {
	count := 0
	for _, call := range calls {
		if toolNameMatches(call.Name, "wake") {
			return count
		}
		if toolNameMatches(call.Name, "specialist_task_status") {
			count++
		}
	}
	return count
}

func toolCallsBefore(calls []ToolCall, cutoff time.Time) []ToolCall {
	if cutoff.IsZero() {
		return nil
	}
	out := []ToolCall{}
	for _, call := range calls {
		if call.EventAt.Before(cutoff) {
			out = append(out, call)
		}
	}
	return out
}
