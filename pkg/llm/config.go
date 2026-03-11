package llm

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// ProviderType represents the LLM provider
type ProviderType string

const (
	ProviderOllama  ProviderType = "ollama"
	ProviderAzure   ProviderType = "azure"
	ProviderOpenAI  ProviderType = "openai"
)

// Config holds configuration for LLM providers
type Config struct {
	Provider    ProviderType
	Model       string
	Temperature float32
	MaxTokens   int
	Timeout     time.Duration

	// Ollama specific
	OllamaURL string

	// Azure specific
	AzureOpenAIEndpoint string
	AzureOpenAIKey      string
	AzureOpenAIVersion  string

	// OpenAI specific
	OpenAIAPIKey string
}

// LoadConfig loads configuration from environment and .env files
func LoadConfig() *Config {
	// Load .env files if they exist
	_ = godotenv.Load(".env.local")
	_ = godotenv.Load(".env")

	cfg := &Config{
		Provider:    ProviderOllama, // Default to Ollama
		Model:       getEnv("LLM_MODEL", "llama2"),
		Temperature: getFloat("LLM_TEMPERATURE", 0.7),
		MaxTokens:   getInt("LLM_MAX_TOKENS", 2000),
		Timeout:     time.Duration(getInt("LLM_TIMEOUT_SECONDS", 30)) * time.Second,
	}

	// Provider selection
	if provider := os.Getenv("LLM_PROVIDER"); provider != "" {
		cfg.Provider = ProviderType(provider)
	}

	// Ollama configuration
	cfg.OllamaURL = getEnv("OLLAMA_BASE_URL", "http://localhost:11434")

	// Azure configuration
	cfg.AzureOpenAIEndpoint = os.Getenv("AZURE_OPENAI_ENDPOINT")
	cfg.AzureOpenAIKey = os.Getenv("AZURE_OPENAI_KEY")
	cfg.AzureOpenAIVersion = getEnv("AZURE_OPENAI_API_VERSION", "2024-02-15-preview")
	if azureModel := os.Getenv("AZURE_OPENAI_DEPLOYMENT"); azureModel != "" {
		cfg.Model = azureModel
	}

	// OpenAI configuration
	cfg.OpenAIAPIKey = os.Getenv("OPENAI_API_KEY")
	if openaiModel := os.Getenv("OPENAI_MODEL"); openaiModel != "" {
		cfg.Model = openaiModel
	}

	return cfg
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	switch c.Provider {
	case ProviderOllama:
		if c.OllamaURL == "" {
			return fmt.Errorf("OLLAMA_BASE_URL not set")
		}
	case ProviderAzure:
		if c.AzureOpenAIEndpoint == "" {
			return fmt.Errorf("AZURE_OPENAI_ENDPOINT not set")
		}
		if c.AzureOpenAIKey == "" {
			return fmt.Errorf("AZURE_OPENAI_KEY not set")
		}
	case ProviderOpenAI:
		if c.OpenAIAPIKey == "" {
			return fmt.Errorf("OPENAI_API_KEY not set")
		}
	default:
		return fmt.Errorf("unknown provider: %s", c.Provider)
	}

	if c.Model == "" {
		return fmt.Errorf("model not set")
	}

	if c.Temperature < 0 || c.Temperature > 2 {
		return fmt.Errorf("temperature must be between 0 and 2")
	}

	if c.MaxTokens < 1 {
		return fmt.Errorf("max_tokens must be at least 1")
	}

	return nil
}

// Helper functions
func getEnv(key, defaultVal string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultVal
}

func getInt(key string, defaultVal int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultVal
}

func getFloat(key string, defaultVal float32) float32 {
	if value, exists := os.LookupEnv(key); exists {
		if floatVal, err := strconv.ParseFloat(value, 32); err == nil {
			return float32(floatVal)
		}
	}
	return defaultVal
}
