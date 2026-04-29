// Code generated from models.dev catalog snapshot — manually curated.
// To extend: add provider/model entries below. Each entry must reference
// a real, manually-tested model. Run `go test ./internal/registry/...` after.
//
// Inclusion policy: every model with release_date >= 2025-11 from each
// curated provider is shipped. Providers with no recent qualifying models
// (e.g. cohere, groq) fall back to their top 5 most-recent tool-call models
// so the catalog is never empty for any provider in the allow-list.
package registry

// curatedProviders is the static allow-list backing registry.Global().
// Replaces the previous //go:embed models.json approach with a hand-
// maintained Go literal so additions go through code review.
var curatedProviders = []Provider{

	{ // anthropic — Anthropic
		ID:   "anthropic",
		Name: "Anthropic",
		Doc:  "https://docs.anthropic.com/en/docs/about-claude/models",
		Models: map[string]Model{
			"claude-sonnet-4-6": {
				ID:          "claude-sonnet-4-6",
				Name:        "Claude Sonnet 4.6",
				Family:      "claude-sonnet",
				Reasoning:   true,
				ToolCall:    true,
				Knowledge:   "2025-08",
				ReleaseDate: "2026-02-17",
				Modalities: &Modalities{
					Input:  []string{"text", "image", "pdf"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  3,
					Output: 15,
				},
				Limit: &Limit{
					Context: 200000,
					Output:  64000,
				},
			},
			"claude-sonnet-4-5": {
				ID:          "claude-sonnet-4-5",
				Name:        "Claude Sonnet 4.5 (latest)",
				Family:      "claude-sonnet",
				Reasoning:   true,
				ToolCall:    true,
				Knowledge:   "2025-07-31",
				ReleaseDate: "2025-09-29",
				Modalities: &Modalities{
					Input:  []string{"text", "image", "pdf"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  3,
					Output: 15,
				},
				Limit: &Limit{
					Context: 200000,
					Output:  64000,
				},
			},
			"claude-3-5-haiku-latest": {
				ID:          "claude-3-5-haiku-latest",
				Name:        "Claude Haiku 3.5 (latest)",
				Family:      "claude-haiku",
				ToolCall:    true,
				Knowledge:   "2024-07-31",
				ReleaseDate: "2024-10-22",
				Modalities: &Modalities{
					Input:  []string{"text", "image", "pdf"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.8,
					Output: 4,
				},
				Limit: &Limit{
					Context: 200000,
					Output:  8192,
				},
			},
			"claude-3-5-haiku-20241022": {
				ID:          "claude-3-5-haiku-20241022",
				Name:        "Claude Haiku 3.5",
				Family:      "claude-haiku",
				ToolCall:    true,
				Knowledge:   "2024-07-31",
				ReleaseDate: "2024-10-22",
				Modalities: &Modalities{
					Input:  []string{"text", "image", "pdf"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.8,
					Output: 4,
				},
				Limit: &Limit{
					Context: 200000,
					Output:  8192,
				},
			},
			"claude-opus-4-6": {
				ID:          "claude-opus-4-6",
				Name:        "Claude Opus 4.6",
				Family:      "claude-opus",
				Reasoning:   true,
				ToolCall:    true,
				Knowledge:   "2025-05",
				ReleaseDate: "2026-02-05",
				Modalities: &Modalities{
					Input:  []string{"text", "image", "pdf"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  5,
					Output: 25,
				},
				Limit: &Limit{
					Context: 200000,
					Output:  128000,
				},
			},
			"claude-opus-4-5": {
				ID:          "claude-opus-4-5",
				Name:        "Claude Opus 4.5 (latest)",
				Family:      "claude-opus",
				Reasoning:   true,
				ToolCall:    true,
				Knowledge:   "2025-03-31",
				ReleaseDate: "2025-11-24",
				Modalities: &Modalities{
					Input:  []string{"text", "image", "pdf"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  5,
					Output: 25,
				},
				Limit: &Limit{
					Context: 200000,
					Output:  64000,
				},
			},
			"claude-opus-4-5-20251101": {
				ID:          "claude-opus-4-5-20251101",
				Name:        "Claude Opus 4.5",
				Family:      "claude-opus",
				Reasoning:   true,
				ToolCall:    true,
				Knowledge:   "2025-03-31",
				ReleaseDate: "2025-11-01",
				Modalities: &Modalities{
					Input:  []string{"text", "image", "pdf"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  5,
					Output: 25,
				},
				Limit: &Limit{
					Context: 200000,
					Output:  64000,
				},
			},
		},
	},
	// cohere, deepseek removed — not in supported provider set
	{ // crof — Crof
		ID:   "crof",
		Name: "Crof",
		API:  "https://crof.ai/v1",
		Models: map[string]Model{
			"kimi-k2.5": {
				ID:          "kimi-k2.5",
				Name:        "Kimi K2.5",
				Family:      "kimi",
				ToolCall:    true,
				OpenWeights: true,
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.35,
					Output: 1.7,
				},
				Limit: &Limit{
					Context: 262144,
					Output:  262144,
				},
			},
			"greg": {
				ID:       "greg",
				Name:     "Greg",
				Family:   "greg",
				ToolCall: true,
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.3,
					Output: 0.3,
				},
				Limit: &Limit{
					Context: 200000,
					Output:  200000,
				},
			},
			"kimi-k2.5-lightning": {
				ID:          "kimi-k2.5-lightning",
				Name:        "Kimi K2.5 (Lightning)",
				Family:      "kimi",
				Reasoning:   true,
				ToolCall:    true,
				OpenWeights: true,
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  1,
					Output: 3,
				},
				Limit: &Limit{
					Context: 131072,
					Output:  32768,
				},
			},
			"glm-5.1": {
				ID:          "glm-5.1",
				Name:        "GLM 5.1",
				Family:      "glm",
				ToolCall:    true,
				OpenWeights: true,
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.45,
					Output: 2.1,
				},
				Limit: &Limit{
					Context: 202752,
					Output:  202752,
				},
			},
			"glm-5.1-precision": {
				ID:          "glm-5.1-precision",
				Name:        "GLM 5.1 (Precision)",
				Family:      "glm",
				ToolCall:    true,
				OpenWeights: true,
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.8,
					Output: 2.9,
				},
				Limit: &Limit{
					Context: 202752,
					Output:  202752,
				},
			},
			"glm-5": {
				ID:          "glm-5",
				Name:        "GLM 5",
				Family:      "glm",
				ToolCall:    true,
				OpenWeights: true,
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.48,
					Output: 1.9,
				},
				Limit: &Limit{
					Context: 202752,
					Output:  202752,
				},
			},
			"glm-4.7": {
				ID:          "glm-4.7",
				Name:        "GLM 4.7",
				Family:      "glm",
				ToolCall:    true,
				OpenWeights: true,
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.25,
					Output: 1.1,
				},
				Limit: &Limit{
					Context: 202752,
					Output:  202752,
				},
			},
			"glm-4.7-flash": {
				ID:          "glm-4.7-flash",
				Name:        "GLM 4.7 Flash",
				Family:      "glm-flash",
				ToolCall:    true,
				OpenWeights: true,
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Limit: &Limit{
					Context: 202752,
					Output:  131072,
				},
			},
			"gemma-4-31b-it": {
				ID:          "gemma-4-31b-it",
				Name:        "Gemma 4 31B",
				Family:      "gemma",
				ToolCall:    true,
				OpenWeights: true,
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.1,
					Output: 0.3,
				},
				Limit: &Limit{
					Context: 262144,
					Output:  262144,
				},
			},
			"minimax-m2.5": {
				ID:          "minimax-m2.5",
				Name:        "MiniMax M2.5",
				Family:      "minimax",
				ToolCall:    true,
				OpenWeights: true,
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.11,
					Output: 0.95,
				},
				Limit: &Limit{
					Context: 204800,
					Output:  131072,
				},
			},
			"qwen3.5-397b-a17b": {
				ID:          "qwen3.5-397b-a17b",
				Name:        "Qwen3.5 397B A17B",
				Family:      "qwen",
				Reasoning:   true,
				ToolCall:    true,
				OpenWeights: true,
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.35,
					Output: 1.75,
				},
				Limit: &Limit{
					Context: 262144,
					Output:  262144,
				},
			},
			"qwen3.5-9b": {
				ID:          "qwen3.5-9b",
				Name:        "Qwen3.5 9B",
				Family:      "qwen",
				Reasoning:   true,
				ToolCall:    true,
				OpenWeights: true,
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Limit: &Limit{
					Context: 262144,
					Output:  262144,
				},
			},
			"qwen3.5-9b-chat": {
				ID:          "qwen3.5-9b-chat",
				Name:        "Qwen3.5 9B (Chat)",
				Family:      "qwen",
				Reasoning:   true,
				ToolCall:    true,
				OpenWeights: true,
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.04,
					Output: 0.15,
				},
				Limit: &Limit{
					Context: 262144,
					Output:  262144,
				},
			},
			"deepseek-v3.2": {
				ID:          "deepseek-v3.2",
				Name:        "DeepSeek V3.2",
				Family:      "deepseek",
				ToolCall:    true,
				OpenWeights: true,
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.28,
					Output: 0.38,
				},
				Limit: &Limit{
					Context: 163840,
					Output:  163840,
				},
			},
		},
	},
	{ // fireworks-ai — Fireworks AI
		ID:   "fireworks-ai",
		Name: "Fireworks AI",
		API:  "https://api.fireworks.ai/inference/v1/",
		Doc:  "https://fireworks.ai/docs/",
		Models: map[string]Model{
			"accounts/fireworks/models/minimax-m2p5": {
				ID:          "accounts/fireworks/models/minimax-m2p5",
				Name:        "MiniMax-M2.5",
				Family:      "minimax",
				Reasoning:   true,
				ToolCall:    true,
				OpenWeights: true,
				ReleaseDate: "2026-02-12",
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.3,
					Output: 1.2,
				},
				Limit: &Limit{
					Context: 196608,
					Output:  196608,
				},
			},
			"accounts/fireworks/models/glm-5": {
				ID:          "accounts/fireworks/models/glm-5",
				Name:        "GLM 5",
				Family:      "glm",
				Reasoning:   true,
				ToolCall:    true,
				OpenWeights: true,
				ReleaseDate: "2026-02-11",
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  1,
					Output: 3.2,
				},
				Limit: &Limit{
					Context: 202752,
					Output:  131072,
				},
			},
			"accounts/fireworks/models/kimi-k2p5": {
				ID:          "accounts/fireworks/models/kimi-k2p5",
				Name:        "Kimi K2.5",
				Family:      "kimi-thinking",
				Reasoning:   true,
				ToolCall:    true,
				OpenWeights: true,
				Knowledge:   "2025-01",
				ReleaseDate: "2026-01-27",
				Modalities: &Modalities{
					Input:  []string{"text", "image", "video"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.6,
					Output: 3,
				},
				Limit: &Limit{
					Context: 256000,
					Output:  256000,
				},
			},
			"accounts/fireworks/models/minimax-m2p1": {
				ID:          "accounts/fireworks/models/minimax-m2p1",
				Name:        "MiniMax-M2.1",
				Family:      "minimax",
				Reasoning:   true,
				ToolCall:    true,
				OpenWeights: true,
				ReleaseDate: "2025-12-23",
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.3,
					Output: 1.2,
				},
				Limit: &Limit{
					Context: 200000,
					Output:  200000,
				},
			},
			"accounts/fireworks/models/glm-4p7": {
				ID:          "accounts/fireworks/models/glm-4p7",
				Name:        "GLM 4.7",
				Family:      "glm",
				Reasoning:   true,
				ToolCall:    true,
				OpenWeights: true,
				Knowledge:   "2025-04",
				ReleaseDate: "2025-12-22",
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.6,
					Output: 2.2,
				},
				Limit: &Limit{
					Context: 198000,
					Output:  198000,
				},
			},
			"accounts/fireworks/models/kimi-k2-thinking": {
				ID:          "accounts/fireworks/models/kimi-k2-thinking",
				Name:        "Kimi K2 Thinking",
				Family:      "kimi-thinking",
				Reasoning:   true,
				ToolCall:    true,
				OpenWeights: true,
				ReleaseDate: "2025-11-06",
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.6,
					Output: 2.5,
				},
				Limit: &Limit{
					Context: 256000,
					Output:  256000,
				},
			},
		},
	},
	{ // google — Google
		ID:   "google",
		Name: "Google",
		Doc:  "https://ai.google.dev/gemini-api/docs/pricing",
		Models: map[string]Model{
			"gemini-3.1-flash-lite-preview": {
				ID:          "gemini-3.1-flash-lite-preview",
				Name:        "Gemini 3.1 Flash Lite Preview",
				Family:      "gemini-flash-lite",
				Reasoning:   true,
				ToolCall:    true,
				Knowledge:   "2025-01",
				ReleaseDate: "2026-03-03",
				Modalities: &Modalities{
					Input:  []string{"text", "image", "video", "audio", "pdf"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.5,
					Output: 3,
				},
				Limit: &Limit{
					Context: 1048576,
					Output:  65536,
				},
			},
			"gemini-3.1-flash-image-preview": {
				ID:          "gemini-3.1-flash-image-preview",
				Name:        "Gemini 3.1 Flash Image (Preview)",
				Family:      "gemini-flash",
				Reasoning:   true,
				Knowledge:   "2025-01",
				ReleaseDate: "2026-02-26",
				Modalities: &Modalities{
					Input:  []string{"text", "image", "pdf"},
					Output: []string{"text", "image"},
				},
				Cost: &Cost{
					Input:  0.25,
					Output: 60,
				},
				Limit: &Limit{
					Context: 131072,
					Output:  32768,
				},
			},
			"gemini-3.1-pro-preview-customtools": {
				ID:          "gemini-3.1-pro-preview-customtools",
				Name:        "Gemini 3.1 Pro Preview Custom Tools",
				Family:      "gemini-pro",
				Reasoning:   true,
				ToolCall:    true,
				Knowledge:   "2025-01",
				ReleaseDate: "2026-02-19",
				Modalities: &Modalities{
					Input:  []string{"text", "image", "video", "audio", "pdf"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  2,
					Output: 12,
				},
				Limit: &Limit{
					Context: 1048576,
					Output:  65536,
				},
			},
			"gemini-3.1-pro-preview": {
				ID:          "gemini-3.1-pro-preview",
				Name:        "Gemini 3.1 Pro Preview",
				Family:      "gemini-pro",
				Reasoning:   true,
				ToolCall:    true,
				Knowledge:   "2025-01",
				ReleaseDate: "2026-02-19",
				Modalities: &Modalities{
					Input:  []string{"text", "image", "video", "audio", "pdf"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  2,
					Output: 12,
				},
				Limit: &Limit{
					Context: 1048576,
					Output:  65536,
				},
			},
			"gemini-3-flash-preview": {
				ID:          "gemini-3-flash-preview",
				Name:        "Gemini 3 Flash Preview",
				Family:      "gemini-flash",
				Reasoning:   true,
				ToolCall:    true,
				Knowledge:   "2025-01",
				ReleaseDate: "2025-12-17",
				Modalities: &Modalities{
					Input:  []string{"text", "image", "video", "audio", "pdf"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.5,
					Output: 3,
				},
				Limit: &Limit{
					Context: 1048576,
					Output:  65536,
				},
			},
			"gemini-3-pro-preview": {
				ID:          "gemini-3-pro-preview",
				Name:        "Gemini 3 Pro Preview",
				Family:      "gemini-pro",
				Reasoning:   true,
				ToolCall:    true,
				Knowledge:   "2025-01",
				ReleaseDate: "2025-11-18",
				Modalities: &Modalities{
					Input:  []string{"text", "image", "video", "audio", "pdf"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  2,
					Output: 12,
				},
				Limit: &Limit{
					Context: 1000000,
					Output:  64000,
				},
			},
			"gemini-2.5-flash": {
				ID:               "gemini-2.5-flash",
				Name:             "Gemini 2.5 Flash",
				Family:           "gemini-flash",
				Reasoning:        true,
				ToolCall:         true,
				StructuredOutput: true,
				Knowledge:        "2025-01",
				ReleaseDate:      "2025-03-20",
				Modalities: &Modalities{
					Input:  []string{"text", "image", "audio", "video", "pdf"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.3,
					Output: 2.5,
				},
				Limit: &Limit{
					Context: 1048576,
					Output:  65536,
				},
			},
		},
	},
	{ // groq — Groq
		ID:   "groq",
		Name: "Groq",
		Doc:  "https://console.groq.com/docs/models",
		Models: map[string]Model{
			"moonshotai/kimi-k2-instruct-0905": {
				ID:          "moonshotai/kimi-k2-instruct-0905",
				Name:        "Kimi K2 Instruct 0905",
				Family:      "kimi",
				ToolCall:    true,
				OpenWeights: true,
				Knowledge:   "2024-10",
				ReleaseDate: "2025-09-05",
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  1,
					Output: 3,
				},
				Limit: &Limit{
					Context: 262144,
					Output:  16384,
				},
			},
			"openai/gpt-oss-20b": {
				ID:          "openai/gpt-oss-20b",
				Name:        "GPT OSS 20B",
				Family:      "gpt-oss",
				Reasoning:   true,
				ToolCall:    true,
				OpenWeights: true,
				ReleaseDate: "2025-08-05",
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.075,
					Output: 0.3,
				},
				Limit: &Limit{
					Context: 131072,
					Output:  65536,
				},
			},
			"openai/gpt-oss-120b": {
				ID:          "openai/gpt-oss-120b",
				Name:        "GPT OSS 120B",
				Family:      "gpt-oss",
				Reasoning:   true,
				ToolCall:    true,
				OpenWeights: true,
				ReleaseDate: "2025-08-05",
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.15,
					Output: 0.6,
				},
				Limit: &Limit{
					Context: 131072,
					Output:  65536,
				},
			},
			"moonshotai/kimi-k2-instruct": {
				ID:          "moonshotai/kimi-k2-instruct",
				Name:        "Kimi K2 Instruct",
				Family:      "kimi",
				ToolCall:    true,
				OpenWeights: true,
				Knowledge:   "2024-10",
				ReleaseDate: "2025-07-14",
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  1,
					Output: 3,
				},
				Limit: &Limit{
					Context: 131072,
					Output:  16384,
				},
				Status: "deprecated",
			},
		},
	},
	// mistral removed — no models from supported provider families
	{ // moonshotai — Moonshot AI
		ID:   "moonshotai",
		Name: "Moonshot AI",
		API:  "https://api.moonshot.ai/v1",
		Doc:  "https://platform.moonshot.ai/docs/api/chat",
		Models: map[string]Model{
			"kimi-k2.5": {
				ID:          "kimi-k2.5",
				Name:        "Kimi K2.5",
				Family:      "kimi",
				Reasoning:   true,
				ToolCall:    true,
				OpenWeights: true,
				Knowledge:   "2025-01",
				ReleaseDate: "2026-01",
				Modalities: &Modalities{
					Input:  []string{"text", "image", "video"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.6,
					Output: 3,
				},
				Limit: &Limit{
					Context: 262144,
					Output:  262144,
				},
			},
			"kimi-k2-thinking-turbo": {
				ID:          "kimi-k2-thinking-turbo",
				Name:        "Kimi K2 Thinking Turbo",
				Family:      "kimi-thinking",
				Reasoning:   true,
				ToolCall:    true,
				OpenWeights: true,
				Knowledge:   "2024-08",
				ReleaseDate: "2025-11-06",
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  1.15,
					Output: 8,
				},
				Limit: &Limit{
					Context: 262144,
					Output:  262144,
				},
			},
			"kimi-k2-thinking": {
				ID:          "kimi-k2-thinking",
				Name:        "Kimi K2 Thinking",
				Family:      "kimi-thinking",
				Reasoning:   true,
				ToolCall:    true,
				OpenWeights: true,
				Knowledge:   "2024-08",
				ReleaseDate: "2025-11-06",
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.6,
					Output: 2.5,
				},
				Limit: &Limit{
					Context: 262144,
					Output:  262144,
				},
			},
		},
	},
	{ // openai — OpenAI
		ID:   "openai",
		Name: "OpenAI",
		Doc:  "https://platform.openai.com/docs/models",
		Models: map[string]Model{
			"gpt-5.4-pro": {
				ID:          "gpt-5.4-pro",
				Name:        "GPT-5.4 Pro",
				Family:      "gpt-pro",
				Reasoning:   true,
				ToolCall:    true,
				Knowledge:   "2025-08-31",
				ReleaseDate: "2026-03-05",
				Modalities: &Modalities{
					Input:  []string{"text", "image"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  30,
					Output: 180,
				},
				Limit: &Limit{
					Context: 1050000,
					Output:  128000,
				},
			},
			"gpt-5.4": {
				ID:          "gpt-5.4",
				Name:        "GPT-5.4",
				Family:      "gpt",
				Reasoning:   true,
				ToolCall:    true,
				Knowledge:   "2025-08-31",
				ReleaseDate: "2026-03-05",
				Modalities: &Modalities{
					Input:  []string{"text", "image"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  2.5,
					Output: 15,
				},
				Limit: &Limit{
					Context: 1050000,
					Output:  128000,
				},
			},
			"gpt-5.3-codex-spark": {
				ID:          "gpt-5.3-codex-spark",
				Name:        "GPT-5.3 Codex Spark",
				Family:      "gpt-codex-spark",
				Reasoning:   true,
				ToolCall:    true,
				Knowledge:   "2025-08-31",
				ReleaseDate: "2026-02-05",
				Modalities: &Modalities{
					Input:  []string{"text", "image", "pdf"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  1.75,
					Output: 14,
				},
				Limit: &Limit{
					Context: 128000,
					Output:  32000,
				},
			},
			"gpt-5.3-codex": {
				ID:          "gpt-5.3-codex",
				Name:        "GPT-5.3 Codex",
				Family:      "gpt-codex",
				Reasoning:   true,
				ToolCall:    true,
				Knowledge:   "2025-08-31",
				ReleaseDate: "2026-02-05",
				Modalities: &Modalities{
					Input:  []string{"text", "image", "pdf"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  1.75,
					Output: 14,
				},
				Limit: &Limit{
					Context: 400000,
					Output:  128000,
				},
			},
			"gpt-5.2-pro": {
				ID:          "gpt-5.2-pro",
				Name:        "GPT-5.2 Pro",
				Family:      "gpt-pro",
				Reasoning:   true,
				ToolCall:    true,
				Knowledge:   "2025-08-31",
				ReleaseDate: "2025-12-11",
				Modalities: &Modalities{
					Input:  []string{"text", "image"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  21,
					Output: 168,
				},
				Limit: &Limit{
					Context: 400000,
					Output:  128000,
				},
			},
			"gpt-5.2-codex": {
				ID:          "gpt-5.2-codex",
				Name:        "GPT-5.2 Codex",
				Family:      "gpt-codex",
				Reasoning:   true,
				ToolCall:    true,
				Knowledge:   "2025-08-31",
				ReleaseDate: "2025-12-11",
				Modalities: &Modalities{
					Input:  []string{"text", "image", "pdf"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  1.75,
					Output: 14,
				},
				Limit: &Limit{
					Context: 400000,
					Output:  128000,
				},
			},
			"gpt-5.2-chat-latest": {
				ID:          "gpt-5.2-chat-latest",
				Name:        "GPT-5.2 Chat",
				Family:      "gpt-codex",
				Reasoning:   true,
				ToolCall:    true,
				Knowledge:   "2025-08-31",
				ReleaseDate: "2025-12-11",
				Modalities: &Modalities{
					Input:  []string{"text", "image"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  1.75,
					Output: 14,
				},
				Limit: &Limit{
					Context: 128000,
					Output:  16384,
				},
			},
			"gpt-5.2": {
				ID:          "gpt-5.2",
				Name:        "GPT-5.2",
				Family:      "gpt",
				Reasoning:   true,
				ToolCall:    true,
				Knowledge:   "2025-08-31",
				ReleaseDate: "2025-12-11",
				Modalities: &Modalities{
					Input:  []string{"text", "image"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  1.75,
					Output: 14,
				},
				Limit: &Limit{
					Context: 400000,
					Output:  128000,
				},
			},
			"gpt-5.1-codex-mini": {
				ID:          "gpt-5.1-codex-mini",
				Name:        "GPT-5.1 Codex mini",
				Family:      "gpt-codex",
				Reasoning:   true,
				ToolCall:    true,
				Knowledge:   "2024-09-30",
				ReleaseDate: "2025-11-13",
				Modalities: &Modalities{
					Input:  []string{"text", "image"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.25,
					Output: 2,
				},
				Limit: &Limit{
					Context: 400000,
					Output:  128000,
				},
			},
			"gpt-5.1-codex-max": {
				ID:          "gpt-5.1-codex-max",
				Name:        "GPT-5.1 Codex Max",
				Family:      "gpt-codex",
				Reasoning:   true,
				ToolCall:    true,
				Knowledge:   "2024-09-30",
				ReleaseDate: "2025-11-13",
				Modalities: &Modalities{
					Input:  []string{"text", "image"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  1.25,
					Output: 10,
				},
				Limit: &Limit{
					Context: 400000,
					Output:  128000,
				},
			},
			"gpt-5.1-codex": {
				ID:          "gpt-5.1-codex",
				Name:        "GPT-5.1 Codex",
				Family:      "gpt-codex",
				Reasoning:   true,
				ToolCall:    true,
				Knowledge:   "2024-09-30",
				ReleaseDate: "2025-11-13",
				Modalities: &Modalities{
					Input:  []string{"text", "image"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  1.25,
					Output: 10,
				},
				Limit: &Limit{
					Context: 400000,
					Output:  128000,
				},
			},
			"gpt-5.1-chat-latest": {
				ID:          "gpt-5.1-chat-latest",
				Name:        "GPT-5.1 Chat",
				Family:      "gpt-codex",
				Reasoning:   true,
				ToolCall:    true,
				Knowledge:   "2024-09-30",
				ReleaseDate: "2025-11-13",
				Modalities: &Modalities{
					Input:  []string{"text", "image"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  1.25,
					Output: 10,
				},
				Limit: &Limit{
					Context: 128000,
					Output:  16384,
				},
			},
			"gpt-5.1": {
				ID:          "gpt-5.1",
				Name:        "GPT-5.1",
				Family:      "gpt",
				Reasoning:   true,
				ToolCall:    true,
				Knowledge:   "2024-09-30",
				ReleaseDate: "2025-11-13",
				Modalities: &Modalities{
					Input:  []string{"text", "image"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  1.25,
					Output: 10,
				},
				Limit: &Limit{
					Context: 400000,
					Output:  128000,
				},
			},
		},
	},
	{ // openrouter — OpenRouter (open-weight models only; gemini-2.5-flash kept for routing but hidden from frontend)
		ID:   "openrouter",
		Name: "OpenRouter",
		API:  "https://openrouter.ai/api/v1",
		Doc:  "https://openrouter.ai/models",
		Models: map[string]Model{
			"z-ai/glm-5.1": {
				ID:          "z-ai/glm-5.1",
				Name:        "GLM-5.1",
				Family:      "glm",
				Speed:       "balanced",
				Reasoning:   true,
				ToolCall:    true,
				OpenWeights: true,
				ReleaseDate: "2026-04-16",
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  1,
					Output: 3.2,
				},
				Limit: &Limit{
					Context: 202752,
					Output:  131000,
				},
			},
			"z-ai/glm-5": {
				ID:          "z-ai/glm-5",
				Name:        "GLM-5",
				Family:      "glm",
				Speed:       "balanced",
				Reasoning:   true,
				ToolCall:    true,
				OpenWeights: true,
				ReleaseDate: "2026-02-12",
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  1,
					Output: 3.2,
				},
				Limit: &Limit{
					Context: 202752,
					Output:  131000,
				},
			},
			"minimax/minimax-m2.5": {
				ID:          "minimax/minimax-m2.5",
				Name:        "MiniMax M2.5",
				Family:      "minimax",
				Speed:       "fast",
				Reasoning:   true,
				ToolCall:    true,
				OpenWeights: true,
				ReleaseDate: "2026-02-12",
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.3,
					Output: 1.2,
				},
				Limit: &Limit{
					Context: 204800,
					Output:  131072,
				},
			},
			"minimax/minimax-m2.1": {
				ID:          "minimax/minimax-m2.1",
				Name:        "MiniMax M2.1",
				Family:      "minimax",
				Speed:       "fast",
				Reasoning:   true,
				ToolCall:    true,
				OpenWeights: true,
				ReleaseDate: "2025-12-23",
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.3,
					Output: 1.2,
				},
				Limit: &Limit{
					Context: 204800,
					Output:  131072,
				},
			},
			"moonshotai/kimi-k2.5": {
				ID:          "moonshotai/kimi-k2.5",
				Name:        "Kimi K2.5",
				Family:      "kimi",
				Speed:       "balanced",
				Reasoning:   true,
				ToolCall:    true,
				OpenWeights: true,
				Knowledge:   "2025-01",
				ReleaseDate: "2026-01-27",
				Modalities: &Modalities{
					Input:  []string{"text", "image", "video"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.6,
					Output: 3,
				},
				Limit: &Limit{
					Context: 262144,
					Output:  262144,
				},
			},
			"moonshotai/kimi-k2-thinking": {
				ID:          "moonshotai/kimi-k2-thinking",
				Name:        "Kimi K2 Thinking",
				Family:      "kimi-thinking",
				Speed:       "slow",
				Reasoning:   true,
				ToolCall:    true,
				OpenWeights: true,
				Knowledge:   "2024-08",
				ReleaseDate: "2025-11-06",
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.6,
					Output: 2.5,
				},
				Limit: &Limit{
					Context: 262144,
					Output:  262144,
				},
			},
			"z-ai/glm-4.7": {
				ID:          "z-ai/glm-4.7",
				Name:        "GLM-4.7",
				Family:      "glm",
				Speed:       "balanced",
				Reasoning:   true,
				ToolCall:    true,
				OpenWeights: true,
				Knowledge:   "2025-04",
				ReleaseDate: "2025-12-22",
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.6,
					Output: 2.2,
				},
				Limit: &Limit{
					Context: 204800,
					Output:  131072,
				},
			},
			"z-ai/glm-4.7-flash": {
				ID:          "z-ai/glm-4.7-flash",
				Name:        "GLM-4.7-Flash",
				Family:      "glm",
				Speed:       "fast",
				Reasoning:   true,
				ToolCall:    true,
				OpenWeights: true,
				ReleaseDate: "2026-01-19",
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.07,
					Output: 0.4,
				},
				Limit: &Limit{
					Context: 200000,
					Output:  65535,
				},
			},
			"google/gemini-2.5-flash": {
				ID:               "google/gemini-2.5-flash",
				Name:             "Gemini 2.5 Flash",
				Family:           "gemini-flash",
				Speed:            "fast",
				Reasoning:        true,
				ToolCall:         true,
				StructuredOutput: true,
				Hidden:           true,
				Knowledge:        "2025-01",
				ReleaseDate:      "2025-07-17",
				Modalities: &Modalities{
					Input:  []string{"text", "image", "audio", "video", "pdf"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.3,
					Output: 2.5,
				},
				Limit: &Limit{
					Context: 1048576,
					Output:  65536,
				},
			},
		},
	},
	// togetherai, xai removed — not in supported provider set
	{ // zai — Z.AI
		ID:   "zai",
		Name: "Z.AI",
		API:  "https://api.z.ai/api/paas/v4",
		Doc:  "https://docs.z.ai/guides/overview/pricing",
		Models: map[string]Model{
			"glm-5": {
				ID:          "glm-5",
				Name:        "GLM-5",
				Family:      "glm",
				Reasoning:   true,
				ToolCall:    true,
				OpenWeights: true,
				ReleaseDate: "2026-02-11",
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  1,
					Output: 3.2,
				},
				Limit: &Limit{
					Context: 204800,
					Output:  131072,
				},
			},
			"glm-4.7-flash": {
				ID:          "glm-4.7-flash",
				Name:        "GLM-4.7-Flash",
				Family:      "glm-flash",
				Reasoning:   true,
				ToolCall:    true,
				OpenWeights: true,
				Knowledge:   "2025-04",
				ReleaseDate: "2026-01-19",
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Limit: &Limit{
					Context: 200000,
					Output:  131072,
				},
			},
			"glm-4.7": {
				ID:          "glm-4.7",
				Name:        "GLM-4.7",
				Family:      "glm",
				Reasoning:   true,
				ToolCall:    true,
				OpenWeights: true,
				Knowledge:   "2025-04",
				ReleaseDate: "2025-12-22",
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.6,
					Output: 2.2,
				},
				Limit: &Limit{
					Context: 204800,
					Output:  131072,
				},
			},
			"glm-4.6v": {
				ID:          "glm-4.6v",
				Name:        "GLM-4.6V",
				Family:      "glm",
				Reasoning:   true,
				ToolCall:    true,
				OpenWeights: true,
				Knowledge:   "2025-04",
				ReleaseDate: "2025-12-08",
				Modalities: &Modalities{
					Input:  []string{"text", "image", "video"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.3,
					Output: 0.9,
				},
				Limit: &Limit{
					Context: 128000,
					Output:  32768,
				},
			},
		},
	},
	{ // zhipuai — Zhipu AI
		ID:   "zhipuai",
		Name: "Zhipu AI",
		API:  "https://open.bigmodel.cn/api/paas/v4",
		Doc:  "https://docs.z.ai/guides/overview/pricing",
		Models: map[string]Model{
			"glm-5": {
				ID:          "glm-5",
				Name:        "GLM-5",
				Family:      "glm",
				Reasoning:   true,
				ToolCall:    true,
				OpenWeights: true,
				ReleaseDate: "2026-02-11",
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  1,
					Output: 3.2,
				},
				Limit: &Limit{
					Context: 204800,
					Output:  131072,
				},
			},
			"glm-4.7-flash": {
				ID:          "glm-4.7-flash",
				Name:        "GLM-4.7-Flash",
				Family:      "glm-flash",
				Reasoning:   true,
				ToolCall:    true,
				OpenWeights: true,
				Knowledge:   "2025-04",
				ReleaseDate: "2026-01-19",
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Limit: &Limit{
					Context: 200000,
					Output:  131072,
				},
			},
			"glm-4.7": {
				ID:          "glm-4.7",
				Name:        "GLM-4.7",
				Family:      "glm",
				Reasoning:   true,
				ToolCall:    true,
				OpenWeights: true,
				Knowledge:   "2025-04",
				ReleaseDate: "2025-12-22",
				Modalities: &Modalities{
					Input:  []string{"text"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.6,
					Output: 2.2,
				},
				Limit: &Limit{
					Context: 204800,
					Output:  131072,
				},
			},
			"glm-4.6v": {
				ID:          "glm-4.6v",
				Name:        "GLM-4.6V",
				Family:      "glm",
				Reasoning:   true,
				ToolCall:    true,
				OpenWeights: true,
				Knowledge:   "2025-04",
				ReleaseDate: "2025-12-08",
				Modalities: &Modalities{
					Input:  []string{"text", "image", "video"},
					Output: []string{"text"},
				},
				Cost: &Cost{
					Input:  0.3,
					Output: 0.9,
				},
				Limit: &Limit{
					Context: 128000,
					Output:  32768,
				},
			},
		},
	},
}
