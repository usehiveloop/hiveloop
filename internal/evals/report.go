package evals

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func BuildSummary(suiteID string, runs []TrialResult) *Summary {
	out := &Summary{SuiteID: suiteID, Runs: runs}
	out.Overall = aggregate(runs)
	byModel := map[string][]TrialResult{}
	for _, run := range runs {
		byModel[run.Key.Model] = append(byModel[run.Key.Model], run)
	}
	models := make([]string, 0, len(byModel))
	for model := range byModel {
		models = append(models, model)
	}
	sort.Strings(models)
	for _, model := range models {
		out.Models = append(out.Models, ModelSummary{
			Model:            model,
			AggregateSummary: aggregate(byModel[model]),
		})
	}
	return out
}

func aggregate(runs []TrialResult) AggregateSummary {
	var out AggregateSummary
	out.TotalCases = len(runs)
	if len(runs) == 0 {
		return out
	}
	var delegateTotal, delegateOK, specialistOK int
	var directTotal, directOK, clarifyTotal, clarifyOK int
	var falseDelegates int
	for _, run := range runs {
		if run.Passed {
			out.Passed++
		}
		switch run.Case.ExpectedBehavior {
		case BehaviorDelegate:
			delegateTotal++
			if run.Decision.Behavior == BehaviorDelegate {
				delegateOK++
			}
			if run.Decision.SpecialistSlug == run.Case.ExpectedSpecialist {
				specialistOK++
			}
		case BehaviorDirect:
			directTotal++
			if run.Passed {
				directOK++
			}
		case BehaviorClarify:
			clarifyTotal++
			if run.Passed {
				clarifyOK++
			}
		}
		if run.Case.ExpectedBehavior != BehaviorDelegate && run.Decision.Behavior == BehaviorDelegate {
			falseDelegates++
		}
		out.AverageCostUSD += run.Metrics.CostUSD
		out.AverageCreditsDebited += float64(run.Metrics.CreditsDebited)
		out.AverageDecisionSeconds += float64(run.Metrics.TimeToDecisionMS) / 1000
	}
	n := float64(len(runs))
	out.PassRate = percent(out.Passed, len(runs))
	out.DelegationAccuracy = percent(delegateOK, delegateTotal)
	out.CorrectSpecialistRate = percent(specialistOK, delegateTotal)
	out.FalseDelegationRate = percent(falseDelegates, len(runs)-delegateTotal)
	out.DirectAnswerAccuracy = percent(directOK, directTotal)
	out.ClarificationAccuracy = percent(clarifyOK, clarifyTotal)
	out.AverageCostUSD /= n
	out.AverageCreditsDebited /= n
	out.AverageDecisionSeconds /= n
	return out
}

func percent(num, den int) float64 {
	if den <= 0 {
		return 0
	}
	return float64(num) * 100 / float64(den)
}

func WriteArtifacts(dir string, suite *Suite, summary *Summary, db *gorm.DB) error {
	if dir == "" {
		dir = filepath.Join("tmp", "evals", "runs", safeFileName(summary.SuiteID))
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create artifacts dir: %w", err)
	}
	if err := writeJSON(filepath.Join(dir, "summary.json"), summary); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, "seeded-fixtures.json"), suite); err != nil {
		return err
	}
	if err := writeCasesJSONL(filepath.Join(dir, "cases.jsonl"), summary.Runs); err != nil {
		return err
	}
	if err := writeMarkdownSummary(filepath.Join(dir, "summary.md"), summary); err != nil {
		return err
	}
	if err := writeEvidenceArtifacts(dir, summary); err != nil {
		return err
	}
	if db != nil {
		if err := writeDBArtifacts(dir, summary, db); err != nil {
			return err
		}
	}
	return writeFailures(filepath.Join(dir, "failures.md"), summary.Runs)
}

func writeJSON(path string, value any) error {
	bytes, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	return os.WriteFile(path, append(bytes, '\n'), 0o644)
}

