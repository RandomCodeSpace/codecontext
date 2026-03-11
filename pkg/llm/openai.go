package llm

import (
	"context"
	"fmt"

	"github.com/sashabaranov/go-openai"
)

// OpenAIProvider implements the Provider interface for OpenAI
type OpenAIProvider struct {
	config *Config
	client *openai.Client
	model  *ModelInfo
}

// NewOpenAIProvider creates a new OpenAI provider
func NewOpenAIProvider(cfg *Config) (Provider, error) {
	if cfg.OpenAIAPIKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY is required for OpenAI provider")
	}

	client := openai.NewClient(cfg.OpenAIAPIKey)

	provider := &OpenAIProvider{
		config: cfg,
		client: client,
		model: &ModelInfo{
			Name:        cfg.Model,
			Provider:    ProviderOpenAI,
			ContextSize: 128000,
			MaxTokens:   cfg.MaxTokens,
			Capabilities: []string{
				"text-completion",
				"chat",
				"embeddings",
			},
		},
	}

	return provider, nil
}

// Complete generates a text completion using OpenAI
func (op *OpenAIProvider) Complete(ctx context.Context, prompt string, options *CompletionOptions) (string, error) {
	opts := openai.CompletionRequest{
		Model:       op.config.Model,
		Prompt:      prompt,
		Temperature: float32(op.config.Temperature),
		MaxTokens:   int(op.config.MaxTokens),
	}

	if options != nil {
		if options.Temperature != nil {
			opts.Temperature = *options.Temperature
		}
		if options.MaxTokens != nil {
			opts.MaxTokens = int(*options.MaxTokens)
		}
		if options.TopP != nil {
			opts.TopP = *options.TopP
		}
		if len(options.StopSequences) > 0 {
			opts.Stop = options.StopSequences
		}
	}

	resp, err := op.client.CreateCompletion(ctx, opts)
	if err != nil {
		return "", fmt.Errorf("openai completion failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no completion choices returned from OpenAI")
	}

	return resp.Choices[0].Text, nil
}

// Chat performs a chat completion using OpenAI
func (op *OpenAIProvider) Chat(ctx context.Context, messages []*Message, options *CompletionOptions) (string, error) {
	chatMessages := make([]openai.ChatCompletionMessage, len(messages))

	for i, msg := range messages {
		chatMessages[i] = openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	opts := openai.ChatCompletionRequest{
		Model:       op.config.Model,
		Messages:    chatMessages,
		Temperature: float32(op.config.Temperature),
		MaxTokens:   int(op.config.MaxTokens),
	}

	if options != nil {
		if options.Temperature != nil {
			opts.Temperature = *options.Temperature
		}
		if options.MaxTokens != nil {
			opts.MaxTokens = int(*options.MaxTokens)
		}
		if options.TopP != nil {
			opts.TopP = *options.TopP
		}
		if len(options.StopSequences) > 0 {
			opts.Stop = options.StopSequences
		}
	}

	resp, err := op.client.CreateChatCompletion(ctx, opts)
	if err != nil {
		return "", fmt.Errorf("openai chat failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no chat choices returned from OpenAI")
	}

	return resp.Choices[0].Message.Content, nil
}

// GetModel returns the model information
func (op *OpenAIProvider) GetModel() *ModelInfo {
	return op.model
}

// IsHealthy checks if the OpenAI API is accessible
func (op *OpenAIProvider) IsHealthy(ctx context.Context) (bool, error) {
	// OpenAI doesn't have a dedicated health endpoint
	// We verify connectivity by attempting a minimal completion request
	maxTokens := int64(1)
	_, err := op.Complete(ctx, "ping", &CompletionOptions{
		MaxTokens: &maxTokens,
	})

	if err != nil {
		return false, fmt.Errorf("openai not responding: %w", err)
	}

	return true, nil
}

// GetProvider returns the provider type
func (op *OpenAIProvider) GetProvider() ProviderType {
	return ProviderOpenAI
}
