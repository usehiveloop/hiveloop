package hindsight

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// MemoryConfig is the customer-facing memory configuration for an identity.
// Stored as JSONB in Identity.MemoryConfig and exposed via the Identity API.
type MemoryConfig struct {
	Enabled               *bool  `json:"enabled,omitempty"`
	RetainMission         string `json:"retain_mission,omitempty"`
	ReflectMission        string `json:"reflect_mission,omitempty"`
	ObservationsMission   string `json:"observations_mission,omitempty"`
	DispositionSkepticism *int   `json:"disposition_skepticism,omitempty"`
	DispositionLiteralism *int   `json:"disposition_literalism,omitempty"`
	DispositionEmpathy    *int   `json:"disposition_empathy,omitempty"`
}

const (
	defaultRetainMission       = "Extract facts, preferences, decisions, relationships, deadlines, and commitments from conversations. Focus on actionable information. Ignore greetings and filler."
	defaultReflectMission      = "You are a professional assistant with full context of past interactions. Reference past decisions. Be precise."
	defaultObservationsMission = "Identify durable patterns: preferences, processes, ongoing projects, evolving metrics. Track contradictions with timestamps."
	defaultSkepticism          = 3
	defaultLiteralism          = 4
	defaultEmpathy             = 2
)

// DefaultMemoryConfig returns sensible defaults for general-purpose agents.
func DefaultMemoryConfig() MemoryConfig {
	enabled := true
	skep, lit, emp := defaultSkepticism, defaultLiteralism, defaultEmpathy
	return MemoryConfig{
		Enabled:               &enabled,
		RetainMission:         defaultRetainMission,
		ReflectMission:        defaultReflectMission,
		ObservationsMission:   defaultObservationsMission,
		DispositionSkepticism: &skep,
		DispositionLiteralism: &lit,
		DispositionEmpathy:    &emp,
	}
}

// ParseMemoryConfig parses the JSONB field from Identity.MemoryConfig into a MemoryConfig.
// Returns the default config if the JSONB is nil or empty.
func ParseMemoryConfig(j model.JSON) MemoryConfig {
	if len(j) == 0 {
		return DefaultMemoryConfig()
	}
	b, err := json.Marshal(j)
	if err != nil {
		return DefaultMemoryConfig()
	}
	var mc MemoryConfig
	if err := json.Unmarshal(b, &mc); err != nil {
		return DefaultMemoryConfig()
	}
	return mc
}

// IsEnabled returns true unless Enabled is explicitly set to false.
func (m MemoryConfig) IsEnabled() bool {
	return m.Enabled == nil || *m.Enabled
}

// Hash returns a SHA256 hex string of the config for change detection.
func (m MemoryConfig) Hash() string {
	b, _ := json.Marshal(m)
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

// ToBankConfigUpdate builds a Hindsight bank config update request,
// merging customer values with defaults for any unset fields.
func (m MemoryConfig) ToBankConfigUpdate(observationScopes [][]string) *BankConfigUpdate {
	retain := m.RetainMission
	if retain == "" {
		retain = defaultRetainMission
	}
	reflect := m.ReflectMission
	if reflect == "" {
		reflect = defaultReflectMission
	}
	observations := m.ObservationsMission
	if observations == "" {
		observations = defaultObservationsMission
	}
	skep := defaultSkepticism
	if m.DispositionSkepticism != nil {
		skep = *m.DispositionSkepticism
	}
	lit := defaultLiteralism
	if m.DispositionLiteralism != nil {
		lit = *m.DispositionLiteralism
	}
	emp := defaultEmpathy
	if m.DispositionEmpathy != nil {
		emp = *m.DispositionEmpathy
	}

	updates := map[string]any{
		"retain_mission":         retain,
		"reflect_mission":        reflect,
		"observations_mission":   observations,
		"disposition_skepticism": skep,
		"disposition_literalism": lit,
		"disposition_empathy":    emp,
	}
	if len(observationScopes) > 0 {
		updates["observation_scopes"] = observationScopes
	}

	return &BankConfigUpdate{Updates: updates}
}
