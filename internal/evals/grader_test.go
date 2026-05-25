package evals

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestGradeCasePassesCorrectSpecialistDelegation(t *testing.T) {
	now := time.Now().UTC()
	task := model.SpecialistTask{
		ID:             uuid.New(),
		SpecialistSlug: "business-research-specialist",
		CreatedAt:      now,
	}
	ev := Evidence{
		Tasks: []model.SpecialistTask{task},
		ToolCalls: []ToolCall{
			{Name: "hivy_memory_recall", EventAt: now.Add(-4 * time.Second)},
			{Name: "skills_list", EventAt: now.Add(-3 * time.Second)},
			{Name: "hivy_specialist_launch_task", EventAt: now.Add(-time.Second)},
		},
	}
	passed, reason, decision := GradeCase(Case{
		ExpectedBehavior:   BehaviorDelegate,
		ExpectedSpecialist: "business-research-specialist",
	}, ev)
	if !passed {
		t.Fatalf("GradeCase failed: %s %#v", reason, decision)
	}
}

func TestGradeCaseAllowsPreliminaryWorkBeforeDelegation(t *testing.T) {
	now := time.Now().UTC()
	ev := Evidence{
		Tasks: []model.SpecialistTask{{
			ID:             uuid.New(),
			SpecialistSlug: "software-engineering-specialist",
			CreatedAt:      now,
		}},
		ToolCalls: []ToolCall{{
			Name:    "bash",
			Args:    []byte(`{"command":"find /workspace -maxdepth 3 -type f | head -60"}`),
			EventAt: now.Add(-time.Second),
		}},
	}
	passed, reason, decision := GradeCase(Case{
		ExpectedBehavior:   BehaviorDelegate,
		ExpectedSpecialist: "software-engineering-specialist",
	}, ev)
	if !passed || decision.Behavior != BehaviorDelegate {
		t.Fatalf("GradeCase = %v %q %#v", passed, reason, decision)
	}
}

func TestGradeCaseClassifiesClarification(t *testing.T) {
	passed, reason, decision := GradeCaseWithJudgement(Case{
		ExpectedBehavior: BehaviorClarify,
	}, Evidence{FinalText: "Which campaign do you mean?", FinalEventAt: time.Now()}, &BehaviorJudgement{Behavior: BehaviorClarify})
	if !passed || decision.Behavior != BehaviorClarify {
		t.Fatalf("GradeCase = %v %q %#v", passed, reason, decision)
	}
}

func TestGradeCaseUsesJudgeClarification(t *testing.T) {
	passed, reason, decision := GradeCaseWithJudgement(Case{
		ExpectedBehavior: BehaviorClarify,
	}, Evidence{FinalText: `Can you refresh me on what "the thing" is?`, FinalEventAt: time.Now()}, &BehaviorJudgement{Behavior: BehaviorClarify})
	if !passed || decision.Behavior != BehaviorClarify {
		t.Fatalf("GradeCase = %v %q %#v", passed, reason, decision)
	}
}

func TestGradeCaseFailsClarifyWithoutJudge(t *testing.T) {
	passed, reason, decision := GradeCase(Case{
		ExpectedBehavior: BehaviorClarify,
	}, Evidence{FinalText: `Can you refresh me on what "the thing" is?`, FinalEventAt: time.Now()})
	if passed || decision.Behavior != BehaviorDirect {
		t.Fatalf("GradeCase = %v %q %#v", passed, reason, decision)
	}
}

func TestGradeCaseAllowsDirectAnswerWithFollowupQuestion(t *testing.T) {
	passed, reason, decision := GradeCase(Case{
		ExpectedBehavior: BehaviorDirect,
	}, Evidence{FinalText: "I'm Hivy, Luma Cakes' engineering coordinator. What do you need done?", FinalEventAt: time.Now()})
	if !passed || decision.Behavior != BehaviorDirect {
		t.Fatalf("GradeCase = %v %q %#v", passed, reason, decision)
	}
}
