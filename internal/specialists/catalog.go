package specialists

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

const defaultDir = "global/specialists"

type Definition struct {
	Slug              string
	Name              string
	Description       string
	SpecialistType    string
	Version           int
	AutoAttach        bool
	DefaultModel      string
	DefaultSkillNames []string
	SystemPrompt      string
}

type Catalog struct {
	items  []Definition
	bySlug map[string]int
}

type manifest struct {
	Slug              string   `json:"slug"`
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	SpecialistType    string   `json:"specialist_type"`
	Version           int      `json:"version"`
	AutoAttach        bool     `json:"auto_attach"`
	DefaultModel      string   `json:"default_model"`
	DefaultSkillNames []string `json:"default_skill_names"`
	PromptPath        string   `json:"prompt_path"`
}

func Load(dir string) (*Catalog, error) {
	if strings.TrimSpace(dir) == "" {
		dir = defaultDir
	}
	resolved, err := resolveDir(dir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(resolved)
	if err != nil {
		return nil, fmt.Errorf("read global specialists dir %q: %w", resolved, err)
	}
	items := make([]Definition, 0, len(entries))
	seen := map[string]bool{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		def, err := loadDefinition(filepath.Join(resolved, entry.Name()))
		if err != nil {
			return nil, err
		}
		if seen[def.Slug] {
			return nil, fmt.Errorf("duplicate specialist slug %q", def.Slug)
		}
		seen[def.Slug] = true
		items = append(items, def)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Slug < items[j].Slug
	})
	return NewCatalog(items)
}

func NewCatalog(items []Definition) (*Catalog, error) {
	c := &Catalog{
		items:  make([]Definition, len(items)),
		bySlug: make(map[string]int, len(items)),
	}
	copy(c.items, items)
	for i := range c.items {
		def := &c.items[i]
		if err := validateDefinition(*def); err != nil {
			return nil, err
		}
		if _, exists := c.bySlug[def.Slug]; exists {
			return nil, fmt.Errorf("duplicate specialist slug %q", def.Slug)
		}
		c.bySlug[def.Slug] = i
	}
	return c, nil
}

func EmptyCatalog() *Catalog {
	c, _ := NewCatalog(nil)
	return c
}

func (c *Catalog) List() []Definition {
	if c == nil {
		return nil
	}
	out := make([]Definition, len(c.items))
	copy(out, c.items)
	return out
}

func (c *Catalog) BySlug(slug string) (*Definition, bool) {
	if c == nil {
		return nil, false
	}
	i, ok := c.bySlug[slug]
	if !ok {
		return nil, false
	}
	def := c.items[i]
	return &def, true
}

func (c *Catalog) AutoAttachSlugs() []string {
	if c == nil {
		return nil
	}
	out := []string{}
	for _, def := range c.items {
		if def.AutoAttach {
			out = append(out, def.Slug)
		}
	}
	return out
}

func (c *Catalog) ValidateSkillNames(ctx context.Context, db *gorm.DB) error {
	if c == nil || db == nil {
		return nil
	}
	required := map[string]bool{}
	for _, def := range c.items {
		for _, name := range def.DefaultSkillNames {
			required[name] = true
		}
	}
	if len(required) == 0 {
		return nil
	}
	var skills []model.Skill
	if err := db.WithContext(ctx).
		Where("org_id IS NULL AND status = ? AND name IN ?", "published", keys(required)).
		Find(&skills).Error; err != nil {
		return fmt.Errorf("load global skills for specialists: %w", err)
	}
	for _, skill := range skills {
		delete(required, skill.Name)
	}
	if len(required) > 0 {
		missing := keys(required)
		sort.Strings(missing)
		return fmt.Errorf("missing global skills for specialists: %s", strings.Join(missing, ", "))
	}
	return nil
}

func loadDefinition(dir string) (Definition, error) {
	path := filepath.Join(dir, "agent.json")
	body, err := os.ReadFile(path)
	if err != nil {
		return Definition{}, fmt.Errorf("read %s: %w", path, err)
	}
	var m manifest
	if err := json.Unmarshal(body, &m); err != nil {
		return Definition{}, fmt.Errorf("parse %s: %w", path, err)
	}
	promptPath := m.PromptPath
	if strings.TrimSpace(promptPath) == "" {
		promptPath = "prompt.md"
	}
	prompt, err := os.ReadFile(filepath.Join(dir, promptPath))
	if err != nil {
		return Definition{}, fmt.Errorf("read specialist prompt %s: %w", filepath.Join(dir, promptPath), err)
	}
	def := Definition{
		Slug:              m.Slug,
		Name:              m.Name,
		Description:       m.Description,
		SpecialistType:    m.SpecialistType,
		Version:           m.Version,
		AutoAttach:        m.AutoAttach,
		DefaultModel:      m.DefaultModel,
		DefaultSkillNames: append([]string(nil), m.DefaultSkillNames...),
		SystemPrompt:      string(prompt),
	}
	return def, validateDefinition(def)
}

func validateDefinition(def Definition) error {
	switch {
	case strings.TrimSpace(def.Slug) == "":
		return fmt.Errorf("specialist slug is required")
	case strings.TrimSpace(def.Name) == "":
		return fmt.Errorf("specialist %q name is required", def.Slug)
	case strings.TrimSpace(def.Description) == "":
		return fmt.Errorf("specialist %q description is required", def.Slug)
	case strings.TrimSpace(def.SpecialistType) == "":
		return fmt.Errorf("specialist %q specialist_type is required", def.Slug)
	case def.Version <= 0:
		return fmt.Errorf("specialist %q version must be positive", def.Slug)
	case strings.TrimSpace(def.DefaultModel) == "":
		return fmt.Errorf("specialist %q default_model is required", def.Slug)
	case strings.TrimSpace(def.SystemPrompt) == "":
		return fmt.Errorf("specialist %q prompt is required", def.Slug)
	}
	if err := registry.Global().ValidateCanonicalModel(strings.TrimSpace(def.DefaultModel)); err != nil {
		return fmt.Errorf("specialist %q default_model: %w", def.Slug, err)
	}
	for _, name := range def.DefaultSkillNames {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("specialist %q default_skill_names contains an empty value", def.Slug)
		}
	}
	return nil
}

func resolveDir(dir string) (string, error) {
	if stat, err := os.Stat(dir); err == nil && stat.IsDir() {
		return dir, nil
	}
	if filepath.IsAbs(dir) {
		return "", fmt.Errorf("global specialists dir %q not found", dir)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get cwd: %w", err)
	}
	for {
		candidate := filepath.Join(cwd, dir)
		if stat, err := os.Stat(candidate); err == nil && stat.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			break
		}
		cwd = parent
	}
	return "", fmt.Errorf("global specialists dir %q not found", dir)
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
