package registry

type HivyModel struct {
	ID     string
	Routes []ModelRoute
}

var supportedHivyModels = []HivyModel{
	{
		ID: "claude-sonnet-4.6",
		Routes: []ModelRoute{
			{ProviderID: "anthropic", ModelID: "claude-sonnet-4-6"},
			{ProviderID: "openrouter", ModelID: "anthropic/claude-sonnet-4.6"},
		},
	},
	{
		ID: "gpt-5.4",
		Routes: []ModelRoute{
			{ProviderID: "openai", ModelID: "gpt-5.4"},
		},
	},
	{
		ID: "gpt-5.4-pro",
		Routes: []ModelRoute{
			{ProviderID: "openai", ModelID: "gpt-5.4-pro"},
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
		ID: "deepseek-v4-flash",
		Routes: []ModelRoute{
			{ProviderID: "crof", ModelID: "deepseek-v4-flash"},
			{ProviderID: "openrouter", ModelID: "deepseek/deepseek-v4-flash"},
		},
	},
	{
		ID: "deepseek-v4-pro",
		Routes: []ModelRoute{
			{ProviderID: "crof", ModelID: "deepseek-v4-pro"},
			{ProviderID: "openrouter", ModelID: "deepseek/deepseek-v4-pro"},
		},
	},
	{
		ID: "kimi-k2.5",
		Routes: []ModelRoute{
			{ProviderID: "crof", ModelID: "kimi-k2.5"},
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
