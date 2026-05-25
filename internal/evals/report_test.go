package evals

import "testing"

func TestBuildSummaryAggregatesByModel(t *testing.T) {
	summary := BuildSummary("suite", []TrialResult{
		{
			Key:    TrialKey{Model: "model-a"},
			Case:   Case{ExpectedBehavior: BehaviorDelegate, ExpectedSpecialist: "software-engineering-specialist"},
			Passed: true,
			Decision: Decision{
				Behavior:       BehaviorDelegate,
				SpecialistSlug: "software-engineering-specialist",
			},
		},
		{
			Key:    TrialKey{Model: "model-a"},
			Case:   Case{ExpectedBehavior: BehaviorDirect},
			Passed: true,
			Decision: Decision{
				Behavior: BehaviorDirect,
			},
		},
	})
	if summary.Overall.PassRate != 100 {
		t.Fatalf("pass rate = %.1f", summary.Overall.PassRate)
	}
	if len(summary.Models) != 1 || summary.Models[0].CorrectSpecialistRate != 100 {
		t.Fatalf("model summary mismatch: %#v", summary.Models)
	}
}
