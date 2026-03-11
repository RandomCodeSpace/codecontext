# AI-Powered Code Analysis Features

This document describes the AI-powered code analysis capabilities of codecontext, built on a multi-provider LLM abstraction layer.

## Architecture

The AI system is built in three layers:

### 1. LLM Provider Layer (`pkg/llm/`)

The provider layer abstracts LLM interactions across multiple services:

- **Provider Interface**: Defines common methods for all LLM providers
  - `Complete()`: Text completion
  - `Chat()`: Multi-turn conversation
  - `GetModel()`: Model information
  - `IsHealthy()`: Provider availability check
  - `GetProvider()`: Provider type identification

- **Supported Providers**:
  - **Ollama** (`ollama.go`): HTTP-based client for local Ollama instances
  - **Azure OpenAI** (`azure.go`): HTTP client for Azure OpenAI services
  - **OpenAI** (`openai.go`): OpenAI API client

- **Configuration** (`config.go`):
  - Loads from environment variables
  - Supports `.env.local` and `.env` files
  - Cascade: .env.local → .env → env vars → defaults
  - Configurable temperature, max_tokens, timeout

- **Provider Chain**:
  - Implements fallback logic across multiple providers
  - Tries each provider until one succeeds
  - Useful for high-availability setups

### 2. AI Chain Layer (`pkg/ai/`)

The AI chain layer provides high-level code analysis operations:

- **Chain Operations**:
  - `QueryNatural()`: Answer questions about code using graph context
  - `AnalyzeEntity()`: Detailed analysis of functions, classes, etc.
  - `GenerateDocs()`: Generate documentation with usage examples
  - `ReviewCode()`: Code review with suggestions
  - `Summarize()`: Summarize file purpose and structure
  - `Chat()`: Multi-turn conversation with context

- **Conversation Context**:
  - Maintains message history
  - Tracks entity and file context
  - Enables multi-turn interactions

### 3. CLI Integration (`main.go`)

Command-line interface for AI features:

```
codecontext ai query <question>      # Natural language questions
codecontext ai analyze <entity>      # Entity analysis
codecontext ai docs <entity>         # Generate docs
codecontext ai review <entity>       # Code review
codecontext ai summarize <file>      # File summary
codecontext ai chat                  # Interactive chat
```

## Configuration

### Environment Variables

```bash
# Provider selection
LLM_PROVIDER=ollama|azure|openai

# Common settings
LLM_MODEL=llama2|gpt-4|etc
LLM_TEMPERATURE=0.7              # 0.0-2.0, higher = more creative
LLM_MAX_TOKENS=2000              # Maximum response length
LLM_TIMEOUT_SECONDS=30           # Request timeout

# Ollama-specific
OLLAMA_BASE_URL=http://localhost:11434

# Azure OpenAI-specific
AZURE_OPENAI_ENDPOINT=https://resource.openai.azure.com/
AZURE_OPENAI_KEY=your-api-key
AZURE_OPENAI_DEPLOYMENT=deployment-name
AZURE_OPENAI_API_VERSION=2024-02-15-preview

# OpenAI-specific
OPENAI_API_KEY=sk-your-api-key
OPENAI_MODEL=gpt-4
```

### Configuration Files

Create `.env.local` (for local overrides) or `.env` (for project defaults):

```sh
LLM_PROVIDER=ollama
OLLAMA_BASE_URL=http://localhost:11434
LLM_MODEL=llama2
LLM_TEMPERATURE=0.7
LLM_MAX_TOKENS=2000
```

**Note**: `.env.local` is in `.gitignore` for credential safety.

## Usage Examples

### 1. Natural Language Queries

Ask questions about your codebase:

```bash
codecontext ai query "what are the main functions in this project?"
codecontext ai query "how does error handling work?"
codecontext ai query "what are the dependencies?"
```

### 2. Entity Analysis

Detailed analysis of specific code elements:

```bash
codecontext ai analyze myFunction
codecontext ai analyze DatabaseConnection
codecontext ai analyze parseJSON
```

### 3. Documentation Generation

Auto-generate documentation:

```bash
codecontext ai docs calculateSum
codecontext ai docs HttpClient
codecontext ai docs validateInput
```

### 4. Code Review

Get AI-powered code review suggestions:

```bash
codecontext ai review fetchUserData
codecontext ai review ConfigManager
codecontext ai review validateEmail
```

### 5. File Summarization

Understand file purpose and structure:

