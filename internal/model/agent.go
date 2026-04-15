package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type Agent struct {
	ID                uuid.UUID        `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID             *uuid.UUID       `gorm:"type:uuid;index:idx_agent_org_id"` // nil for system agents
	Org               *Org             `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	IdentityID        *uuid.UUID       `gorm:"type:uuid;index"`
	Identity          *Identity        `gorm:"foreignKey:IdentityID;constraint:OnDelete:SET NULL"`
	Name              string           `gorm:"not null"`
	Description       *string          `gorm:"type:text"`
	CredentialID      *uuid.UUID       `gorm:"type:uuid;index"` // nil for system agents
	Credential        *Credential      `gorm:"foreignKey:CredentialID;constraint:OnDelete:SET NULL"`
	SandboxType       string           `gorm:"not null"` // "dedicated" or "shared"
	SandboxID         *uuid.UUID       `gorm:"type:uuid;index"` // set for shared agents (points to pool sandbox)
	SandboxTemplateID *uuid.UUID       `gorm:"type:uuid"`
	SandboxTemplate   *SandboxTemplate `gorm:"foreignKey:SandboxTemplateID;constraint:OnDelete:SET NULL"`

	// Bridge AgentDefinition fields
	SystemPrompt    string             `gorm:"type:text;not null"`
	ProviderPrompts ProviderPromptsMap `gorm:"type:jsonb;not null;default:'{}'"` // map[provider_group] -> {system_prompt, model}
	Instructions    *string `gorm:"type:text"`                        // optional markdown instructions for auto-starting runs
	Model           string  `gorm:"not null"`                         // must match credential's provider
	Tools        JSON   `gorm:"type:jsonb;not null;default:'{}'"`
	McpServers   JSON   `gorm:"type:jsonb;not null;default:'{}'"`
	Skills       JSON   `gorm:"type:jsonb;not null;default:'{}'"`
	Integrations JSON   `gorm:"type:jsonb;not null;default:'{}'"` // selected integration IDs/configs
	AgentConfig  JSON   `gorm:"type:jsonb;not null;default:'{}'"` // max_tokens, max_turns, temperature, etc.
	Permissions  JSON   `gorm:"type:jsonb;not null;default:'{}'"` // tool permission overrides
	Team         string `gorm:"not null;default:''"` // team tag for memory scoping (e.g. "engineering", "sales")
	SharedMemory bool   `gorm:"not null;default:false"` // can store shared memories visible to all agents in identity

	// Sandbox setup (dedicated agents only — ignored for shared agents)
	SandboxTools     pq.StringArray `gorm:"type:text[];default:'{}'"`  // enabled sandbox tools (e.g. "chrome", "codedb", "codebase-memory")
	SetupCommands    pq.StringArray `gorm:"type:text[];default:'{}'"`  // shell commands run on dedicated sandbox creation
	EncryptedEnvVars []byte         `gorm:"type:bytea"`                // AES-256-GCM encrypted JSON map of env vars

	Status        string `gorm:"not null;default:'active'"` // active, archived
	AgentType     string `gorm:"not null;default:'agent';index"` // "agent" or "subagent"
	IsSystem      bool   `gorm:"not null;default:false;index"`
	ProviderGroup string `gorm:"not null;default:''"` // e.g. "anthropic", "openai", "gemini" — set for system agents
	DeletedAt     *time.Time `gorm:"index"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (Agent) TableName() string { return "agents" }

const (
	AgentTypeAgent    = "agent"
	AgentTypeSubagent = "subagent"
)

// SandboxToolDefinition describes a tool/service that can be enabled in a dedicated sandbox.
type SandboxToolDefinition struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ValidSandboxTools is the canonical list of sandbox tools the platform supports.
var ValidSandboxTools = []SandboxToolDefinition{
	{ID: "chrome", Name: "Chrome browser", Description: "Headless Chrome for web scraping, testing, and browser automation via agent-browser."},
	{ID: "codedb", Name: "CodeDB", Description: "Code intelligence tools for searching, navigating, and understanding codebases."},
	{ID: "codebase-memory", Name: "Codebase memory", Description: "MCP server that builds a structural code graph from ASTs for deep code understanding."},
}

// validSandboxToolIDs is a set for fast validation lookups.
var validSandboxToolIDs = func() map[string]bool {
	result := make(map[string]bool, len(ValidSandboxTools))
	for _, tool := range ValidSandboxTools {
		result[tool.ID] = true
	}
	return result
}()

// ValidateSandboxTools checks that all provided tool IDs are recognized.
// Returns the first invalid ID, or empty string if all are valid.
func ValidateSandboxTools(tools []string) string {
	for _, tool := range tools {
		if !validSandboxToolIDs[tool] {
			return tool
		}
	}
	return ""
}

// BuiltInToolDefinition describes a built-in tool that can be granted to an agent.
type BuiltInToolDefinition struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Locked      bool   `json:"locked"` // true = cannot be toggled off by the user
}

// ValidBuiltInTools is the canonical list of all built-in tools available in Bridge.
// Used for: the frontend tool picker, permission key validation, and forge tool mocking.
var ValidBuiltInTools = []BuiltInToolDefinition{
	// ── Filesystem ──
	{ID: "Read", Name: "Read file", Description: "Read file contents with optional line range and hash-based caching.", Category: "filesystem"},
	{ID: "write", Name: "Write file", Description: "Create or overwrite a file with new content.", Category: "filesystem"},
	{ID: "edit", Name: "Edit file", Description: "Apply targeted edits to a file using search-and-replace.", Category: "filesystem"},
	{ID: "multiedit", Name: "Multi-edit file", Description: "Apply multiple edits to a single file in one call.", Category: "filesystem"},
	{ID: "apply_patch", Name: "Apply patch", Description: "Apply a unified diff patch to one or more files.", Category: "filesystem"},
	{ID: "Glob", Name: "Glob", Description: "Find files matching a glob pattern.", Category: "filesystem"},
	{ID: "Grep", Name: "Grep", Description: "Search file contents using regex patterns.", Category: "filesystem"},
	{ID: "LS", Name: "List directory", Description: "List files and directories at a given path.", Category: "filesystem"},

	// ── Shell ──
	{ID: "bash", Name: "Bash", Description: "Execute shell commands and return output.", Category: "shell"},

	// ── Web ──
	{ID: "web_fetch", Name: "Fetch URL", Description: "Fetch content from a URL and convert to markdown, text, or HTML.", Category: "web"},
	{ID: "web_search", Name: "Web search", Description: "Search the web and return results with titles, descriptions, and URLs.", Category: "web"},
	{ID: "web_crawl", Name: "Crawl website", Description: "Crawl a website following links from a starting URL.", Category: "web"},
	{ID: "web_get_links", Name: "Get links", Description: "Extract all links from a webpage.", Category: "web"},
	{ID: "web_screenshot", Name: "Screenshot", Description: "Take a screenshot of a webpage as base64-encoded PNG.", Category: "web"},
	{ID: "web_transform", Name: "Transform HTML", Description: "Convert HTML to markdown or plain text without HTTP requests.", Category: "web"},

	// ── Agent orchestration ──
	{ID: "agent", Name: "Agent", Description: "Launch a clone of yourself to handle a focused task autonomously.", Category: "orchestration"},
	{ID: "sub_agent", Name: "Sub-agent", Description: "Launch a named subagent to handle complex multistep tasks.", Category: "orchestration"},
	{ID: "parallel_agent", Name: "Parallel agent", Description: "Spawn multiple subagents in parallel and wait for all to complete.", Category: "orchestration"},
	{ID: "batch", Name: "Batch", Description: "Execute multiple independent tool calls concurrently.", Category: "orchestration"},
	{ID: "join", Name: "Join", Description: "Wait for multiple background subagent tasks to complete.", Category: "orchestration"},

	// ── Task management ──
	{ID: "todowrite", Name: "Write tasks", Description: "Create and manage a structured task list for the current session.", Category: "tasks"},
	{ID: "todoread", Name: "Read tasks", Description: "Read the current task list with statuses and priorities.", Category: "tasks"},

	// ── Journal ──
	{ID: "journal_write", Name: "Write journal", Description: "Write an entry to the persistent conversation journal.", Category: "journal", Locked: true},
	{ID: "journal_read", Name: "Read journal", Description: "Read all journal entries including checkpoint summaries.", Category: "journal", Locked: true},

	// ── Code intelligence ──
	{ID: "lsp", Name: "LSP", Description: "Language Server Protocol operations for code navigation and diagnostics.", Category: "code_intelligence"},
	{ID: "skill", Name: "Skill", Description: "Execute a skill within the conversation.", Category: "code_intelligence"},

	// ── Memory ──
	{ID: "memory_recall", Name: "Recall memory", Description: "Search long-term memory for relevant context from past conversations.", Category: "memory", Locked: true},
	{ID: "memory_retain", Name: "Retain memory", Description: "Store important information to long-term memory.", Category: "memory", Locked: true},
	{ID: "memory_reflect", Name: "Reflect on memory", Description: "Get a synthesized answer by analyzing full memory.", Category: "memory", Locked: true},

	// ── CodeDB ──
	{ID: "codedb_tree", Name: "File tree", Description: "Get the full file tree with language detection, line counts, and symbol counts.", Category: "codedb"},
	{ID: "codedb_outline", Name: "Outline", Description: "Get structural outline of a file: functions, structs, imports with line numbers.", Category: "codedb"},
	{ID: "codedb_symbol", Name: "Find symbol", Description: "Find where a symbol is defined across the codebase.", Category: "codedb"},
	{ID: "codedb_search", Name: "Search code", Description: "Trigram-accelerated full-text search across all indexed files.", Category: "codedb"},
	{ID: "codedb_query", Name: "Query pipeline", Description: "Composable search pipeline — chain find, search, filter, outline, read steps.", Category: "codedb"},
	{ID: "codedb_find", Name: "Find file", Description: "Fuzzy file search with typo-tolerant subsequence matching.", Category: "codedb"},
	{ID: "codedb_read", Name: "Read indexed", Description: "Read file contents from the index with line ranges and caching.", Category: "codedb"},
	{ID: "codedb_deps", Name: "Dependencies", Description: "Get reverse dependencies: which files import the given file.", Category: "codedb"},
	{ID: "codedb_word", Name: "Word lookup", Description: "O(1) inverted index exact word lookup across the codebase.", Category: "codedb"},
	{ID: "codedb_edit", Name: "Edit file", Description: "Apply a line-based edit to a file: replace, insert, or delete.", Category: "codedb"},
	{ID: "codedb_bundle", Name: "Bundle queries", Description: "Batch multiple read-only queries in one call (max 20).", Category: "codedb"},
	{ID: "codedb_snapshot", Name: "Snapshot", Description: "Get the full pre-rendered snapshot of the codebase as JSON.", Category: "codedb"},
	{ID: "codedb_status", Name: "Index status", Description: "Get current index status: file count and sequence number.", Category: "codedb"},
	{ID: "codedb_changes", Name: "Changes", Description: "Get files that changed since a sequence number.", Category: "codedb"},
	{ID: "codedb_hot", Name: "Hot files", Description: "Get the most recently modified files.", Category: "codedb"},
	{ID: "codedb_index", Name: "Index folder", Description: "Index a local folder and create a codedb.snapshot.", Category: "codedb"},
	{ID: "codedb_remote", Name: "Remote query", Description: "Query any GitHub repo via cloud intelligence without cloning.", Category: "codedb"},
	{ID: "codedb_projects", Name: "List projects", Description: "List all locally indexed projects on this machine.", Category: "codedb"},
}

// validBuiltInToolIDs is a set for fast validation lookups.
var validBuiltInToolIDs = func() map[string]bool {
	result := make(map[string]bool, len(ValidBuiltInTools))
	for _, tool := range ValidBuiltInTools {
		result[tool.ID] = true
	}
	return result
}()

// BuiltInToolIDs returns just the ID strings from the registry.
func BuiltInToolIDs() []string {
	ids := make([]string, len(ValidBuiltInTools))
	for index, tool := range ValidBuiltInTools {
		ids[index] = tool.ID
	}
	return ids
}

// ValidatePermissionKeys checks that all keys in a permissions map are valid built-in tool IDs.
// Returns the first invalid key, or empty string if all are valid.
func ValidatePermissionKeys(permissions map[string]string) string {
	for key := range permissions {
		if !validBuiltInToolIDs[key] {
			return key
		}
	}
	return ""
}
