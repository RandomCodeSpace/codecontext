package llm

import (
	"context"
	"fmt"
)

// Provider is the interface for LLM providers
type Provider interface {
	// Complete generates text completion
	Complete(ctx context.Context, prompt string, options *CompletionOptions) (string, error)

	// Chat performs a chat completion with message history
	Chat(ctx context.Context, messages []*Message, options *CompletionOptions) (string, error)

	// GetModel returns information about the configured model
	GetModel() *ModelInfo

	// IsHealthy checks if the provider is available and healthy
	IsHealthy(ctx context.Context) (bool, error)

	// GetProvider returns the provider type
	GetProvider() ProviderType
}

// CompletionOptions contains options for LLM completions
type CompletionOptions struct {
	Temperature   *float32
	MaxTokens     *int64
	TopP          *float32
	StopSequences []string
}

// Message represents a chat message
type Message struct {
	Role    string // "system", "user", "assistant"
	Content string
}

// ModelInfo contains information about an LLM model
type ModelInfo struct {
	Name          string
	Provider      ProviderType
	ContextSize   int
	MaxTokens     int
	CostPer1kTokens float64 // in USD
	Capabilities  []string
}

// NewProvider creates a new LLM provider based on config
func NewProvider(cfg *Config) (Provider, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	switch cfg.Provider {
	case ProviderOllama:
		return NewOllamaProvider(cfg)
	case ProviderAzure:
		return NewAzureOpenAIProvider(cfg)
	case ProviderOpenAI:
		return NewOpenAIProvider(cfg)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", cfg.Provider)
	}
}

// ProviderChain implements fallback chain for providers
type ProviderChain struct {
	providers []Provider
}

// NewProviderChain creates a new provider chain with fallbacks
func NewProviderChain(providers ...Provider) *ProviderChain {
	return &ProviderChain{providers: providers}
}

// Complete tries each provider in sequence until one succeeds
func (pc *ProviderChain) Complete(ctx context.Context, prompt string, options *CompletionOptions) (string, error) {
	var lastErr error

	for _, provider := range pc.providers {
		// Check health first
		healthy, err := provider.IsHealthy(ctx)
		if err != nil || !healthy {
			lastErr = err
			continue
		}

		result, err := provider.Complete(ctx, prompt, options)
		if err != nil {
			lastErr = err
			continue
		}

		return result, nil
	}

	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("no providers available")
}

// Chat tries each provider in sequence until one succeeds
func (pc *ProviderChain) Chat(ctx context.Context, messages []*Message, options *CompletionOptions) (string, error) {
	var lastErr error

	for _, provider := range pc.providers {
		// Check health first
		healthy, err := provider.IsHealthy(ctx)
		if err != nil || !healthy {
			lastErr = err
			continue
		}

		result, err := provider.Chat(ctx, messages, options)
		if err != nil {
			lastErr = err
			continue
		}

		return result, nil
	}

	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("no providers available")
}

// GetProvider returns the first available provider's type
func (pc *ProviderChain) GetProvider() ProviderType {
	if len(pc.providers) > 0 {
		return pc.providers[0].GetProvider()
	}
	return ProviderOllama
}

// GetModel returns the first available provider's model
func (pc *ProviderChain) GetModel() *ModelInfo {
	if len(pc.providers) > 0 {
		return pc.providers[0].GetModel()
	}
	return nil
}

// IsHealthy checks if at least one provider is healthy
func (pc *ProviderChain) IsHealthy(ctx context.Context) (bool, error) {
	for _, provider := range pc.providers {
		healthy, _ := provider.IsHealthy(ctx)
		if healthy {
			return true, nil
		}
	}
	return false, fmt.Errorf("no healthy providers available")
}