func writeCasesJSONL(path string, runs []TrialResult) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer file.Close()
	w := bufio.NewWriter(file)
	for _, run := range runs {
		bytes, err := json.Marshal(run)
		if err != nil {
			return err
		}
		if _, err := w.Write(append(bytes, '\n')); err != nil {
			return err
		}
	}
	return w.Flush()
}

func writeMarkdownSummary(path string, summary *Summary) error {
	var b strings.Builder
	b.WriteString("# Eval Summary\n\n")
	b.WriteString(fmt.Sprintf("Suite: `%s`\n\n", summary.SuiteID))
	b.WriteString("| Model | Cases | Passed | Pass % | Delegation % | Correct specialist % | False delegation % | Avg credits |\n")
	b.WriteString("| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, model := range summary.Models {
		b.WriteString(fmt.Sprintf("| %s | %d | %d | %.1f | %.1f | %.1f | %.1f | %.2f |\n",
			model.Model,
			model.TotalCases,
			model.Passed,
			model.PassRate,
			model.DelegationAccuracy,
			model.CorrectSpecialistRate,
			model.FalseDelegationRate,
			model.AverageCreditsDebited,
		))
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func writeFailures(path string, runs []TrialResult) error {
	var b strings.Builder
	b.WriteString("# Eval Failures\n\n")
	for _, run := range runs {
		if run.Passed {
			continue
		}
		b.WriteString(fmt.Sprintf("## %s / %s / run %d\n\n", run.Key.Model, run.Key.CaseID, run.Key.RunIndex))
		b.WriteString(fmt.Sprintf("- Reason: %s\n", run.Reason))
		if run.Error != "" {
			b.WriteString(fmt.Sprintf("- Error: %s\n", run.Error))
		}
		b.WriteString(fmt.Sprintf("- Expected: %s `%s`\n", run.Case.ExpectedBehavior, run.Case.ExpectedSpecialist))
		b.WriteString(fmt.Sprintf("- Actual: %s `%s`\n\n", run.Decision.Behavior, run.Decision.SpecialistSlug))
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func writeEvidenceArtifacts(dir string, summary *Summary) error {
	events, err := os.Create(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		return err
	}
	defer events.Close()
	toolCalls := []ToolCall{}
	tasks := []model.SpecialistTask{}
	w := bufio.NewWriter(events)
	for _, run := range summary.Runs {
		toolCalls = append(toolCalls, run.Evidence.ToolCalls...)
		tasks = append(tasks, run.Evidence.Tasks...)
		for _, event := range run.Evidence.Events {
			row := map[string]any{"key": run.Key, "event": event}
			bytes, _ := json.Marshal(row)
			if _, err := w.Write(append(bytes, '\n')); err != nil {
				return err
			}
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, "tool-calls.json"), toolCalls); err != nil {
		return err
	}
	return writeJSON(filepath.Join(dir, "specialist-tasks.json"), tasks)
}

func writeDBArtifacts(dir string, summary *Summary, db *gorm.DB) error {
	orgIDs := []uuid.UUID{}
	for _, run := range summary.Runs {
		if run.Fixture.OrgID != uuid.Nil {
			orgIDs = append(orgIDs, run.Fixture.OrgID)
		}
	}
	if len(orgIDs) == 0 {
		return nil
	}
	var generations []model.Generation
	if err := db.Where("org_id IN ?", orgIDs).Order("created_at ASC").Find(&generations).Error; err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, "generations.json"), generations); err != nil {
		return err
	}
	var ledger []model.CreditLedgerEntry
	if err := db.Where("org_id IN ?", orgIDs).Order("created_at ASC").Find(&ledger).Error; err != nil {
		return err
	}
	return writeJSON(filepath.Join(dir, "credit-ledger.json"), ledger)
}

func safeFileName(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	if b.Len() == 0 {
		return "eval"
	}
	return b.String()
}
