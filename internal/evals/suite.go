package evals

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

func LoadSuite(path string) (*Suite, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read suite: %w", err)
	}
	var suite Suite
	if err := yaml.Unmarshal(raw, &suite); err != nil {
		return nil, fmt.Errorf("decode suite: %w", err)
	}
	if err := suite.Validate(); err != nil {
		return nil, err
	}
	return &suite, nil
}

func (s *Suite) Validate() error {
	if strings.TrimSpace(s.ID) == "" {
		return fmt.Errorf("suite id is required")
	}
	if strings.TrimSpace(s.Business.Name) == "" {
		return fmt.Errorf("business.name is required")
	}
	if strings.TrimSpace(s.Business.Profile) == "" {
		return fmt.Errorf("business.profile is required")
	}
	if len(s.Memories) == 0 {
		return fmt.Errorf("at least one memory fixture is required")
	}
	for i, memory := range s.Memories {
		if strings.TrimSpace(memory.Content) == "" {
			return fmt.Errorf("memories[%d].content is required", i)
		}
	}
	if len(s.Cases) == 0 {
		return fmt.Errorf("at least one case is required")
	}
	seen := map[string]bool{}
	for i, c := range s.Cases {
		if strings.TrimSpace(c.ID) == "" {
			return fmt.Errorf("cases[%d].id is required", i)
		}
		if seen[c.ID] {
			return fmt.Errorf("duplicate case id %q", c.ID)
		}
		seen[c.ID] = true
		if strings.TrimSpace(c.Message) == "" {
			return fmt.Errorf("cases[%d].message is required", i)
		}
		if strings.TrimSpace(c.ExpectedInitial) != "" && !validBehavior(c.ExpectedInitial) {
			return fmt.Errorf("cases[%d].expected_initial_behavior is invalid", i)
		}
		if !validBehavior(c.ExpectedBehavior) {
			return fmt.Errorf("cases[%d].expected_behavior is invalid", i)
		}
		if c.ExpectedBehavior == BehaviorDelegate && strings.TrimSpace(c.ExpectedSpecialist) == "" {
			return fmt.Errorf("cases[%d].expected_specialist is required", i)
		}
		if c.FollowUp != nil {
			mode := strings.TrimSpace(c.FollowUp.Mode)
			if mode == "" {
				mode = "llm"
			}
			if mode != "llm" && mode != "static" {
				return fmt.Errorf("cases[%d].follow_up.mode must be llm or static", i)
			}
			if strings.TrimSpace(c.FollowUp.Context) == "" {
				return fmt.Errorf("cases[%d].follow_up.context is required", i)
			}
		}
	}
	return nil
}

func (s Suite) TimeoutFor(c Case) time.Duration {
	seconds := c.TimeoutSeconds
	if seconds <= 0 {
		seconds = s.TimeoutSeconds
	}
	if seconds <= 0 {
		seconds = 120
	}
	return time.Duration(seconds) * time.Second
}

func validBehavior(value string) bool {
	switch strings.TrimSpace(value) {
	case BehaviorDelegate, BehaviorDirect, BehaviorClarify:
		return true
	default:
		return false
	}
}

func businessPrompt(f BusinessFixture) string {
	parts := []string{}
	if strings.TrimSpace(f.Name) != "" {
		parts = append(parts, "Company name: "+strings.TrimSpace(f.Name))
	}
	if strings.TrimSpace(f.Industry) != "" {
		parts = append(parts, "Industry: "+strings.TrimSpace(f.Industry))
	}
	if strings.TrimSpace(f.Profile) != "" {
		parts = append(parts, strings.TrimSpace(f.Profile))
	}
	return strings.Join(parts, "\n")
}
