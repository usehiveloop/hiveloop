package plancatalog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

const (
	defaultPath   = "global/plans/catalog.json"
	seedLockKey   = 2026052501
	supportedVers = 1
)

type Catalog struct {
	Version int        `json:"version"`
	Plans   []PlanSpec `json:"plans"`
}

type PlanSpec struct {
	Slug           string       `json:"slug"`
	Name           string       `json:"name"`
	Provider       string       `json:"provider"`
	Visible        *bool        `json:"visible"`
	Active         *bool        `json:"active"`
	MonthlyCredits int64        `json:"monthly_credits"`
	WelcomeCredits int64        `json:"welcome_credits"`
	PriceCents     int64        `json:"price_cents"`
	Currency       string       `json:"currency"`
	Features       []string     `json:"features"`
	Paystack       PaystackSpec `json:"paystack,omitempty"`
}

type PaystackSpec struct {
	Sync     bool   `json:"sync"`
	Interval string `json:"interval,omitempty"`
}

type SyncResult struct {
	Created   int
	Updated   int
	Unchanged int
}

func Load(path string) (*Catalog, error) {
	if strings.TrimSpace(path) == "" {
		path = defaultPath
	}
	resolved, err := resolveFile(path)
	if err != nil {
		return nil, err
	}
	body, err := os.ReadFile(resolved)
	if err != nil {
		return nil, fmt.Errorf("read plan catalog %q: %w", resolved, err)
	}
	var catalog Catalog
	if err := json.Unmarshal(body, &catalog); err != nil {
		return nil, fmt.Errorf("parse plan catalog %q: %w", resolved, err)
	}
	if err := catalog.Validate(); err != nil {
		return nil, err
	}
	return &catalog, nil
}

func SyncDB(ctx context.Context, db *gorm.DB, path string) (*SyncResult, error) {
	if db == nil {
		return nil, fmt.Errorf("db is required")
	}
	catalog, err := Load(path)
	if err != nil {
		return nil, err
	}
	result := &SyncResult{}
	slugs := make([]string, 0, len(catalog.Plans))
	for _, spec := range catalog.Plans {
		slugs = append(slugs, spec.Slug)
	}

	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", seedLockKey).Error; err != nil {
			return fmt.Errorf("lock plan catalog seed: %w", err)
		}
		var existing []model.Plan
		if err := tx.Where("slug IN ?", slugs).Find(&existing).Error; err != nil {
			return fmt.Errorf("load existing plans: %w", err)
		}
		bySlug := make(map[string]model.Plan, len(existing))
		for _, plan := range existing {
			bySlug[plan.Slug] = plan
		}
		for _, spec := range catalog.Plans {
			next, err := spec.toModel()
			if err != nil {
				return err
			}
			current, ok := bySlug[next.Slug]
			if !ok {
				if err := tx.Select(
					"Slug",
					"Name",
					"Provider",
					"Features",
					"MonthlyCredits",
					"WelcomeCredits",
					"PriceCents",
					"Currency",
					"Active",
					"Visible",
				).Create(&next).Error; err != nil {
					return fmt.Errorf("create plan %q: %w", next.Slug, err)
				}
				result.Created++
				continue
			}
			if samePlan(current, next) {
				result.Unchanged++
				continue
			}
			updates := map[string]any{
				"name":            next.Name,
				"provider":        next.Provider,
				"features":        next.Features,
				"monthly_credits": next.MonthlyCredits,
				"welcome_credits": next.WelcomeCredits,
				"price_cents":     next.PriceCents,
				"currency":        next.Currency,
				"active":          next.Active,
				"visible":         next.Visible,
			}
			if err := tx.Model(&model.Plan{}).Where("id = ?", current.ID).Updates(updates).Error; err != nil {
				return fmt.Errorf("update plan %q: %w", next.Slug, err)
			}
			result.Updated++
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (c Catalog) Validate() error {
	if c.Version != supportedVers {
		return fmt.Errorf("plan catalog version %d is unsupported", c.Version)
	}
	if len(c.Plans) == 0 {
		return fmt.Errorf("plan catalog must contain at least one plan")
	}
	seen := make(map[string]bool, len(c.Plans))
	for _, plan := range c.Plans {
		if err := plan.Validate(); err != nil {
			return err
		}
		if seen[plan.Slug] {
			return fmt.Errorf("duplicate plan slug %q", plan.Slug)
		}
		seen[plan.Slug] = true
	}
	return nil
}

func (p PlanSpec) Validate() error {
	switch {
	case strings.TrimSpace(p.Slug) == "":
		return fmt.Errorf("plan slug is required")
	case strings.TrimSpace(p.Name) == "":
		return fmt.Errorf("plan %q name is required", p.Slug)
	case p.Visible == nil:
		return fmt.Errorf("plan %q visible is required", p.Slug)
	case p.Active == nil:
		return fmt.Errorf("plan %q active is required", p.Slug)
	case strings.TrimSpace(p.Currency) == "":
		return fmt.Errorf("plan %q currency is required", p.Slug)
	case p.MonthlyCredits < 0:
		return fmt.Errorf("plan %q monthly_credits cannot be negative", p.Slug)
	case p.WelcomeCredits < 0:
		return fmt.Errorf("plan %q welcome_credits cannot be negative", p.Slug)
	case p.PriceCents < 0:
		return fmt.Errorf("plan %q price_cents cannot be negative", p.Slug)
	}
	for _, feature := range p.Features {
		if strings.TrimSpace(feature) == "" {
			return fmt.Errorf("plan %q features contains an empty value", p.Slug)
		}
	}
	if p.Paystack.Sync && strings.TrimSpace(p.Paystack.Interval) == "" {
		return fmt.Errorf("plan %q paystack.interval is required when paystack.sync is true", p.Slug)
	}
	return nil
}

func (p PlanSpec) toModel() (model.Plan, error) {
	features, err := json.Marshal(p.Features)
	if err != nil {
		return model.Plan{}, fmt.Errorf("marshal features for plan %q: %w", p.Slug, err)
	}
	return model.Plan{
		Slug:           p.Slug,
		Name:           p.Name,
		Provider:       p.Provider,
		Features:       model.RawJSON(features),
		MonthlyCredits: p.MonthlyCredits,
		WelcomeCredits: p.WelcomeCredits,
		PriceCents:     p.PriceCents,
		Currency:       strings.ToUpper(p.Currency),
		Active:         *p.Active,
		Visible:        *p.Visible,
	}, nil
}

func samePlan(a, b model.Plan) bool {
	return a.Name == b.Name &&
		a.Provider == b.Provider &&
		bytes.Equal(normalizeJSON(a.Features), normalizeJSON(b.Features)) &&
		a.MonthlyCredits == b.MonthlyCredits &&
		a.WelcomeCredits == b.WelcomeCredits &&
		a.PriceCents == b.PriceCents &&
		a.Currency == b.Currency &&
		a.Active == b.Active &&
		a.Visible == b.Visible
}

func normalizeJSON(raw model.RawJSON) []byte {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return []byte(raw)
	}
	out, err := json.Marshal(value)
	if err != nil {
		return []byte(raw)
	}
	return out
}

func resolveFile(path string) (string, error) {
	if stat, err := os.Stat(path); err == nil && !stat.IsDir() {
		return path, nil
	}
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("plan catalog %q not found", path)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get cwd: %w", err)
	}
	for {
		candidate := filepath.Join(cwd, path)
		if stat, err := os.Stat(candidate); err == nil && !stat.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			break
		}
		cwd = parent
	}
	return "", fmt.Errorf("plan catalog %q not found", path)
}
