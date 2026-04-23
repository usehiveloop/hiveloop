package dispatch

import (
	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// --------------------------------------------------------------------------
// Test helpers exported for use by other packages (e.g. tasks tests).
// These are NOT _test.go functions so they can be imported.
// --------------------------------------------------------------------------

// RouterTriggerForTest creates a RouterTrigger with the given type and
// instructions for use in external package tests.
func RouterTriggerForTest(triggerID, orgID, routerID uuid.UUID, triggerType, instructions string) model.RouterTrigger {
	return model.RouterTrigger{
		ID:           triggerID,
		OrgID:        orgID,
		RouterID:     routerID,
		TriggerType:  triggerType,
		RoutingMode:  "rule",
		Enabled:      true,
		Instructions: instructions,
	}
}

// RouterForTest creates a Router for use in external package tests.
func RouterForTest(routerID, orgID uuid.UUID, persona string, defaultAgentID *uuid.UUID) model.Router {
	return model.Router{
		ID:             routerID,
		OrgID:          orgID,
		Name:           "Zira",
		Persona:        persona,
		DefaultAgentID: defaultAgentID,
	}
}

// RuleForTest creates a catch-all RoutingRule for use in external package tests.
func RuleForTest(agentID uuid.UUID, priority int) model.RoutingRule {
	return model.RoutingRule{
		AgentID:  agentID,
		Priority: priority,
	}
}

// AgentForTest creates a minimal Agent for use in external package tests.
func AgentForTest(agentID uuid.UUID, orgID *uuid.UUID, name string) model.Agent {
	desc := "Test agent: " + name
	return model.Agent{
		ID:          agentID,
		OrgID:       orgID,
		Name:        name,
		Description: &desc,
		Status:      "active",
	}
}
