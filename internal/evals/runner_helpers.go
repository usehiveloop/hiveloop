package evals

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func firstGatewayMessage(suite *Suite, key TrialKey, message string) string {
	memories := allTrialMemories(suite, key.CaseID)
	if len(memories) == 0 {
		return message
	}
	var b strings.Builder
	b.WriteString("Business memory context:\n")
	for _, memory := range memories {
		content := strings.TrimSpace(renderMemoryTemplate(memory.Content, key, 0))
		if content == "" {
			continue
		}
		memoryType := strings.TrimSpace(memory.Type)
		if memoryType == "" {
			memoryType = "memory"
		}
		b.WriteString("- ")
		b.WriteString(memoryType)
		b.WriteString(": ")
		b.WriteString(strings.Join(strings.Fields(content), " "))
		b.WriteString("\n")
	}
	b.WriteString("\nUser request:\n")
	b.WriteString(message)
	return strings.TrimSpace(b.String())
}

func allTrialMemories(suite *Suite, caseID string) []MemoryFixture {
	if suite == nil {
		return nil
	}
	out := make([]MemoryFixture, 0, len(suite.Memories)+4)
	out = append(out, suite.Memories...)
	out = append(out, caseMemories(suite, caseID)...)
	return out
}

func (r *Runner) terminateSpecialistTasks(ctx context.Context, orgID uuid.UUID, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	var tasks []model.SpecialistTask
	if err := r.deps.DB.WithContext(ctx).Where("org_id = ? AND id IN ?", orgID, ids).Find(&tasks).Error; err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, task := range tasks {
		_ = r.deps.DB.WithContext(ctx).Model(&task).Updates(map[string]any{
			"status":   "terminated",
			"ended_at": now,
		}).Error
		var sb model.Sandbox
		if err := r.deps.DB.WithContext(ctx).Where("id = ?", task.SandboxID).First(&sb).Error; err == nil {
			_ = r.deps.Orchestrator.DeleteSandboxResource(ctx, &sb)
		}
	}
	return nil
}

func (r *Runner) cleanupTrial(fixture TrialFixture) {
	if fixture.OrgID == uuid.Nil || fixture.EmployeeID == uuid.Nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	var tasks []model.SpecialistTask
	if err := r.deps.DB.WithContext(ctx).
		Where("org_id = ? AND employee_id = ? AND status <> ?", fixture.OrgID, fixture.EmployeeID, "terminated").
		Find(&tasks).Error; err == nil {
		_ = r.terminateSpecialistTasks(ctx, fixture.OrgID, specialistTaskIDs(tasks))
	}
	if fixture.SandboxID == uuid.Nil {
		return
	}
	var sb model.Sandbox
	if err := r.deps.DB.WithContext(ctx).Where("id = ?", fixture.SandboxID).First(&sb).Error; err == nil {
		_ = r.deps.Orchestrator.DeleteSandboxResource(ctx, &sb)
	}
}

func failedResult(result TrialResult, err error) TrialResult {
	result.Passed = false
	result.Reason = "error"
	result.Error = err.Error()
	result.EndedAt = time.Now().UTC()
	return result
}

func suiteCase(suite *Suite, id string) Case {
	for _, c := range suite.Cases {
		if c.ID == id {
			return c
		}
	}
	return Case{ID: id}
}
