// Package executor takes AgentDispatch instructions from the router dispatcher
// and creates/continues Bridge conversations with per-conversation MCP tools
// for reply, agent integrations, and shared memory.
package executor

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	"gorm.io/gorm"

	bridgepkg "github.com/usehiveloop/hiveloop/internal/bridge"
	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
	"github.com/usehiveloop/hiveloop/internal/trigger/dispatch"
)

// Executor creates or continues Bridge conversations based on routing decisions.
type Executor struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	signingKey   []byte
	cfg          *config.Config
}

// NewExecutor creates an executor with the dependencies it needs to create
// Bridge conversations and manage sandbox connections.
func NewExecutor(db *gorm.DB, orchestrator *sandbox.Orchestrator, signingKey []byte, cfg *config.Config) *Executor {
	return &Executor{db: db, orchestrator: orchestrator, signingKey: signingKey, cfg: cfg}
}

// Execute processes a slice of AgentDispatch instructions. Same-priority agents
// execute in parallel; lower priority waits for higher.
func (executor *Executor) Execute(ctx context.Context, dispatches []dispatch.AgentDispatch) error {
	if len(dispatches) == 0 {
		return nil
	}

	// Group by priority.
	groups := groupByPriority(dispatches)
	for _, group := range groups {
		var waitGroup sync.WaitGroup
		errors := make([]error, len(group))
		for index, agentDispatch := range group {
			waitGroup.Add(1)
			go func(idx int, agentDisp dispatch.AgentDispatch) {
				defer waitGroup.Done()
				errors[idx] = executor.executeOne(ctx, agentDisp)
			}(index, agentDispatch)
		}
		waitGroup.Wait()

		for _, err := range errors {
			if err != nil {
				slog.Error("executor: agent dispatch failed", "error", err)
			}
		}
	}
	return nil
}

func (executor *Executor) executeOne(ctx context.Context, agentDispatch dispatch.AgentDispatch) error {
	// Continue existing conversation.
	if agentDispatch.RunIntent == "continue" {
		return executor.continueConversation(ctx, agentDispatch)
	}

	// New conversation flow.
	return executor.createConversation(ctx, agentDispatch)
}

func (executor *Executor) continueConversation(ctx context.Context, agentDispatch dispatch.AgentDispatch) error {
	var sb model.Sandbox
	if err := executor.db.Where("id = ?", agentDispatch.ExistingSandboxID).First(&sb).Error; err != nil {
		return fmt.Errorf("loading sandbox for continuation: %w", err)
	}
	client, err := executor.orchestrator.GetBridgeClient(ctx, &sb)
	if err != nil {
		return fmt.Errorf("getting bridge client for continuation: %w", err)
	}

	// Build a concise update message from the new event's refs.
	updateMessage := buildContinuationMessage(agentDispatch)
	return client.SendMessage(ctx, agentDispatch.ExistingConversationID, updateMessage)
}

func (executor *Executor) createConversation(ctx context.Context, agentDispatch dispatch.AgentDispatch) error {
	// 1. Load agent.
	var agent model.Agent
	if err := executor.db.Where("id = ? AND deleted_at IS NULL", agentDispatch.AgentID).First(&agent).Error; err != nil {
		return fmt.Errorf("loading agent %s: %w", agentDispatch.AgentID, err)
	}

	if agent.SandboxID == nil {
		return fmt.Errorf("agent %s has no sandbox assigned", agent.Name)
	}

	// 2. Get Bridge client.
	var sb model.Sandbox
	if err := executor.db.Where("id = ?", *agent.SandboxID).First(&sb).Error; err != nil {
		return fmt.Errorf("loading sandbox for agent %s: %w", agent.Name, err)
	}
	client, err := executor.orchestrator.GetBridgeClient(ctx, &sb)
	if err != nil {
		return fmt.Errorf("getting bridge client for %s: %w", agent.Name, err)
	}

	// 3. Build per-conversation MCP list.
	mcpServers := executor.buildMCPList(agentDispatch)

	// 4. Build provider override from agent's credential.
	provider := executor.buildProvider(&agent)

	// 5. Create conversation with per-conv MCPs.
	conv, err := client.CreateConversationWithOptions(ctx, agent.ID.String(), bridgepkg.CreateConversationRequest{
		Provider:   provider,
		McpServers: &mcpServers,
	})
	if err != nil {
		return fmt.Errorf("creating conversation for %s: %w", agent.Name, err)
	}

	// 6. Store RouterConversation for thread affinity.
	if err := executor.db.Create(&model.RouterConversation{
		OrgID:                agentDispatch.ReplyOrgID,
		RouterTriggerID:      agentDispatch.RouterTriggerID,
		AgentID:              agentDispatch.AgentID,
		ConnectionID:         agentDispatch.ReplyConnectionID,
		ResourceKey:          agentDispatch.ResourceKey,
		BridgeConversationID: conv.ConversationId,
		SandboxID:            *agent.SandboxID,
	}).Error; err != nil {
		slog.Error("executor: failed to store router conversation", "error", err)
	}

	// 7. Build and send instructions.
	instructionSource := "flat_refs"
	if agentDispatch.EnrichedMessage != "" {
		instructionSource = "enriched"
	}
	instructions := buildInstructions(agentDispatch)

	slog.Info("executor: sending instructions",
		"agent", agent.Name,
		"agent_id", agentDispatch.AgentID,
		"conversation_id", conv.ConversationId,
		"instruction_source", instructionSource,
		"instruction_bytes", len(instructions),
	)

	if err := client.SendMessage(ctx, conv.ConversationId, instructions); err != nil {
		return fmt.Errorf("sending instructions to %s: %w", agent.Name, err)
	}

	slog.Info("executor: conversation created",
		"agent", agent.Name,
		"agent_id", agentDispatch.AgentID,
		"conversation_id", conv.ConversationId,
		"resource_key", agentDispatch.ResourceKey,
		"instruction_source", instructionSource,
	)
	return nil
}

