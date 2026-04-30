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
	ArgObject     ArgType = "object"
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

	ReasoningEffort string

	CacheTTL time.Duration

	// Resolve, when set, runs between ValidateArgs and template render.
	// Returning a *ResolveError surfaces error_code to the client; any
	// other error becomes a generic 400.
	Resolve func(ctx context.Context, deps ResolveDeps, args map[string]any) (map[string]any, error)
}

type ResolveDeps struct {
	DB             *gorm.DB
	OrgID          uuid.UUID
	Registry       *providerregistry.Registry
	ActionsCatalog *catalog.Catalog
}

type ResolveError struct {
	Code    string
	Message string
}

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
	Reasoning      *reasoningSpec `json:"reasoning,omitempty"`
}

type reasoningSpec struct {
	Effort string `json:"effort,omitempty"`
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