```bash
codecontext ai summarize main.go
codecontext ai summarize database.py
codecontext ai summarize utils/helpers.js
```

### 6. Interactive Chat

Multi-turn conversation about code:

```bash
codecontext ai chat

> What's the purpose of AuthManager?
Assistant: AuthManager is a class that handles...

> Can you show me how to use it?
Assistant: Here's how to use AuthManager...

> What are its dependencies?
Assistant: AuthManager depends on...

> analyze LoginService
Assistant: Here's the detailed analysis...

> exit
```

## How It Works

### Data Flow

```
User Input
    ↓
CLI Handler
    ↓
AI Chain
    ↓
Code Graph Query ←→ LLM Provider
    ↓
Response
```

### Example: Query Flow

1. User runs: `codecontext ai query "what does main do?"`
2. CLI loads LLM configuration
3. AI Chain gets graph statistics for context
4. LLM provider is called with:
   - System prompt (role description)
   - Graph context (file count, entity count, etc.)
   - User question
5. Response is returned to user

### Example: Entity Analysis Flow

1. User runs: `codecontext ai analyze myFunction`
2. CLI loads LLM configuration
3. AI Chain queries code graph:
   - Finds entity "myFunction"
   - Gets call graph (callers, callees)
   - Gets file location and documentation
4. LLM provider is called with:
   - System prompt (role: code analyst)
   - Entity details and relationships
5. Response with analysis is returned

## Provider-Specific Notes

### Ollama

- **Requirements**: Ollama service running locally
- **Setup**:
  ```bash
  # Install from https://ollama.ai
  # Download a model
  ollama pull llama2
  # Start the service
  ollama serve
  ```
- **Advantages**: Free, private, no API keys
- **Performance**: Depends on local hardware

### Azure OpenAI

- **Requirements**: Azure account with OpenAI service
- **Setup**:
  1. Deploy Azure OpenAI resource
  2. Get endpoint and API key
  3. Create deployment
  4. Set environment variables
- **Advantages**: Enterprise security, SOC2/ISO compliance
- **Cost**: Pay-per-use based on tokens

### OpenAI

- **Requirements**: OpenAI API key
- **Setup**:
  1. Sign up at openai.com
  2. Create API key
  3. Set OPENAI_API_KEY environment variable
- **Advantages**: Most capable models (GPT-4), well-documented API
- **Cost**: Pay-per-use based on tokens

## Error Handling

The system includes graceful error handling:

- **Provider Not Available**: Clear error message with setup instructions
- **Configuration Missing**: Lists required environment variables
- **API Errors**: Detailed error messages with retry suggestions
- **Fallback Chain**: Automatically tries next provider if one fails

## Performance Considerations

- **Caching**: Consider caching LLM responses for repeated queries
- **Token Limits**: Configure `LLM_MAX_TOKENS` based on provider limits
- **Timeouts**: Adjust `LLM_TIMEOUT_SECONDS` for slow connections
- **Graph Context**: Larger graphs mean more context for better analysis

## Future Enhancements

Potential improvements:

1. **Caching Layer**: Cache analysis results to reduce API calls
2. **Batch Analysis**: Analyze multiple entities at once
3. **Custom Prompts**: Allow user-defined system prompts
4. **Multi-turn History**: Persistent conversation history
5. **Streaming**: Stream responses for faster feedback
6. **Cost Tracking**: Monitor API usage and costs
7. **RAG Integration**: Integrate with vector databases
8. **Model Fine-tuning**: Fine-tune models on project-specific data

## Troubleshooting

### Provider not responding

```bash
# Check provider health
# For Ollama: curl http://localhost:11434/api/tags

# Check configuration
env | grep LLM_
env | grep OLLAMA_
env | grep AZURE_
env | grep OPENAI_
```

### Unexpected responses

- Check `LLM_TEMPERATURE` (higher = more creative, lower = more deterministic)
- Check `LLM_MAX_TOKENS` (may be too low for complex queries)
- Try with `LLM_PROVIDER=openai` for more capable models

### Configuration not loading

1. Ensure `.env.local` or `.env` exists
2. Check `.env` syntax (no spaces around `=`)
3. Run from project root directory
4. Verify file permissions

## Integration with Code Graph

The AI features leverage the code graph for context:

- **Graph Statistics**: File count, entity count, dependency count
- **Entity Queries**: Find specific functions, classes, etc.
- **Call Graphs**: Understand caller/callee relationships
- **Dependencies**: See how modules interact

This integration enables accurate, context-aware code analysis.
