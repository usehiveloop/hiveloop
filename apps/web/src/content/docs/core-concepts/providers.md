---
title: Providers
description: The embedded LLM provider catalog with auto-detection, model metadata, and pricing information sourced from models.dev.
---

# Providers

LLMVault includes an embedded provider and model catalog sourced from [models.dev](https://models.dev). The catalog provides metadata about supported LLM providers, their models, capabilities, and pricing.

## What is the Provider Catalog?

The provider catalog is an embedded JSON file (`models.json`) that is parsed at application startup and held in memory. This design provides:

- **Zero network latency** - No external API calls needed
- **O(1) lookups** - Direct map access by provider ID
- **Build-time embedding** - Catalog is baked into the binary
- **Type safety** - Strongly typed Go structs

```go
//go:embed models.json
var modelsJSON []byte
```

## Provider Structure

```go
type Provider struct {
    ID     string           `json:"id"`      // Unique identifier
    Name   string           `json:"name"`    // Display name
    API    string           `json:"api"`     // API documentation URL
    Doc    string           `json:"doc"`     // Provider documentation URL
    Models map[string]Model `json:"models"`  // Map of model_id → Model
}
```

## Model Structure

```go
type Model struct {
    ID               string      `json:"id"`                // Unique model ID
    Name             string      `json:"name"`              // Display name
    Family           string      `json:"family,omitempty"`  // Model family
    Reasoning        bool        `json:"reasoning,omitempty"`
    ToolCall         bool        `json:"tool_call,omitempty"`
    StructuredOutput bool        `json:"structured_output,omitempty"`
    OpenWeights      bool        `json:"open_weights,omitempty"`
    Knowledge        string      `json:"knowledge,omitempty"`     // Knowledge cutoff date
    ReleaseDate      string      `json:"release_date,omitempty"`
    Modalities       *Modalities `json:"modalities,omitempty"`
    Cost             *Cost       `json:"cost,omitempty"`
    Limit            *Limit      `json:"limit,omitempty"`
    Status           string      `json:"status,omitempty"`
}
```

### Model Capabilities

| Field | Description |
|-------|-------------|
| `Reasoning` | Model supports chain-of-thought reasoning |
| `ToolCall` | Model can call external tools/functions |
| `StructuredOutput` | Model supports JSON mode / structured outputs |
| `OpenWeights` | Model weights are openly available |

### Modalities

```go
type Modalities struct {
    Input  []string `json:"input,omitempty"`   // e.g., ["text", "image"]
    Output []string `json:"output,omitempty"`  // e.g., ["text"]
}
```

### Pricing

```go
type Cost struct {
    Input  float64 `json:"input,omitempty"`   // Price per 1M input tokens
    Output float64 `json:"output,omitempty"`  // Price per 1M output tokens
}
```

### Token Limits

```go
type Limit struct {
    Context int64 `json:"context,omitempty"`  // Context window size
    Output  int64 `json:"output,omitempty"`   // Max output tokens
}
```

## Supported Providers

The catalog includes all major LLM providers:

| Provider ID | Name |
|-------------|------|
| `openai` | OpenAI |
| `anthropic` | Anthropic |
| `google` | Google AI |
| `cohere` | Cohere |
| `mistral` | Mistral AI |
| `groq` | Groq |
| `together` | Together AI |
| `fireworks` | Fireworks AI |
| `perplexity` | Perplexity |
| `azure_openai` | Azure OpenAI |
| `aws_bedrock` | AWS Bedrock |

## API Endpoints

### List All Providers

```bash
GET /v1/providers
```

Response:

```json
[
  {
    "id": "openai",
    "name": "OpenAI",
    "api": "https://platform.openai.com/docs/api-reference",
    "doc": "https://platform.openai.com/docs",
    "model_count": 12
  },
  {
    "id": "anthropic",
    "name": "Anthropic",
    "api": "https://docs.anthropic.com/claude/reference",
    "doc": "https://docs.anthropic.com",
    "model_count": 5
  }
]
```

### Get Provider Detail

```bash
GET /v1/providers/{id}
```

Response:

```json
{
  "id": "openai",
  "name": "OpenAI",
  "api": "https://platform.openai.com/docs/api-reference",
  "doc": "https://platform.openai.com/docs",
  "models": [
    {
      "id": "gpt-4o",
      "name": "GPT-4o",
      "family": "gpt-4o",
      "reasoning": false,
      "tool_call": true,
      "structured_output": true,
      "open_weights": false,
      "knowledge": "2023-10",
      "release_date": "2024-05-13",
      "modalities": {
        "input": ["text", "image"],
        "output": ["text"]
      },
      "cost": {
        "input": 5.0,
        "output": 15.0
      },
      "limit": {
        "context": 128000,
        "output": 4096
      },
      "status": "active"
    }
  ]
}
```

### List Provider Models

```bash
GET /v1/providers/{id}/models
```

Returns just the models array, sorted by model ID.

## Provider Verification

When creating credentials, the provider ID is validated against the catalog:

```go
if _, ok := registry.Global().GetProvider(req.ProviderID); !ok {
    writeJSON(w, http.StatusBadRequest, map[string]string{
        "error": fmt.Sprintf("unknown provider_id %q", req.ProviderID)
    })
    return
}
```

## Creating Credentials with Providers

```bash
curl -X POST https://api.llmvault.dev/v1/credentials \
  -H "Authorization: Bearer {org_token}" \
  -H "Content-Type: application/json" \
  -d '{
    "provider_id": "openai",
    "base_url": "https://api.openai.com/v1",
    "auth_scheme": "bearer",
    "api_key": "sk-...",
    "label": "OpenAI Production"
  }'
```

## Custom Providers

While the catalog covers major providers, you can use any OpenAI-compatible endpoint by specifying a custom `base_url` and `provider_id`:

```bash
curl -X POST https://api.llmvault.dev/v1/credentials \
  -H "Authorization: Bearer {org_token}" \
  -d '{
    "provider_id": "custom",
    "base_url": "https://api.mycustomllm.com/v1",
    "auth_scheme": "bearer",
    "api_key": "my-key"
  }'
```

The `provider_id` must exist in the catalog, but you can use any supported provider's ID for custom endpoints with compatible APIs.

## Model Selection

When making proxy requests, use standard model IDs:

```bash
curl https://api.llmvault.dev/v1/proxy/v1/chat/completions \
  -H "Authorization: Bearer ptok_..." \
  -d '{
    "model": "gpt-4o",
    "messages": [...]
  }'
```

The model ID is passed through transparently to the upstream provider.

## Catalog Access

The global registry singleton provides thread-safe access:

```go
import "github.com/llmvault/llmvault/internal/registry"

// Get provider
provider, ok := registry.Global().GetProvider("openai")

// Get all providers
allProviders := registry.Global().AllProviders()

// Get counts
providerCount := registry.Global().ProviderCount()
modelCount := registry.Global().ModelCount()
```

## Catalog Updates

The embedded catalog is updated by:

1. Updating the `models.json` file
2. Rebuilding the application
3. Redeploying

The catalog is parsed once at startup using `sync.Once`:

```go
var (
    globalRegistry *Registry
    initOnce       sync.Once
)

func Global() *Registry {
    initOnce.Do(func() {
        globalRegistry = mustParse(modelsJSON)
    })
    return globalRegistry
}
```

## Provider Metadata Fields

### Status Values

| Status | Description |
|--------|-------------|
| `active` | Currently available |
| `deprecated` | Scheduled for removal |
| `preview` | Beta/early access |

### Knowledge Cutoff

The `knowledge` field indicates the training data cutoff date:

- `2023-10` - Knowledge current through October 2023
- `2024-04` - Knowledge current through April 2024

### Release Dates

ISO 8601 format: `YYYY-MM-DD`

Example: `2024-05-13` for GPT-4o's release date.

## Integration with MCP

The actions catalog (separate from the provider catalog) defines available actions for MCP integrations:

```
internal/mcp/catalog/providers/*.actions.json
```

Each provider can have an associated actions file defining:
- Available actions (e.g., `chat.completions`, `embeddings`)
- Resource types (e.g., `models`, `assistants`)
- Execution configuration

## Security Considerations

1. **No secrets in catalog** - The embedded catalog contains only public metadata
2. **Build-time embedding** - Catalog cannot be modified at runtime
3. **Validation on credential creation** - Unknown provider IDs are rejected
4. **URL validation** - Base URLs are validated against SSRF attacks
