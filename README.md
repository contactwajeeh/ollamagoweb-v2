# OllamaGoWeb Features

This is a improved / enhanced fork of https://github.com/ml2068/ollamagoweb

A comprehensive LLM chat client built with Go, featuring a modern web interface and support for multiple AI providers.

---

## 🎨 User Interface

### Modern Chat Experience
- **Clean, responsive design** with light and dark theme support
- **Real-time streaming responses** with typewriter animation effect
- **Markdown rendering** with syntax highlighting for code blocks
- **Mobile-friendly** sidebar that collapses on smaller screens

### Theme Support
- **Light mode** - Clean, bright interface for daytime use
- **Dark mode** - Eye-friendly dark interface for low-light environments
- **Persistent preference** - Theme choice is saved and synced across sessions

---

## 💬 Chat Features

### Conversation Management
- **Automatic chat saving** - All conversations are automatically saved to the database
- **Chat history sidebar** - Browse and search through previous conversations
- **Rename chats** - Click the edit icon to rename any conversation inline
- **Delete chats** - Remove conversations you no longer need
- **Search functionality** - Search through chat titles and message content
- **Web Search** - use `/search <query>` to enrich your prompts with real-time results from Brave Search

### Context & Memory
- **Full Conversation Memory** - The LLM remembers previous messages in the chat
- **Sliding Window Context** - Automatically manages context window to keep the conversation relevant without exceeding token limits
- **Token Budgeting** - Intelligent management of history within the 4096 (or configured) token limit

### Message Editing & Versioning
- **Edit user messages** - Click the pencil icon to modify sent messages inline
- **Version history** - Edited messages create new versions while preserving the original
- **Version navigation** - Browse between different versions of a conversation
- **Regenerate responses** - Request a new AI response for any message

### System Prompts
- **Per-chat system prompts** - Configure custom instructions for each conversation
- **Persistent prompts** - System prompts are saved with the chat
- **Visual indicator** - Badge shows when a system prompt is active

### Export
- **Export to HTML** - Save any conversation as a formatted HTML document
- **Preserves formatting** - Code blocks, markdown, and styling are maintained

---

## 🔌 Provider Support

### Multiple Provider Types
- **Ollama (Local)** - Connect to local Ollama installations
- **OpenAI-compatible APIs** - Support for various cloud providers:
  - Groq
  - DeepInfra
  - OpenRouter
  - Any OpenAI-compatible endpoint

### Provider Management
- **Add multiple providers** - Configure as many providers as needed
- **Switch between providers** - Easily activate different providers
- **Auto-detect models** - Fetch available models from provider APIs
- **Manual model entry** - Add models manually if auto-fetch isn't available
- **Default model selection** - Set a preferred model for each provider

### Security
- **Encrypted API keys** - All API keys are encrypted before storage
- **Secure key migration** - Existing keys are automatically encrypted on upgrade

---

## 🤖 Model Features

### Model Management
- **Model dropdown** - Quick model switching in the chat header
- **Provider display** - Shows the active provider name alongside the model
- **Per-provider models** - Each provider maintains its own model list
- **Default model** - Set which model to use by default for each provider

### Auto-discovery
- **Fetch from Ollama** - Automatically discover locally installed models
- **Fetch from APIs** - Query compatible APIs for available models
- **Add to provider** - Easily add discovered models to your configuration

---

## ⚙️ Settings

### Configuration Page
- **Dedicated settings page** - Centralized configuration interface
- **Theme selection** - Toggle between light and dark modes
- **Provider management** - Add, edit, and remove providers
- **Generation settings** - Configure temperature and max tokens

### Generation Parameters
- **Temperature control** - Adjust response creativity (0.0 - 2.0)
- **Max tokens** - Set the maximum response length
- **Persistent settings** - All preferences are saved to the database

---

## 📊 Analytics

### Response Metadata
- **Model display** - Shows which model generated each response
- **Token usage** - Displays tokens used for each response
- **Generation speed** - Shows tokens per second performance
- **Visual indicators** - Clean icons for metadata display

---

## 🗄️ Database

### SQLite Storage
- **Embedded database** - No external database server required
- **Automatic migrations** - Schema updates happen automatically
- **Foreign key support** - Data integrity is maintained

### Data Stored
- **Providers** - Provider configurations and credentials
- **Models** - Available models for each provider
- **Chats** - Conversation history with metadata
- **Messages** - Full message history with timestamps
- **Settings** - User preferences and configuration

---

## 🔧 Technical Features

### Backend (Go)
- **Chi router** - Fast and lightweight HTTP routing
- **Graceful shutdown** - Clean server termination
- **Request logging** - Built-in middleware for request logging
- **Error recovery** - Automatic panic recovery

### Frontend
- **Vanilla JavaScript** - No framework dependencies
- **CSS Variables** - Easy theming with CSS custom properties
- **Showdown.js** - Markdown to HTML conversion
- **Highlight.js** - Syntax highlighting for code blocks
- **Bootstrap** - UI components and utilities

### API Endpoints
- **RESTful design** - Standard HTTP methods (GET, POST, PUT, DELETE)
- **JSON responses** - Consistent API response format
- **Streaming support** - Real-time response streaming

---

## 🚀 Getting Started

### Prerequisites
- Go 1.21 or later
- Ollama (for local models) or an API key for cloud providers

### Quick Start
1. Clone the repository
2. Copy `.env.example` to `.env` and configure
3. Run `go run .`
4. Open `http://localhost:1102` in your browser

### Environment Variables
| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | Server port | `1102` |
| `llm` | Default model | `llama3.1:8b` |
| `baseUrl` | API base URL (for OpenAI-compatible) | - |
| `apiKey` | API key (for OpenAI-compatible) | - |
| `DB_PATH` | SQLite database path | `./ollamagoweb.db` |
| `OLLAMA_HOST` | Ollama server URL | `http://localhost:11434` |

---

## 📁 Project Structure

```
ollamagoweb/
├── main.go           # Application entry point and routes
├── handlers.go       # HTTP request handlers
├── provider.go       # LLM provider implementations
├── search.go         # Brave Search integration
├── database.go       # Database initialization and migrations
├── crypto.go         # API key encryption utilities
├── search.go         # Brave API Search interface utilities
├── static/
│   ├── index.html    # Main chat interface
│   ├── settings.html # Settings page
│   ├── css/
│   │   └── styles.css
│   └── js/
│       ├── app.js    # Main application logic
│       └── settings.js
└── .env              # Configuration file
```

---

## 🎯 Key Highlights

| Feature | Description |
|---------|-------------|
| **Multi-provider** | Switch between Ollama, Groq, DeepInfra, and more |
| **Auto-save** | Never lose a conversation |
| **Message versioning** | Edit and track message history |
| **Encrypted secrets** | API keys are securely stored |
| **Responsive UI** | Works on desktop and mobile |
| **Theme support** | Light and dark modes |
| **No dependencies** | Single binary deployment |


