package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// AzureOpenAIProvider implements the Provider interface for Azure OpenAI
type AzureOpenAIProvider struct {
	config *Config
	client *http.Client
	model  *ModelInfo
}

// NewAzureOpenAIProvider creates a new Azure OpenAI provider
func NewAzureOpenAIProvider(cfg *Config) (Provider, error) {
	if cfg.AzureOpenAIEndpoint == "" {
		return nil, fmt.Errorf("AZURE_OPENAI_ENDPOINT is required")
	}
	if cfg.AzureOpenAIKey == "" {
		return nil, fmt.Errorf("AZURE_OPENAI_KEY is required")
	}

	provider := &AzureOpenAIProvider{
		config: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
		model: &ModelInfo{
			Name:        cfg.Model,
			Provider:    ProviderAzure,
			ContextSize: 128000,
			MaxTokens:   int(cfg.MaxTokens),
			Capabilities: []string{
				"text-completion",
				"chat",
			},
		},
	}

	return provider, nil
}

// Complete generates a text completion using Azure OpenAI
func (ap *AzureOpenAIProvider) Complete(ctx context.Context, prompt string, options *CompletionOptions) (string, error) {
	temperature := ap.config.Temperature
	maxTokens := ap.config.MaxTokens
	var topP *float32
	var stopSeq []string

	if options != nil {
		if options.Temperature != nil {
			temperature = *options.Temperature
		}
		if options.MaxTokens != nil {
			maxTokens = int(*options.MaxTokens)
		}
		if options.TopP != nil {
			topP = options.TopP
		}
		if len(options.StopSequences) > 0 {
			stopSeq = options.StopSequences
		}
	}

	reqBody := map[string]interface{}{
		"model":       ap.config.Model,
		"prompt":      prompt,
		"temperature": temperature,
		"max_tokens":  maxTokens,
	}

	if topP != nil {
		reqBody["top_p"] = *topP
	}
	if len(stopSeq) > 0 {
		reqBody["stop"] = stopSeq
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	// Azure OpenAI uses a slightly different URL format
	url := fmt.Sprintf("%s/openai/deployments/%s/completions?api-version=%s",
		ap.config.AzureOpenAIEndpoint,
		ap.config.Model,
		ap.config.AzureOpenAIVersion)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", ap.config.AzureOpenAIKey)

	resp, err := ap.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("azure openai request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("azure openai error (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Choices []struct {
			Text string `json:"text"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse azure response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no completion choices returned from Azure OpenAI")
	}

	return result.Choices[0].Text, nil
}

// Chat performs a chat completion using Azure OpenAI
func (ap *AzureOpenAIProvider) Chat(ctx context.Context, messages []*Message, options *CompletionOptions) (string, error) {
	chatMessages := make([]map[string]string, len(messages))

	for i, msg := range messages {
		chatMessages[i] = map[string]string{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}

	temperature := ap.config.Temperature
	maxTokens := ap.config.MaxTokens
	var topP *float32
	var stopSeq []string

	if options != nil {
		if options.Temperature != nil {
			temperature = *options.Temperature
		}
		if options.MaxTokens != nil {
			maxTokens = int(*options.MaxTokens)
		}
		if options.TopP != nil {
			topP = options.TopP
		}
		if len(options.StopSequences) > 0 {
			stopSeq = options.StopSequences
		}
	}

	reqBody := map[string]interface{}{
		"model":       ap.config.Model,
		"messages":    chatMessages,
		"temperature": temperature,
		"max_tokens":  maxTokens,
	}

	if topP != nil {
		reqBody["top_p"] = *topP
	}
	if len(stopSeq) > 0 {
		reqBody["stop"] = stopSeq
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s",
		ap.config.AzureOpenAIEndpoint,
		ap.config.Model,
		ap.config.AzureOpenAIVersion)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", ap.config.AzureOpenAIKey)

	resp, err := ap.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("azure openai chat request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("azure openai error (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse azure response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no chat choices returned from Azure OpenAI")
	}

	return result.Choices[0].Message.Content, nil
}

// GetModel returns the model information
func (ap *AzureOpenAIProvider) GetModel() *ModelInfo {
	return ap.model
}

// IsHealthy checks if the Azure OpenAI service is available
func (ap *AzureOpenAIProvider) IsHealthy(ctx context.Context) (bool, error) {
	// Azure OpenAI doesn't have a simple health check endpoint
	// We'll verify by trying a simple completion request with a timeout
	maxTokens := int64(1)
	_, err := ap.Complete(ctx, "test", &CompletionOptions{
		MaxTokens: &maxTokens,
	})

	if err != nil {
		return false, fmt.Errorf("azure openai not responding: %w", err)
	}

	return true, nil
}

// GetProvider returns the provider type
func (ap *AzureOpenAIProvider) GetProvider() ProviderType {
	return ProviderAzure
}
