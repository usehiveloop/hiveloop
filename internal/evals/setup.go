package evals

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/bootstrap"
	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/employeeprompts"
	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/hindsight"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/token"
)

const evalCreditGrant = int64(100_000)
const evalJudgeProxyTokenTTL = time.Hour
const evalJudgeProxyTokenType = "eval_judge_proxy"

func setupTrial(ctx context.Context, deps *bootstrap.Deps, suite *Suite, key TrialKey, judgeModelID string) (TrialFixture, error) {
	if deps == nil || deps.DB == nil || deps.Orchestrator == nil {
		return TrialFixture{}, fmt.Errorf("eval setup requires database and sandbox orchestrator")
	}
	org, user, err := createTrialOrg(ctx, deps.DB, suite, key)
	if err != nil {
		return TrialFixture{}, err
	}
	employee, err := createTrialEmployee(ctx, deps, suite, org.ID, key.Model)
	if err != nil {
		return TrialFixture{}, err
	}
	if err := grantTrialCredits(deps, org.ID, key); err != nil {
		return TrialFixture{}, err
	}
	if err := seedTrialMemories(ctx, deps, suite, org.ID, employee.ID, key); err != nil {
		return TrialFixture{}, err
	}
	sb, err := syncTrialEmployee(ctx, deps, &employee)
	if err != nil {
		return TrialFixture{}, err
	}
	route, err := createTrialRoute(ctx, deps.DB, org.ID, employee.ID, key)
	if err != nil {
		return TrialFixture{}, err
	}
	judgeToken, err := mintEvalJudgeProxyToken(ctx, deps, org.ID, employee.ID, judgeModelID)
	if err != nil {
		return TrialFixture{}, fmt.Errorf("mint eval judge proxy token: %w", err)
	}
	threadID := fmt.Sprintf("eval:%s:%s:%d:%s", key.SuiteID, key.CaseID, key.RunIndex, uuid.NewString())
	return TrialFixture{
		Key:             key,
		UserID:          user.ID,
		OrgID:           org.ID,
		EmployeeID:      employee.ID,
		RouteID:         route.ID,
		SandboxID:       sb.ID,
		ThreadID:        threadID,
		MessageID:       "msg:" + uuid.NewString(),
		JudgeProxyToken: judgeToken.Token,
		JudgeTokenJTI:   judgeToken.JTI,
	}, nil
}

func mintEvalJudgeProxyToken(ctx context.Context, deps *bootstrap.Deps, orgID, employeeID uuid.UUID, modelID string) (*employeeruntime.ProxyTokenResult, error) {
	if deps == nil || deps.DB == nil {
		return nil, fmt.Errorf("eval judge proxy token: db is required")
	}
	if len(deps.SigningKey) == 0 {
		return nil, fmt.Errorf("eval judge proxy token: signing key is required")
	}
	cred, err := credentials.NewPickerWithRegistry(deps.DB, deps.Registry).PickByModel(ctx, judgeModel(modelID))
	if err != nil {
		return nil, fmt.Errorf("resolve judge credential: %w", err)
	}
	rawToken, jti, err := token.Mint(
		deps.SigningKey,
		orgID.String(),
		cred.ID.String(),
		evalJudgeProxyTokenTTL,
		token.MintOptions{IsSystem: false},
	)
	if err != nil {
		return nil, fmt.Errorf("mint judge token: %w", err)
	}
	now := time.Now().UTC()
	expiresAt := now.Add(evalJudgeProxyTokenTTL)
	dbToken := model.Token{
		OrgID:        orgID,
		CredentialID: cred.ID,
		JTI:          jti,
		ExpiresAt:    expiresAt,
		Scopes:       model.JSON{},
		Meta: model.JSON{
			model.TokenMetaType:       evalJudgeProxyTokenType,
			model.TokenMetaEmployeeID: employeeID.String(),
			model.TokenMetaHarness:    "employee-eval",
		},
	}
	if err := deps.DB.WithContext(ctx).Create(&dbToken).Error; err != nil {
		return nil, fmt.Errorf("persist judge token: %w", err)
	}
	return &employeeruntime.ProxyTokenResult{
		Token:     "ptok_" + rawToken,
		JTI:       jti,
		ExpiresAt: expiresAt,
	}, nil
}

