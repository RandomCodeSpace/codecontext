package ai

import (
	"context"
	"fmt"

	"github.com/RandomCodeSpace/codecontext/pkg/indexer"
	"github.com/RandomCodeSpace/codecontext/pkg/llm"
)

// Chain handles AI operations on code
type Chain struct {
	indexer  *indexer.Indexer
	provider llm.Provider
}

// NewChain creates a new AI chain
func NewChain(idx *indexer.Indexer, provider llm.Provider) *Chain {
	return &Chain{
		indexer:  idx,
		provider: provider,
	}
}

// QueryNatural performs a natural language query on the code graph
func (c *Chain) QueryNatural(ctx context.Context, query string) (string, error) {
	// Get graph stats to include in context
	stats, err := c.indexer.GetStats()
	if err != nil {
		return "", fmt.Errorf("failed to get graph stats: %w", err)
	}

	// Build system prompt
	systemPrompt := fmt.Sprintf(`You are a code analysis AI assistant. You have access to a code graph database.
Current graph contains:
- %d files
- %d entities (functions, classes, etc.)
- %d dependencies

When answering questions about code, provide specific references to entities in the codebase.
Format references as [EntityName] when mentioning code elements.`,
		stats["file_count"],
		stats["entity_count"],
		stats["dependency_count"])

	messages := []*llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: query},
	}

	response, err := c.provider.Chat(ctx, messages, nil)
	if err != nil {
		return "", fmt.Errorf("failed to query: %w", err)
	}

	return response, nil
}

// AnalyzeEntity provides detailed analysis of a specific entity
func (c *Chain) AnalyzeEntity(ctx context.Context, entityName string) (string, error) {
	// Query the entity from the graph
	entities, err := c.indexer.QueryEntity(entityName)
	if err != nil {
		return "", fmt.Errorf("failed to query entity: %w", err)
	}

	if len(entities) == 0 {
		return "", fmt.Errorf("entity not found: %s", entityName)
	}

	entity := entities[0]

	// Get the call graph for this entity
	callGraph, err := c.indexer.QueryCallGraph(entity.ID)
	if err != nil {
		return "", fmt.Errorf("failed to get call graph: %w", err)
	}

	// Get file path
	filePath := ""
	if entity.File != nil {
		filePath = entity.File.Path
	}

	// Build context about the entity
	contextStr := fmt.Sprintf(`Analyze this code entity:
Name: %s
Type: %s
Kind: %s
Signature: %s
Location: %s (lines %d-%d)
Documentation: %s`,
		entity.Name,
		entity.Type,
		entity.Kind,
		entity.Signature,
		filePath,
		entity.StartLine,
		entity.EndLine,
		entity.Documentation)

	if callGraph != nil {
		if callers, ok := callGraph["callers"]; ok {
			contextStr += fmt.Sprintf("\n\nCalled by: %v", callers)
		}
		if callees, ok := callGraph["callees"]; ok {
			contextStr += fmt.Sprintf("\nCalls: %v", callees)
		}
	}

	messages := []*llm.Message{
		{Role: "system", Content: "You are a code analysis expert. Provide detailed analysis of the given code entity including its purpose, dependencies, and potential issues."},
		{Role: "user", Content: contextStr},
	}

	response, err := c.provider.Chat(ctx, messages, nil)
	if err != nil {
		return "", fmt.Errorf("failed to analyze entity: %w", err)
	}

	return response, nil
}

// GenerateDocs generates documentation for an entity
func (c *Chain) GenerateDocs(ctx context.Context, entityName string) (string, error) {
	// Query the entity
	entities, err := c.indexer.QueryEntity(entityName)
	if err != nil {
		return "", fmt.Errorf("failed to query entity: %w", err)
	}

	if len(entities) == 0 {
		return "", fmt.Errorf("entity not found: %s", entityName)
	}

	entity := entities[0]

	// Get the call graph
	callGraph, err := c.indexer.QueryCallGraph(entity.ID)
	if err != nil {
		return "", fmt.Errorf("failed to get call graph: %w", err)
	}

	// Get file path
	filePath := ""
	if entity.File != nil {
		filePath = entity.File.Path
	}

	// Build context
	contextStr := fmt.Sprintf(`Generate comprehensive documentation for this code entity:

Name: %s
Type: %s
Kind: %s
Signature: %s
Location: %s (lines %d-%d)
Current Documentation: %s`,
		entity.Name,
		entity.Type,
		entity.Kind,
		entity.Signature,
		filePath,
		entity.StartLine,
		entity.EndLine,
		entity.Documentation)

	if callGraph != nil {
		if callees, ok := callGraph["callees"]; ok {
			contextStr += fmt.Sprintf("\nDependencies: %v", callees)
		}
	}

	messages := []*llm.Message{
		{
			Role: "system",
			Content: `You are a technical documentation expert. Generate clear, concise documentation for code entities.
Include:
1. Purpose and functionality
2. Parameters/arguments (if applicable)
3. Return value (if applicable)
4. Example usage (if applicable)
5. Exceptions/errors (if applicable)
6. Related entities`,
		},
		{Role: "user", Content: contextStr},
	}

	response, err := c.provider.Chat(ctx, messages, nil)
	if err != nil {
		return "", fmt.Errorf("failed to generate docs: %w", err)
	}

	return response, nil
}