// --------------------------------------------------------------------------
// MCP list builder
// --------------------------------------------------------------------------

func (executor *Executor) buildMCPList(agentDispatch dispatch.AgentDispatch) []bridgepkg.McpServerDefinition {
	var servers []bridgepkg.McpServerDefinition

	// Reply MCP: exposes the source channel's write tools.
	if executor.cfg != nil && executor.cfg.MCPBaseURL != "" {
		replyURL := fmt.Sprintf("%s/reply/%s", executor.cfg.MCPBaseURL, agentDispatch.ReplyConnectionID)
		servers = append(servers, bridgepkg.McpServerDefinition{
			Name:      "hiveloop-reply",
			Transport: buildMcpTransport(replyURL, ""),
		})
	}

	// Memory MCP: shared team namespace.
	if agentDispatch.MemoryTeam != "" && executor.cfg != nil && executor.cfg.HindsightAPIURL != "" {
		memoryURL := fmt.Sprintf("%s/memory/%s", executor.cfg.MCPBaseURL, agentDispatch.AgentID)
		servers = append(servers, bridgepkg.McpServerDefinition{
			Name:      "memory",
			Transport: buildMcpTransport(memoryURL, ""),
		})
	}

	return servers
}

func buildMcpTransport(url, token string) bridgepkg.McpTransport {
	var transport bridgepkg.McpTransport
	httpTransport := bridgepkg.McpTransport1{
		Type: bridgepkg.StreamableHttp,
		Url:  url,
	}
	if token != "" {
		headers := map[string]string{"Authorization": "Bearer " + token}
		httpTransport.Headers = &headers
	}
	transport.FromMcpTransport1(httpTransport)
	return transport
}

// --------------------------------------------------------------------------
// Provider override builder
// --------------------------------------------------------------------------

func (executor *Executor) buildProvider(agent *model.Agent) *bridgepkg.ProviderConfig {
	if agent.CredentialID == nil {
		return nil
	}
	// In production, we'd load the credential and build a proper override.
	// For system agents or agents without credentials, return nil (use agent default).
	return nil
}

// --------------------------------------------------------------------------
// Instructions builder
// --------------------------------------------------------------------------

func buildInstructions(agentDispatch dispatch.AgentDispatch) string {
	var builder strings.Builder

	// Persona preamble.
	if agentDispatch.RouterPersona != "" {
		builder.WriteString(agentDispatch.RouterPersona)
		builder.WriteString("\n\n---\n\n")
	}

	// Prefer enriched message over flat refs.
	if agentDispatch.EnrichedMessage != "" {
		builder.WriteString(agentDispatch.EnrichedMessage)
		return builder.String()
	}

	// Fallback: flat refs.
	for key, value := range agentDispatch.Refs {
		builder.WriteString(fmt.Sprintf("%s: %s\n", key, value))
	}

	return builder.String()
}

func buildContinuationMessage(agentDispatch dispatch.AgentDispatch) string {
	var builder strings.Builder
	builder.WriteString("New event on this resource:\n\n")
	for key, value := range agentDispatch.Refs {
		builder.WriteString(fmt.Sprintf("%s: %s\n", key, value))
	}
	return builder.String()
}

// --------------------------------------------------------------------------
// Priority grouping
// --------------------------------------------------------------------------

func groupByPriority(dispatches []dispatch.AgentDispatch) [][]dispatch.AgentDispatch {
	if len(dispatches) == 0 {
		return nil
	}

	sort.Slice(dispatches, func(indexA, indexB int) bool {
		return dispatches[indexA].Priority < dispatches[indexB].Priority
	})

	var groups [][]dispatch.AgentDispatch
	currentPriority := dispatches[0].Priority
	currentGroup := []dispatch.AgentDispatch{dispatches[0]}

	for _, agentDispatch := range dispatches[1:] {
		if agentDispatch.Priority != currentPriority {
			groups = append(groups, currentGroup)
			currentPriority = agentDispatch.Priority
			currentGroup = []dispatch.AgentDispatch{agentDispatch}
		} else {
			currentGroup = append(currentGroup, agentDispatch)
		}
	}
	groups = append(groups, currentGroup)
	return groups
}

// Ensure uuid is used (referenced in AgentDispatch fields).
var _ = uuid.Nil
