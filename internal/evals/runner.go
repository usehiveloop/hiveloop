package evals

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/bootstrap"
	"github.com/usehivy/hivy/internal/registry"
)

type Runner struct {
	deps   *bootstrap.Deps
	client *http.Client
	judge  *Judge
}

func NewRunner(deps *bootstrap.Deps) *Runner {
	return &Runner{deps: deps, client: &http.Client{Timeout: 30 * time.Second}}
}

func (r *Runner) Run(ctx context.Context, suite *Suite, opts RunOptions) (*Summary, error) {
	models := opts.Models
	if len(models) == 0 {
		models = suite.Models
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("at least one model is required")
	}
	for _, modelID := range models {
		if err := registry.Global().ValidateCanonicalModel(modelID); err != nil {
			return nil, fmt.Errorf("model %q: %w", modelID, err)
		}
	}
	if err := registry.Global().ValidateCanonicalModel(judgeModel(opts.JudgeModel)); err != nil {
		return nil, fmt.Errorf("judge model %q: %w", judgeModel(opts.JudgeModel), err)
	}
	runs := opts.Runs
	if runs <= 0 {
		runs = 1
	}
	parallel := opts.Parallel
	if parallel <= 0 {
		parallel = 1
	}
	apiURL := strings.TrimRight(opts.APIURL, "/")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}
	r.judge = NewJudge(opts.JudgeModel)

	jobs := buildJobs(suite, models, runs)
	results := make([]TrialResult, len(jobs))
	work := make(chan int)
	var wg sync.WaitGroup
	for i := 0; i < parallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range work {
				result, _ := r.runTrial(ctx, suite, jobs[index], apiURL)
				results[index] = result
			}
		}()
	}
	for index := range jobs {
		work <- index
	}
	close(work)
	wg.Wait()
	return BuildSummary(suite.ID, results), nil
}

func buildJobs(suite *Suite, models []string, runs int) []TrialKey {
	jobs := []TrialKey{}
	for _, modelID := range models {
		for _, c := range suite.Cases {
			for i := 1; i <= runs; i++ {
				jobs = append(jobs, TrialKey{
					SuiteID:  suite.ID,
					Model:    modelID,
					CaseID:   c.ID,
					RunIndex: i,
				})
			}
		}
	}
	return jobs
}

func (r *Runner) runTrial(ctx context.Context, suite *Suite, key TrialKey, apiURL string) (TrialResult, error) {
	c := suiteCase(suite, key.CaseID)
	setupCtx, setupCancel := context.WithTimeout(ctx, 2*time.Minute)
	defer setupCancel()

	result := TrialResult{Key: key, Case: c, StartedAt: time.Now().UTC()}
	fixture, err := setupTrial(setupCtx, r.deps, suite, key, r.judge.model)
	result.Fixture = fixture
	defer r.cleanupTrial(result.Fixture)
	if err != nil {
		return failedResult(result, err), err
	}
	trialCtx, cancel := context.WithTimeout(ctx, suite.TimeoutFor(c))
	defer cancel()

	started := time.Now().UTC()
	result.StartedAt = started
	gateway, err := r.sendGatewayMessage(trialCtx, apiURL, fixture, firstGatewayMessage(suite, key, c.Message))
	result.Gateway = gateway
	if err != nil {
		return failedResult(result, err), err
	}
	ev, err := r.waitForDecision(trialCtx, suite.TimeoutFor(c), fixture, initialCase(c), apiURL, started)
	if err == nil && c.FollowUp != nil {
		followUp, genErr := r.judge.GenerateFollowUp(trialCtx, proxyBaseURL(apiURL), fixture.JudgeProxyToken, c, ev.Evidence.FinalText)
		if genErr != nil {
			err = fmt.Errorf("generate clarification follow-up: %w", genErr)
		} else {
			followUpAt := time.Now().UTC()
			if _, sendErr := r.sendGatewayMessageWithID(trialCtx, apiURL, fixture, followUp, "msg:"+uuid.NewString()); sendErr != nil {
				err = sendErr
			} else {
				ev, err = r.waitForDecision(trialCtx, suite.TimeoutFor(c), fixture, c, apiURL, followUpAt)
			}
		}
	}
	result.Evidence = ev.Evidence
	if err != nil {
		result = failedResult(result, err)
		result.Decision = ev.Decision
		result.Metrics = r.metrics(context.Background(), fixture, started)
		result.Evidence = ev.Evidence
		return result, nil
	}
	result.Passed = ev.Passed
	result.Reason = ev.Reason
	result.Decision = ev.Decision
	result.EndedAt = time.Now().UTC()
	result.Metrics = r.metrics(ctx, fixture, started)
	if !result.Decision.DecidedAt.IsZero() {
		result.Metrics.TimeToDecisionMS = result.Decision.DecidedAt.Sub(started).Milliseconds()
	}
	return result, nil
}

type evaluatedEvidence struct {
	Evidence Evidence
	Passed   bool
	Reason   string
	Decision Decision
}

func (r *Runner) waitForDecision(ctx context.Context, timeout time.Duration, fixture TrialFixture, c Case, apiURL string, since time.Time) (evaluatedEvidence, error) {
	deadline := time.Now().Add(timeout)
	var last evaluatedEvidence
	for time.Now().Before(deadline) {
		evidence, err := r.loadEvidenceSince(ctx, fixture, since)
		if err != nil {
			return last, err
		}
		var judgement *BehaviorJudgement
		if len(evidence.Tasks) == 0 && strings.TrimSpace(evidence.FinalText) != "" {
			var err error
			judgement, err = r.judge.ClassifyFinalText(ctx, proxyBaseURL(apiURL), fixture.JudgeProxyToken, c, evidence.FinalText)
			if err != nil {
				return last, fmt.Errorf("judge final response: %w", err)
			}
		}
		passed, reason, decision := GradeCaseWithJudgement(c, evidence, judgement)
		last = evaluatedEvidence{Evidence: evidence, Passed: passed, Reason: reason, Decision: decision}
		if IsTerminal(c, evidence) && !needsMoreObservation(c, evidence) {
			return last, nil
		}
		time.Sleep(2 * time.Second)
	}
	return last, fmt.Errorf("trial timed out after %s: %s", timeout, last.Reason)
}

func (r *Runner) sendGatewayMessage(ctx context.Context, apiURL string, fixture TrialFixture, message string) (GatewayResponse, error) {
	return r.sendGatewayMessageWithID(ctx, apiURL, fixture, message, fixture.MessageID)
}

func (r *Runner) sendGatewayMessageWithID(ctx context.Context, apiURL string, fixture TrialFixture, message, messageID string) (GatewayResponse, error) {
	body, _ := json.Marshal(map[string]any{
		"markdown":    message,
		"thread_id":   fixture.ThreadID,
		"message_id":  messageID,
		"sender_id":   "eval-user",
		"sender_name": "Eval User",
	})
	url := fmt.Sprintf("%s/incoming/gateways/http/%s", apiURL, fixture.RouteID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return GatewayResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return GatewayResponse{}, fmt.Errorf("post gateway message: %w", err)
	}
	defer resp.Body.Close()
	var out GatewayResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, fmt.Errorf("decode gateway response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, fmt.Errorf("gateway returned status %d", resp.StatusCode)
	}
	return out, nil
}
