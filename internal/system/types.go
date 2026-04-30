package system

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	providerregistry "github.com/usehiveloop/hiveloop/internal/registry"
)

// ArgType is the value-type of a task argument. Validation against this is
// strict — an int arg won't accept a string-encoded number.
type ArgType string

const (
	ArgString     ArgType = "string"
	ArgInt        ArgType = "int"
	ArgBool       ArgType = "bool"
	ArgStringList ArgType = "string_list"
	// ArgObject is a JSON object decoded as map[string]any. Validation only
	// asserts the top-level shape; deep validation belongs in the task's
	// Resolve hook, where business rules (org-scoping, catalog lookups) live.
	ArgObject ArgType = "object"
	// ArgObjectList is a JSON array of objects, decoded as []any where each
	// element is a map[string]any. Same depth-of-validation contract as
	// ArgObject.
	ArgObjectList ArgType = "object_list"
)

// ArgSpec is one declared input argument for a task.
type ArgSpec struct {
	Name     string
	Type     ArgType
	Required bool
	// MaxLen applies to ArgString and ArgStringList element strings.
	MaxLen int
	// Min/Max apply to ArgInt.
	Min *int
	Max *int
}

// ModelTier instructs the model resolver how to pick the upstream model.
type ModelTier string

const (
	// ModelCheapest picks the lowest-input-cost non-deprecated model on the
	// resolved provider via internal/registry.cheapestModel.
	ModelCheapest ModelTier = "cheapest"
	// ModelDefault uses the registry's default model for the provider.
	ModelDefault ModelTier = "default"
	// ModelNamed pins a specific model id; combined with Task.Model.
	ModelNamed ModelTier = "named"
)

// ResponseFormat controls how the upstream is asked to shape its reply.
type ResponseFormat string

const (
	ResponseText ResponseFormat = "text"
	ResponseJSON ResponseFormat = "json_object"
)

// Task is the full declarative definition of a system task. Each file under
// internal/system/tasks/ exports one of these and self-registers via init().
type Task struct {
	// Name is the URL slug; must be unique across all registered tasks.
	Name string
	// Version is bumped whenever SystemPrompt, UserPromptTemplate, Args,
	// model preference, or response format changes. Used as part of the
	// cache key so a prompt change invalidates cached results without a
	// global flush.
	Version string

	Description string

	// ProviderGroup is fed to credentials.Picker.Pick — "openai", "anthropic",
	// "groq", etc.
	ProviderGroup string

	// ModelTier + Model resolve the upstream model id.
	ModelTier ModelTier
	Model     string // only consulted when ModelTier == ModelNamed

	SystemPrompt       string
	UserPromptTemplate string

	Args []ArgSpec

	MaxOutputTokens int
	Temperature     *float32
	ResponseFormat  ResponseFormat

	DefaultStream bool

	// CacheTTL is the lifetime of cached results for this task. Zero
	// disables caching. Unset (zero default) means "no caching"; tasks that
	// want the default 24h policy must say so explicitly. The handler uses
	// the cache only when CacheTTL > 0.
	CacheTTL time.Duration

	// Resolve, if set, runs after ValidateArgs and before the user template
	// is rendered. It receives the validated raw args and returns the map
	// that actually feeds the template. This is the place to do org-scoped
	// DB lookups (skill IDs → name+description, connection IDs → provider
	// + actions catalog, etc.) so the template can range over rich data.
	//
	// Returning a *ResolveError surfaces a stable machine-readable code to
	// the client; any other error becomes a generic 400.
	Resolve func(ctx context.Context, deps ResolveDeps, args map[string]any) (map[string]any, error)
}

// ResolveDeps are the runtime dependencies a Resolve hook may need. The
// handler injects these from its own constructed deps; tasks pull only what
// they care about.
type ResolveDeps struct {
	DB             *gorm.DB
	OrgID          uuid.UUID
	Registry       *providerregistry.Registry
	ActionsCatalog *catalog.Catalog
}

// ResolveError is the typed error a Resolve hook returns when it wants to
// surface a stable error_code to the client (e.g. "unknown_skill"). The
// handler maps this to a 400 with the error_code field populated.
type ResolveError struct {
	Code    string
	Message string
}

// Error implements the error interface.
func (e *ResolveError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// LLMRequest is the upstream-shape request the forwarder sends. OpenAI
// chat-completions wire shape (other providers added later).
type LLMRequest struct {
	Model          string         `json:"model"`
	Messages       []LLMMessage   `json:"messages"`
	MaxTokens      int            `json:"max_tokens,omitempty"`
	Temperature    *float32       `json:"temperature,omitempty"`
	Stream         bool           `json:"stream,omitempty"`
	ResponseFormat *responseSpec  `json:"response_format,omitempty"`
	StreamOptions  *streamOptions `json:"stream_options,omitempty"`
}

type LLMMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responseSpec struct {
	Type string `json:"type"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// Usage is the normalised token usage we surface to callers and store in the
// cache. Mirrors observe.UsageData but lives here to keep the system package
// importable from tasks without pulling observe.
type Usage struct {
	InputTokens     int `json:"input_tokens"`
	OutputTokens    int `json:"output_tokens"`
	CachedTokens    int `json:"cached_tokens,omitempty"`
	ReasoningTokens int `json:"reasoning_tokens,omitempty"`
}

// CompletionResult is what the forwarder returns on a non-streaming call and
// what the cache stores. Both branches produce one of these at completion.
type CompletionResult struct {
	Text  string `json:"text"`
	Usage Usage  `json:"usage"`
	Model string `json:"model"`
}