func createTrialOrg(ctx context.Context, db *gorm.DB, suite *Suite, key TrialKey) (model.Org, model.User, error) {
	user := model.User{
		Email: fmt.Sprintf("eval-%s-%s-%d-%s@usehivy.local",
			key.Model, key.CaseID, key.RunIndex, uuid.NewString()[:8]),
		Name: "Eval User",
	}
	user.Email = strings.ReplaceAll(user.Email, "/", "-")
	org := model.Org{
		Name:          fmt.Sprintf("eval-%s-%s-%d-%s", key.SuiteID, key.CaseID, key.RunIndex, uuid.NewString()[:8]),
		Active:        true,
		PlanSlug:      billing.FreePlanSlug,
		PromptCompany: businessPrompt(suite.Business),
		Description:   suite.Business.Industry,
		Onboarded:     true,
	}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return fmt.Errorf("create eval user: %w", err)
		}
		if err := tx.Create(&org).Error; err != nil {
			return fmt.Errorf("create eval org: %w", err)
		}
		membership := model.OrgMembership{UserID: user.ID, OrgID: org.ID, Role: "owner"}
		if err := tx.Create(&membership).Error; err != nil {
			return fmt.Errorf("create eval membership: %w", err)
		}
		return nil
	})
	return org, user, err
}

func createTrialEmployee(ctx context.Context, deps *bootstrap.Deps, suite *Suite, orgID uuid.UUID, modelID string) (model.Employee, error) {
	choice, err := credentials.Resolve(ctx, deps.DB, credentials.NewPickerWithRegistry(deps.DB, deps.Registry), &model.Employee{
		OrgID: &orgID,
		Model: modelID,
	})
	if err != nil {
		return model.Employee{}, fmt.Errorf("resolve eval employee credential: %w", err)
	}
	runtimeConfig := model.JSON{"eval": true}
	employee := model.Employee{
		OrgID:                     &orgID,
		CredentialID:              &choice.ID,
		Model:                     modelID,
		IdentityPrompt:            employeeprompts.EngineeringIdentityPrompt,
		Status:                    "draft",
		Harness:                   "employee-sandbox",
		Tools:                     model.JSON{},
		McpServers:                model.JSON{},
		Skills:                    model.JSON{},
		RuntimeConfig:             runtimeConfig,
		Permissions:               model.JSON{},
		Resources:                 model.JSON{},
		AttachedSpecialists:       pq.StringArray(deps.Specialists.AutoAttachSlugs()),
		PromptOperatingPrinciples: strings.TrimSpace(suite.Employee.Role),
	}
	if err := deps.DB.WithContext(ctx).Create(&employee).Error; err != nil {
		return employee, fmt.Errorf("create eval employee: %w", err)
	}
	if err := attachGlobalSkill(ctx, deps.DB, employee.ID, "asset-uploads"); err != nil {
		return employee, err
	}
	return employee, nil
}

func attachGlobalSkill(ctx context.Context, db *gorm.DB, employeeID uuid.UUID, name string) error {
	var skill model.Skill
	if err := db.WithContext(ctx).
		Where("org_id IS NULL AND status = ? AND name = ?", model.SkillStatusPublished, name).
		First(&skill).Error; err != nil {
		return nil
	}
	link := model.EmployeeSkill{EmployeeID: employeeID, SkillID: skill.ID}
	return db.WithContext(ctx).FirstOrCreate(&link, link).Error
}

func grantTrialCredits(deps *bootstrap.Deps, orgID uuid.UUID, key TrialKey) error {
	rawRef := fmt.Sprintf("%s:%s:%s:%d", key.SuiteID, key.Model, key.CaseID, key.RunIndex)
	sum := sha256.Sum256([]byte(rawRef))
	ref := "eval:" + hex.EncodeToString(sum[:])[:32]
	if err := deps.Credits.Grant(orgID, evalCreditGrant, billing.ReasonAdjustment, "eval_trial", ref, nil); err != nil && err != billing.ErrAlreadyRecorded {
		return fmt.Errorf("grant eval credits: %w", err)
	}
	return nil
}

