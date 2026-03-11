package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// OllamaProvider implements the Provider interface for Ollama
type OllamaProvider struct {
	config *Config
	client *http.Client
	model  *ModelInfo
}

// NewOllamaProvider creates a new Ollama provider
func NewOllamaProvider(cfg *Config) (Provider, error) {
	provider := &OllamaProvider{
		config: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
		model: &ModelInfo{
			Name:        cfg.Model,
			Provider:    ProviderOllama,
			ContextSize: 4096,
			MaxTokens:   cfg.MaxTokens,
			Capabilities: []string{
				"text-completion",
				"chat",
			},
		},
	}

	return provider, nil
}

// Complete generates a text completion using Ollama
func (op *OllamaProvider) Complete(ctx context.Context, prompt string, options *CompletionOptions) (string, error) {
	reqBody := map[string]interface{}{
		"model":  op.config.Model,
		"prompt": prompt,
		"stream": false,
	}

	if options != nil {
		if options.Temperature != nil {
			reqBody["temperature"] = *options.Temperature
		}
		if options.MaxTokens != nil {
			reqBody["num_predict"] = int(*options.MaxTokens)
		}
	} else {
		reqBody["temperature"] = op.config.Temperature
		reqBody["num_predict"] = op.config.MaxTokens
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/api/generate", op.config.OllamaURL),
		bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := op.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama error (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Response string `json:"response"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse ollama response: %w", err)
	}

	return result.Response, nil
}

// Chat performs a chat completion using Ollama
func (op *OllamaProvider) Chat(ctx context.Context, messages []*Message, options *CompletionOptions) (string, error) {
	ollamaMessages := make([]map[string]string, len(messages))
	for i, msg := range messages {
		ollamaMessages[i] = map[string]string{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}

	reqBody := map[string]interface{}{
		"model":    op.config.Model,
		"messages": ollamaMessages,
		"stream":   false,
	}

	if options != nil {
		if options.Temperature != nil {
			reqBody["temperature"] = *options.Temperature
		}
		if options.MaxTokens != nil {
			reqBody["num_predict"] = int(*options.MaxTokens)
		}
	} else {
		reqBody["temperature"] = op.config.Temperature
		reqBody["num_predict"] = op.config.MaxTokens
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/api/chat", op.config.OllamaURL),
		bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := op.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama chat request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama error (status %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse ollama response: %w", err)
	}

	return result.Message.Content, nil
}

// GetModel returns the model information
func (op *OllamaProvider) GetModel() *ModelInfo {
	return op.model
}

// IsHealthy checks if the Ollama service is available
func (op *OllamaProvider) IsHealthy(ctx context.Context) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, "HEAD",
		fmt.Sprintf("%s/api/tags", op.config.OllamaURL), nil)
	if err != nil {
		return false, err
	}

	resp, err := op.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("ollama not responding: %w", err)
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

// GetProvider returns the provider type
func (op *OllamaProvider) GetProvider() ProviderType {
	return ProviderOllama
}

// GetAvailableModels lists available models in Ollama
func (op *OllamaProvider) GetAvailableModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("%s/api/tags", op.config.OllamaURL), nil)
	if err != nil {
		return nil, err
	}

	resp, err := op.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get ollama models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama error: status %d", resp.StatusCode)
	}

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	models := make([]string, len(result.Models))
	for i, m := range result.Models {
		models[i] = m.Name
	}

	return models, nil
}

// PullModel pulls a model from Ollama registry
func (op *OllamaProvider) PullModel(ctx context.Context, modelName string) error {
	reqBody := map[string]string{
		"name": modelName,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/api/pull", op.config.OllamaURL),
		bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := op.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to pull model: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ollama error: %s", string(body))
	}

	return nil
}