// ReviewCode provides code review and suggestions
func (c *Chain) ReviewCode(ctx context.Context, entityName string) (string, error) {
	// Query the entity
	entities, err := c.indexer.QueryEntity(entityName)
	if err != nil {
		return "", fmt.Errorf("failed to query entity: %w", err)
	}

	if len(entities) == 0 {
		return "", fmt.Errorf("entity not found: %s", entityName)
	}

	entity := entities[0]

	// Get call graph for complexity analysis
	callGraph, err := c.indexer.QueryCallGraph(entity.ID)
	if err != nil {
		return "", fmt.Errorf("failed to get call graph: %w", err)
	}

	// Get file path
	filePath := ""
	if entity.File != nil {
		filePath = entity.File.Path
	}

	contextStr := fmt.Sprintf(`Review this code for quality and best practices:

Name: %s
Type: %s
Kind: %s
Signature: %s
Location: %s (lines %d-%d)`,
		entity.Name,
		entity.Type,
		entity.Kind,
		entity.Signature,
		filePath,
		entity.StartLine,
		entity.EndLine)

	if callGraph != nil {
		if callers, ok := callGraph["callers"]; ok {
			contextStr += fmt.Sprintf("\nCalled by: %v", callers)
		}
		if callees, ok := callGraph["callees"]; ok {
			contextStr += fmt.Sprintf("\nCalls: %v", callees)
		}
	}

	messages := []*llm.Message{
		{
			Role: "system",
			Content: `You are an experienced code reviewer. Provide constructive feedback including:
1. Code quality issues
2. Performance considerations
3. Best practice recommendations
4. Potential bugs or edge cases
5. Suggested improvements`,
		},
		{Role: "user", Content: contextStr},
	}

	response, err := c.provider.Chat(ctx, messages, nil)
	if err != nil {
		return "", fmt.Errorf("failed to review code: %w", err)
	}

	return response, nil
}

// Summarize generates a summary of a file or entity
func (c *Chain) Summarize(ctx context.Context, filePath string) (string, error) {
	// Get dependency graph for the file
	deps, err := c.indexer.QueryDependencyGraph(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to query dependencies: %w", err)
	}

	contextStr := fmt.Sprintf("Summarize the purpose and structure of this file:\nPath: %s\n", filePath)

	if deps != nil {
		if dependencies, ok := deps["dependencies"].([]interface{}); ok {
			contextStr += fmt.Sprintf("External Dependencies: %v\n", dependencies)
		}
		if dependents, ok := deps["dependents"].([]interface{}); ok {
			contextStr += fmt.Sprintf("Used By: %v\n", dependents)
		}
	}

	messages := []*llm.Message{
		{
			Role: "system",
			Content: `You are a code documentation expert. Generate a concise summary of a code file including:
1. Overall purpose
2. Key components/entities
3. Main functionality
4. Important dependencies
5. Typical use cases`,
		},
		{Role: "user", Content: contextStr},
	}

	response, err := c.provider.Chat(ctx, messages, nil)
	if err != nil {
		return "", fmt.Errorf("failed to summarize: %w", err)
	}

	return response, nil
}

// Chat enables multi-turn conversation about code
func (c *Chain) Chat(ctx context.Context, conversation *ConversationContext, userMessage string) (string, error) {
	// Add user message to conversation
	conversation.Messages = append(conversation.Messages, &Message{
		Role:    "user",
		Content: userMessage,
	})

	// Convert to llm.Message format
	llmMessages := make([]*llm.Message, len(conversation.Messages))
	for i, msg := range conversation.Messages {
		llmMessages[i] = &llm.Message{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	// Get response from provider
	response, err := c.provider.Chat(ctx, llmMessages, nil)
	if err != nil {
		return "", fmt.Errorf("failed to chat: %w", err)
	}

	// Add assistant response to conversation
	conversation.Messages = append(conversation.Messages, &Message{
		Role:    "assistant",
		Content: response,
	})

	return response, nil
}