func seedTrialMemories(ctx context.Context, deps *bootstrap.Deps, suite *Suite, orgID, employeeID uuid.UUID, key TrialKey) error {
	memories := caseMemories(suite, key.CaseID)
	if len(suite.Memories) == 0 && len(memories) == 0 {
		return nil
	}
	if deps.HindsightClient == nil {
		return fmt.Errorf("suite requires memories but Hindsight is not configured")
	}
	bankID := hindsight.OrgBankID(orgID)
	if err := deps.HindsightClient.ConfigureBank(ctx, bankID, hindsight.DefaultMemoryConfig().ToBankConfigUpdate()); err != nil {
		return fmt.Errorf("configure eval memory bank: %w", err)
	}
	if err := retainTrialMemories(ctx, deps, suite.Memories, bankID, orgID, employeeID, key, true); err != nil {
		return err
	}
	if err := retainTrialMemories(ctx, deps, memories, bankID, orgID, employeeID, key, false); err != nil {
		return err
	}
	return nil
}

func retainTrialMemories(ctx context.Context, deps *bootstrap.Deps, memories []MemoryFixture, bankID string, orgID, employeeID uuid.UUID, key TrialKey, async bool) error {
	if len(memories) == 0 {
		return nil
	}
	items := make([]hindsight.RetainItem, 0, len(memories))
	for i, memory := range memories {
		documentID := renderMemoryTemplate(strings.TrimSpace(memory.DocumentID), key, i)
		if documentID == "" {
			documentID = fmt.Sprintf("eval:%s:%s:%d:%d", key.SuiteID, key.CaseID, key.RunIndex, i)
		}
		items = append(items, hindsight.RetainItem{
			Content:    renderMemoryTemplate(strings.TrimSpace(memory.Content), key, i),
			Context:    "Seeded eval memory for a realistic business fixture.",
			DocumentID: documentID,
			Tags: []string{
				"company:" + orgID.String(),
				"source:eval",
				"visibility:company",
				"type:" + strings.TrimSpace(memory.Type),
			},
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Metadata: map[string]string{
				"employee_id": employeeID.String(),
				"suite_id":    key.SuiteID,
				"case_id":     key.CaseID,
				"model":       key.Model,
			},
			ObservationScopes: [][]string{{"company:" + orgID.String()}},
		})
	}
	timeout := 5 * time.Minute
	retainCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	_, err := deps.HindsightClient.Retain(retainCtx, bankID, &hindsight.RetainRequest{
		Items: items,
		Async: async,
	})
	if err != nil {
		return fmt.Errorf("retain eval memories: %w", err)
	}
	return nil
}

func caseMemories(suite *Suite, caseID string) []MemoryFixture {
	if suite == nil {
		return nil
	}
	for _, c := range suite.Cases {
		if c.ID == caseID {
			return c.Memories
		}
	}
	return nil
}

func renderMemoryTemplate(value string, key TrialKey, index int) string {
	replacements := map[string]string{
		"{{suite_id}}":     key.SuiteID,
		"{{case_id}}":      key.CaseID,
		"{{model}}":        strings.ReplaceAll(key.Model, "/", "-"),
		"{{run_index}}":    fmt.Sprintf("%d", key.RunIndex),
		"{{memory_index}}": fmt.Sprintf("%d", index),
	}
	out := value
	for from, to := range replacements {
		out = strings.ReplaceAll(out, from, to)
	}
	return out
}

func createTrialRoute(ctx context.Context, db *gorm.DB, orgID, employeeID uuid.UUID, key TrialKey) (model.EmployeeGatewayRoute, error) {
	route := model.EmployeeGatewayRoute{
		OrgID:      orgID,
		EmployeeID: employeeID,
		Provider:   "http",
		Name:       "Eval HTTP Gateway " + key.CaseID,
		Enabled:    true,
		Config:     model.JSON{},
	}
	if err := db.WithContext(ctx).Create(&route).Error; err != nil {
		return route, fmt.Errorf("create eval gateway route: %w", err)
	}
	return route, nil
}
