package evals

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/model"
)

func (r *Runner) loadEvidence(ctx context.Context, fixture TrialFixture) (Evidence, error) {
	return r.loadEvidenceSince(ctx, fixture, time.Time{})
}

func (r *Runner) loadEvidenceSince(ctx context.Context, fixture TrialFixture, since time.Time) (Evidence, error) {
	var events []model.EmployeeSessionEvent
	q := r.deps.DB.WithContext(ctx).
		Where("org_id = ? AND employee_id = ?", fixture.OrgID, fixture.EmployeeID)
	if !since.IsZero() {
		q = q.Where("event_at >= ?", since)
	}
	if err := q.Order("event_at ASC, created_at ASC").Find(&events).Error; err != nil {
		return Evidence{}, fmt.Errorf("load eval session events: %w", err)
	}

	var tasks []model.SpecialistTask
	taskQ := r.deps.DB.WithContext(ctx).
		Where("org_id = ? AND employee_id = ?", fixture.OrgID, fixture.EmployeeID).
		Order("created_at ASC")
	if !since.IsZero() {
		taskQ = taskQ.Where("created_at >= ?", since)
	}
	if err := taskQ.Find(&tasks).Error; err != nil {
		return Evidence{}, fmt.Errorf("load eval specialist tasks: %w", err)
	}
	return BuildEvidence(events, tasks), nil
}

func initialCase(c Case) Case {
	if strings.TrimSpace(c.ExpectedInitial) == "" {
		return c
	}
	first := c
	first.ExpectedBehavior = c.ExpectedInitial
	first.ExpectedSpecialist = ""
	first.FollowUp = nil
	first.Assertions = CaseAssertions{}
	return first
}

func needsMoreObservation(c Case, ev Evidence) bool {
	if len(ev.Tasks) == 0 {
		return false
	}
	if c.Assertions.ObserveAfterDelegateSeconds > 0 {
		firstTaskAt := ev.Tasks[0].CreatedAt
		if time.Since(firstTaskAt) < time.Duration(c.Assertions.ObserveAfterDelegateSeconds)*time.Second {
			return true
		}
	}
	for _, name := range c.Assertions.RequiredToolCalls {
		if !hasToolCall(ev.ToolCalls, name) {
			return true
		}
	}
	return false
}
