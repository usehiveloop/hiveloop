package evals

import (
	"context"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/model"
)

func (r *Runner) metrics(ctx context.Context, fixture TrialFixture, since time.Time) TrialMetrics {
	var out TrialMetrics
	type row struct {
		Count           int64
		InputTokens     int64
		OutputTokens    int64
		ReasoningTokens int64
		Cost            float64
		Credits         int64
	}
	var gen row
	q := r.deps.DB.WithContext(ctx).Model(&model.Generation{}).
		Select("COUNT(*) AS count, COALESCE(SUM(input_tokens),0) AS input_tokens, COALESCE(SUM(output_tokens),0) AS output_tokens, COALESCE(SUM(reasoning_tokens),0) AS reasoning_tokens, COALESCE(SUM(cost),0) AS cost, COALESCE(SUM(credits_debited),0) AS credits").
		Where("org_id = ? AND created_at >= ?", fixture.OrgID, since)
	if strings.TrimSpace(fixture.JudgeTokenJTI) != "" {
		q = q.Where("token_jti <> ?", fixture.JudgeTokenJTI)
	}
	_ = q.Scan(&gen).Error
	out.GenerationCount = gen.Count
	out.InputTokens = gen.InputTokens
	out.OutputTokens = gen.OutputTokens
	out.ReasoningTokens = gen.ReasoningTokens
	out.CostUSD = gen.Cost
	out.CreditsDebited = gen.Credits
	return out
}
