package sandbox

import (
	"strings"

)

func defaultMaxTokens(providerID, modelName string) int32 {
	switch {
	case strings.Contains(modelName, "gpt-5-pro"):
		return 217600
	case strings.Contains(modelName, "gpt-5") && !strings.Contains(modelName, "chat"):
		return 102400
	case strings.Contains(modelName, "o3") || strings.Contains(modelName, "o4") || strings.Contains(modelName, "o1"):
		return 80000
	case strings.Contains(modelName, "kimi") && strings.Contains(modelName, "instruct"):
		return 13107
	case strings.Contains(modelName, "kimi"):
		return 209715
	case strings.Contains(modelName, "minimax") || strings.Contains(modelName, "MiniMax"):
		return 104857
	case strings.Contains(modelName, "glm-5") || strings.Contains(modelName, "glm-4.7") || strings.Contains(modelName, "glm-4.6"):
		return 104857
	case strings.Contains(modelName, "glm-4.5"):
		return 78643
	case strings.Contains(modelName, "glm"):
		return 26214
	}

	switch providerID {
	case "anthropic":
		return 51200
	case "openai":
		return 102400
	case "google":
		return 52428
	case "moonshotai":
		return 209715
	case "zai", "zhipuai":
		return 104857
	case "minimax":
		return 104857
	default:
		return 13107
	}
}

func defaultTemperature(providerID, modelName string) float64 {
	if strings.Contains(modelName, "kimi") {
		return 1.0
	}
	if strings.Contains(modelName, "deepseek-r1") || strings.Contains(modelName, "deepseek-reasoner") {
		return 0.6
	}
	if strings.Contains(modelName, "o1") || strings.Contains(modelName, "o3") || strings.Contains(modelName, "o4") {
		return 1.0
	}

	switch providerID {
	case "anthropic":
		return 1.0
	case "google":
		return 1.0
	case "openai":
		return 1.0
	case "deepseek":
		return 1.0
	case "cohere":
		return 0.3
	case "xai":
		return 0.7
	case "mistral":
		return 0.7
	default:
		return 0.7
	}
}
