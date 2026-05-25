package registry

type HivyModel struct {
	ID     string
	Routes []ModelRoute
}

var supportedHivyModels = []HivyModel{
	{
		ID: "claude-opus-4.7",
		Routes: []ModelRoute{
			{ProviderID: "anthropic", ModelID: "claude-opus-4-7"},
			{ProviderID: "openrouter", ModelID: "anthropic/claude-opus-4.7"},
		},
	},
	{
		ID: "claude-opus-4.7-fast",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "anthropic/claude-opus-4.7-fast"},
		},
	},
	{
		ID: "claude-opus-4.6",
		Routes: []ModelRoute{
			{ProviderID: "anthropic", ModelID: "claude-opus-4-6"},
			{ProviderID: "openrouter", ModelID: "anthropic/claude-opus-4.6"},
		},
	},
	{
		ID: "claude-opus-4.5",
		Routes: []ModelRoute{
			{ProviderID: "anthropic", ModelID: "claude-opus-4-5"},
			{ProviderID: "openrouter", ModelID: "anthropic/claude-opus-4.5"},
		},
	},
	{
		ID: "claude-sonnet-4.6",
		Routes: []ModelRoute{
			{ProviderID: "anthropic", ModelID: "claude-sonnet-4-6"},
			{ProviderID: "openrouter", ModelID: "anthropic/claude-sonnet-4.6"},
		},
	},
	{
		ID: "claude-sonnet-4.5",
		Routes: []ModelRoute{
			{ProviderID: "anthropic", ModelID: "claude-sonnet-4-5"},
			{ProviderID: "openrouter", ModelID: "anthropic/claude-sonnet-4.5"},
		},
	},
	{
		ID: "claude-sonnet-4",
		Routes: []ModelRoute{
			{ProviderID: "anthropic", ModelID: "claude-sonnet-4-0"},
			{ProviderID: "openrouter", ModelID: "anthropic/claude-sonnet-4"},
		},
	},
	{
		ID: "gpt-5.5",
		Routes: []ModelRoute{
			{ProviderID: "openai", ModelID: "gpt-5.5"},
			{ProviderID: "openrouter", ModelID: "openai/gpt-5.5"},
		},
	},
	{
		ID: "gpt-5.5-pro",
		Routes: []ModelRoute{
			{ProviderID: "openai", ModelID: "gpt-5.5-pro"},
			{ProviderID: "openrouter", ModelID: "openai/gpt-5.5-pro"},
		},
	},
	{
		ID: "gpt-5.4",
		Routes: []ModelRoute{
			{ProviderID: "openai", ModelID: "gpt-5.4"},
			{ProviderID: "openrouter", ModelID: "openai/gpt-5.4"},
		},
	},
	{
		ID: "gpt-5.4-pro",
		Routes: []ModelRoute{
			{ProviderID: "openai", ModelID: "gpt-5.4-pro"},
			{ProviderID: "openrouter", ModelID: "openai/gpt-5.4-pro"},
		},
	},
	{
		ID: "gpt-5.4-mini",
		Routes: []ModelRoute{
			{ProviderID: "openai", ModelID: "gpt-5.4-mini"},
			{ProviderID: "openrouter", ModelID: "openai/gpt-5.4-mini"},
		},
	},
	{
		ID: "gpt-5.4-nano",
		Routes: []ModelRoute{
			{ProviderID: "openai", ModelID: "gpt-5.4-nano"},
			{ProviderID: "openrouter", ModelID: "openai/gpt-5.4-nano"},
		},
	},
	{
		ID: "gpt-4o-mini",
		Routes: []ModelRoute{
			{ProviderID: "openai", ModelID: "gpt-4o-mini"},
			{ProviderID: "openrouter", ModelID: "openai/gpt-4o-mini"},
		},
	},
	{
		ID: "gpt-5.3-codex",
		Routes: []ModelRoute{
			{ProviderID: "openai", ModelID: "gpt-5.3-codex"},
			{ProviderID: "openrouter", ModelID: "openai/gpt-5.3-codex"},
		},
	},
	{
		ID: "gpt-5.3-codex-spark",
		Routes: []ModelRoute{
			{ProviderID: "openai", ModelID: "gpt-5.3-codex-spark"},
		},
	},
	{
		ID: "gemini-3.5-flash",
		Routes: []ModelRoute{
			{ProviderID: "google", ModelID: "gemini-3.5-flash"},
			{ProviderID: "openrouter", ModelID: "google/gemini-3.5-flash"},
		},
	},
	{
		ID: "gemini-3.1-flash-lite",
		Routes: []ModelRoute{
			{ProviderID: "google", ModelID: "gemini-3.1-flash-lite"},
			{ProviderID: "openrouter", ModelID: "google/gemini-3.1-flash-lite"},
		},
	},
	{
		ID: "gemini-3.1-pro-preview",
		Routes: []ModelRoute{
			{ProviderID: "google", ModelID: "gemini-3.1-pro-preview"},
			{ProviderID: "openrouter", ModelID: "google/gemini-3.1-pro-preview"},
		},
	},
	{
		ID: "gemini-3-flash-preview",
		Routes: []ModelRoute{
			{ProviderID: "google", ModelID: "gemini-3-flash-preview"},
			{ProviderID: "openrouter", ModelID: "google/gemini-3-flash-preview"},
		},
	},
	{
		ID: "deepseek-v4-pro",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "deepseek/deepseek-v4-pro"},
		},
	},
	{
		ID: "deepseek-v4-flash",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "deepseek/deepseek-v4-flash"},
		},
	},
	{
		ID: "qwen3.7-max",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "qwen/qwen3.7-max"},
		},
	},
	{
		ID: "qwen3.6-max-preview",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "qwen/qwen3.6-max-preview"},
		},
	},
	{
		ID: "qwen3.6-flash",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "qwen/qwen3.6-flash"},
		},
	},
	{
		ID: "qwen3.6-35b-a3b",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "qwen/qwen3.6-35b-a3b"},
		},
	},
	{
		ID: "qwen3.6-27b",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "qwen/qwen3.6-27b"},
		},
	},
	{
		ID: "kimi-k2.6",
		Routes: []ModelRoute{
			{ProviderID: "moonshotai", ModelID: "kimi-k2.6"},
			{ProviderID: "openrouter", ModelID: "moonshotai/kimi-k2.6"},
		},
	},
	{
		ID: "mimo-v2.5-pro",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "xiaomi/mimo-v2.5-pro"},
		},
	},
	{
		ID: "mimo-v2.5",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "xiaomi/mimo-v2.5"},
		},
	},
	{
		ID: "minimax-m2.7",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "minimax/minimax-m2.7"},
		},
	},
	{
		ID: "glm-5.1",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "z-ai/glm-5.1"},
		},
	},
	{
		ID: "glm-5-turbo",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "z-ai/glm-5-turbo"},
		},
	},
	{
		ID: "glm-5",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "z-ai/glm-5"},
		},
	},
	{
		ID: "glm-4.7",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "z-ai/glm-4.7"},
		},
	},
	{
		ID: "glm-4.7-flash",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "z-ai/glm-4.7-flash"},
		},
	},
	{
		ID: "mistral-small-4",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "mistralai/mistral-small-2603"},
		},
	},
	{
		ID: "minimax-m2.5",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "minimax/minimax-m2.5"},
		},
	},
	{
		ID: "step-3.5-flash",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "stepfun/step-3.5-flash"},
		},
	},
	{
		ID: "kimi-k2.5",
		Routes: []ModelRoute{
			{ProviderID: "moonshotai", ModelID: "kimi-k2.5"},
			{ProviderID: "openrouter", ModelID: "moonshotai/kimi-k2.5"},
		},
	},
}

var hivyModelsByID = func() map[string]HivyModel {
	out := make(map[string]HivyModel, len(supportedHivyModels))
	for _, model := range supportedHivyModels {
		out[model.ID] = model
	}
	return out
}()
