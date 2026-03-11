package ai

// ChainType represents the type of AI chain
type ChainType string

const (
	ChainNLQuery       ChainType = "nl_query"
	ChainAnalyze       ChainType = "analyze"
	ChainGenerateDocs  ChainType = "generate_docs"
	ChainCodeReview    ChainType = "code_review"
	ChainSummarize     ChainType = "summarize"
)

// Message represents a conversation message
type Message struct {
	Role    string // "system", "user", "assistant"
	Content string
}

// ConversationContext holds conversation state
type ConversationContext struct {
	Messages   []*Message
	EntityID   int64
	EntityName string
	FilePath   string
}
