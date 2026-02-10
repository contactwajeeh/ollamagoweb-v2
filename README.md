# OllamaGoWeb

A comprehensive LLM chat client built with Go, featuring a modern web interface and support for multiple AI providers.

---

## 🎨 User Interface

### Modern Chat Experience
- **Clean, responsive design** with light and dark theme support
- **Real-time streaming responses** with typewriter animation effect
- **Markdown rendering** with syntax highlighting for code blocks
- **Mobile-friendly** sidebar that collapses on smaller screens
- **Loading skeletons** - Animated placeholder UI while loading

### Accessibility
- **ARIA labels** - Full screen reader support
- **Skip link** - Quick navigation to main content
- **Focus indicators** - Visible focus states for keyboard navigation
- **Screen reader announcements** - Live region for status updates

### Theme Support
- **Light mode** - Clean, bright interface for daytime use
- **Dark mode** - Eye-friendly dark interface for low-light environments
- **Persistent preference** - Theme choice is saved and synced across sessions

---

## 💬 Chat Features

### Conversation Management
- **Automatic chat saving** - All conversations are automatically saved to database
- **Chat history sidebar** - Browse and search through previous conversations
- **Rename chats** - Click to edit icon to rename any conversation inline
- **Delete chats** - Remove conversations with 5-second undo option
- **Pinned chats** - Pin important conversations to top of list
- **Floating New Chat** - "New Chat" button (FAB) available on mobile

### Message Features
- **Copy-to-clipboard** - One-click copy for code blocks and messages
- **Edit user messages** - Click to pencil icon to modify sent messages inline
- **Regenerate responses** - Request a new AI response for any message
- **Undo deletion** - 5-second window to undo chat deletion with toast notification

### Export
- **Export to HTML** - Save conversations as formatted HTML documents
- **Export to JSON** - Export chat data in JSON format
- **Preserves formatting** - Code blocks, markdown, and styling are maintained

### Keyboard Shortcuts
| Shortcut | Action |
|----------|--------|
| `Ctrl+N` | New chat |
| `Ctrl+S` | Export chat |
| `Arrow Keys` | Navigate chat history |
| `Delete` | Delete selected chat |
| `Shift+?` | Show keyboard shortcuts |

---

## 🤖 Telegram Bot Integration

### Bot Setup
- **Create bot via @BotFather** - Get bot token from Telegram's official bot
- **Configure with environment variables** - Set `TELEGRAM_BOT_TOKEN` in `.env`
- **User allowlisting** - Restrict bot to specific Telegram user IDs for security

### Available Commands
| Command | Description |
|---------|-------------|
| `/start` | Start a new session |
| `/help` | Show all available commands |
| `/memories` | View your saved memories |
| `/clear` | Clear current conversation history |
| `/settings` | Show your current settings |
| `/link_session <id> <token>` | Link Telegram to web session |
| `/unlink_session` | Unlink from web session |
| `/session_info` | Show session status (linked/unlinked) |

### Session Linking
- **Secure token-based linking** - 256-bit entropy cryptographically secure tokens
- **15-minute token expiration** - Tokens expire automatically for security
- **One-time use tokens** - Each token can only be used once
- **Shared memories and context** - Unified experience across web and Telegram

### Linking to Web Session
1. Open your web app at `http://localhost:1102`
2. Open browser console (F12) and run:
   ```javascript
   fetch('/api/session/link-token').then(r=>r.json()).then(console.log)
   ```
3. Copy `session_id` and `link_token` from response
4. In Telegram, send: `/link_session <session_id> <link_token>`

### Telegram Features
- **Typing indicators** - Shows "typing..." while bot generates responses
- **Full LLM response support** - Complete AI responses in Telegram
- **Memory sharing** - Memories sync between web and Telegram when linked
- **Chat summary support** - Uses rolling summaries for efficient context
- **System prompt support** - Respects chat-specific system instructions
- **Automatic conversation saving** - All Telegram chats saved to database

### Configuration
See `.env.example` for Telegram bot configuration variables:
- `TELEGRAM_BOT_TOKEN` - Bot token from @BotFather
- `TELEGRAM_ALLOWED_USERS` - Comma-separated list of allowed Telegram user IDs

---

## 🧠 Memory System

### Automatic Extraction
- **LLM-based extraction** - Uses AI to extract important information from messages
- **Pattern-based extraction** - Recognizes common phrases (e.g., "my name is...")
- **Background processing** - Memory extraction runs asynchronously

### Memory Categories
| Category | Description | Examples |
|----------|-------------|-----------|
| `reminder` | Appointments, meetings, tasks, deadlines | "Meeting with Ram at 5 PM EST" |
| `fact` | Personal information, important details | "My name is John" |
| `preference` | User preferences, communication style | "Prefer concise responses" |
| `entity` | People, organizations, locations | "Works at Acme Corp" |

### Confidence Scoring
- **70-100 confidence range** - Higher scores for more reliable extractions
- **Automatic categorization** - Memories are auto-categorized by AI
- **Quality tracking** - Confidence scores help prioritize important information

### Memory Management
- **Add memories** - Automatically extracted from conversations
- **Update memories** - Modify existing memories with new information
- **Delete memories** - Remove outdated or incorrect memories
- **Search memories** - Find memories by category or keyword
- **Automatic injection** - Memories are automatically included in AI context

---

## 🔄 Rolling Summary System

### Automatic Summarization
- **Triggers after 10+ unsummarized messages** - Automatically summarizes old messages
- **Batch processing** - Processes oldest 10 messages per batch
- **Background processing** - Summarization runs without blocking user interaction

### Context Window Management
- **Combines summary + recent messages** - Maintains conversation continuity
- **Token-efficient** - Reduces token usage for long conversations
- **Intelligent history** - Smart context window management

### Summary Evolution
- **Incremental updates** - Summaries are updated with each batch
- **Preserves key information** - Maintains important facts, decisions, context
- **Narrative format** - Summaries are readable conversation summaries

---

## 📜 System Prompts

### Chat-Specific Prompts
- **Custom system instructions** - Set unique AI behavior per chat
- **Override default behavior** - Customize how AI responds in specific conversations
- **Persistent across sessions** - System prompts saved with chat

### Prompt Management
| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/chats/{id}/system-prompt` | Get system prompt for chat |
| `PUT` | `/api/chats/{id}/system-prompt` | Update system prompt for chat |

### Usage
System prompts are automatically applied to all LLM generations within that chat, allowing for:
- Different personas for different conversation types
- Specialized instructions for technical vs casual chats
- Custom behavior for specific use cases

---

## 🔌 Provider Support

### Multiple Provider Types
- **Ollama (Local)** - Connect to local Ollama installations
- **OpenAI-compatible APIs** - Support for:
  - Groq
  - DeepInfra
  - OpenRouter
  - Any OpenAI-compatible endpoint

### Provider Management
- **Add multiple providers** - Configure as many providers as needed
- **Switch between providers** - Easily activate different providers
- **Auto-detect models** - Fetch available models from provider APIs
- **Manual model entry** - Add models manually if needed
- **Default model selection** - Set a preferred model for each provider

### Security
- **Encrypted API keys** - All API keys encrypted with AES-GCM
- **Encryption enforcement** - Application fails if `ENCRYPTION_KEY` not set
- **Secure key migration** - Existing keys are automatically encrypted

---

## 🔍 Brave Search Integration

### Search API Configuration
- **API key setup** - Set `brave_api_key` in settings
- **Encrypted storage** - API keys stored securely in database
- **Optional feature** - Works fine without search integration

### Search Features
- **`/search <query>` command** - Initiate web search from chat
- **Automatic enrichment** - Search results automatically added to context
- **Real-time results** - Live search results from Brave Search API

### Result Integration
- **Merges search results** - Combines web search with user query
- **Enhanced AI responses** - AI provides answers with current information
- **Seamless experience** - Search integration is transparent to user

---

## 🔌 MCP Integration

### Model Context Protocol
- **Connect to MCP servers** - Integrate with external tools and services
- **Tool discovery** - Automatically discover available tools from servers
- **HTTP and stdio support** - Connect to both HTTP and stdio-based servers

### Server Management
| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/mcp/servers` | List all configured MCP servers |
| `POST` | `/api/mcp/servers` | Create new MCP server |
| `PUT` | `/api/mcp/servers/{id}` | Update MCP server configuration |
| `DELETE` | `/api/mcp/servers/{id}` | Delete MCP server |
| `GET` | `/api/mcp/servers/tools` | Fetch available tools from servers |

### Tool Integration
- **Automatic tool discovery** - Fetches tools from connected servers
- **Tool-based AI capabilities** - AI can use external tools for enhanced responses
- **Server management** - Enable/disable servers as needed

### Configuration
- **Add MCP servers** via web interface
- **Configure endpoints and commands**
- **Manage server connections**
- **View available tools**

---

## 🔐 Authentication

### Session-Based Authentication
- **Optional authentication** - Enable via environment variables
- **Secure session cookies** - HttpOnly, Secure, SameSite
- **AES-GCM encryption** - Secure credential storage
- **Session expiration** - 24-hour session TTL with auto-cleanup

### Endpoints
| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/auth/login` | Authenticate user |
| `POST` | `/api/auth/logout` | End session |
| `GET` | `/api/auth/session` | Check session status |
| `GET` | `/admin` | Admin login page |

### Protected Routes
When authentication is enabled, these endpoints require a valid session:
- All `/api/chats/*` endpoints
- All `/api/messages/*` endpoints

### Configuration
See `.env.example` for authentication configuration variables:
- `AUTH_USER` - Admin username
- `AUTH_PASSWORD` - Admin password

Without `AUTH_USER` and `AUTH_PASSWORD`, the application runs in public mode (no authentication required).

---

## 📊 Analytics & Metrics

### Response Metadata
- **Model display** - Shows which model generated each response
- **Token usage** - Displays tokens used for each response
- **Generation speed** - Shows tokens per second performance

### Application Metrics
- **Endpoint: `GET /api/metrics`**
  - Chat count
  - Message count
  - Provider count
  - Model count
  - Uptime
  - Version

---

## 🔒 Security Features

### Encryption
API keys and sensitive configuration are encrypted using AES-GCM.

**Required for production:**
```bash
# Generate a secure key (Linux/Mac)
openssl rand -hex 32

# Set environment variable
export ENCRYPTION_KEY=your-generated-key-here
```

The application will fail to start if `ENCRYPTION_KEY` is not set in production.

### Rate Limiting
- 10 requests per second per IP address
- Burst capacity of 50 requests
- Configurable via `middleware.go`

### CSRF Protection
- State-changing API requests require a valid CSRF token
- Obtain token from: `GET /api/csrf`

### SQL Injection Prevention
- All user inputs are sanitized before database queries
- `sanitizeSearchQuery()` function handles search input

### Content Security Policy
- Strict CSP headers configured
- Self-referencing directives for scripts and styles

---

## 🧪 Testing

### Unit Tests
```bash
go test -v .
```

**Test Coverage:**
- `TestSanitizeSearchQuery` - Input sanitization validation
- `TestWriteError` - Error response formatting
- `TestWriteJSON` - JSON response helper
- `TestRateLimitMiddleware` - Rate limiting logic
- `TestGenerateCSRFToken` - CSRF token generation

### End-to-End Tests
```bash
npm install
npm run test:e2e
```

**E2E Tests (Playwright):**
- Homepage rendering
- Settings page functionality
- Keyboard shortcuts validation

---

## ⚡ Performance

### Backend Optimizations
- **Connection pooling** - 25 max connections, 5 idle, 5-minute lifetime
- **Message pagination** - Load 50 messages at a time for large chats
- **Lazy loading** - Chat history loaded on demand
- **Debounced search** - 300ms debounce for chat search

### Frontend Optimizations
- **Error boundaries** - Graceful error handling with toast notifications

---

## 📡 API Reference

### Core Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/chats` | List all chats |
| `GET` | `/api/chats/{id}` | Get specific chat |
| `POST` | `/api/chats` | Create new chat |
| `DELETE` | `/api/chats/{id}` | Delete chat |
| `PUT` | `/api/chats/{id}/rename` | Rename chat |
| `PUT` | `/api/chats/{id}/pin` | Toggle pin |
| `POST` | `/api/chats/{id}/messages` | Add message |
| `GET` | `/api/chats/search` | Search chats |

### System Prompt Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/chats/{id}/system-prompt` | Get system prompt |
| `PUT` | `/api/chats/{id}/system-prompt` | Update system prompt |

### Message Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `PUT` | `/api/messages/{id}` | Update message |
| `DELETE` | `/api/messages/{id}` | Delete message |

### Provider Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/providers` | List providers |
| `POST` | `/api/providers` | Create provider |
| `PUT` | `/api/providers/{id}` | Update provider |
| `DELETE` | `/api/providers/{id}` | Delete provider |
| `POST` | `/api/providers/{id}/activate` | Activate provider |
| `POST` | `/api/providers/{id}/fetch-models` | Fetch models |

### Model Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/models/{providerId}` | Get models |
| `POST` | `/api/models` | Add model |
| `DELETE` | `/api/models/{id}` | Delete model |
| `POST` | `/api/models/{id}/set-default` | Set default |

### Memory Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/memories` | Get memories for current session |
| `POST` | `/api/memories` | Set a memory |
| `DELETE` | `/api/memories` | Delete a memory |
| `GET` | `/api/memories/search` | Search memories |
| `POST` | `/api/memories/extract` | Test memory extraction |

### MCP Server Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/mcp/servers` | List MCP servers |
| `POST` | `/api/mcp/servers` | Create MCP server |
| `PUT` | `/api/mcp/servers/{id}` | Update MCP server |
| `DELETE` | `/api/mcp/servers/{id}` | Delete MCP server |
| `GET` | `/api/mcp/servers/tools` | Fetch server tools |

### Session Linking Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/session/link-token` | Generate secure link token |

### Authentication Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/auth/login` | User login |
| `POST` | `/api/auth/logout` | User logout |
| `GET` | `/api/auth/session` | Session status |
| `GET` | `/admin` | Admin login page |

### Utility Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/csrf` | Get CSRF token |
| `GET` | `/api/metrics` | Get app metrics |
| `GET` | `/api/settings/{key}` | Get setting |
| `PUT` | `/api/settings/{key}` | Update setting |
| `GET` | `/api/active-provider` | Get active provider |

---

## 🚀 Getting Started

### Prerequisites
- Go 1.21 or later
- Node.js (for E2E tests)
- Ollama (for local models) or API key for cloud providers

### Quick Start

```bash
# Clone repository
git clone <repo-url>
cd ollamagoweb

# Copy environment template
cp .env.example .env

# Edit .env with your configuration
nano .env
```

**Required Configuration:**
```bash
# Set server port (default: 1102)
PORT=1102

# Set encryption key (required for production)
# Generate with: openssl rand -hex 32
ENCRYPTION_KEY=your-encryption-key-here

# Configure LLM provider
llm=ollama  # or your preferred model
```

**Optional Configuration:**
```bash
# Enable authentication
AUTH_USER=admin
AUTH_PASSWORD=your-secure-password

# Configure Telegram bot
TELEGRAM_BOT_TOKEN=your-bot-token-from-botfather
TELEGRAM_ALLOWED_USERS=123456789,987654321

# Configure Brave Search
brave_api_key=your-brave-api-key
```

```bash
# Run application
go run .
```

### Build

```bash
# Build binary
go build -o ollamagoweb.exe .

# Run tests
go test -v .

# Run E2E tests
npm install
npm run test:e2e
```

### Environment Variables

See `.env.example` for all available configuration variables:

| Variable | Description | Default | Required |
|----------|-------------|---------|-----------|
| `PORT` | Server port | `1102` | No |
| `DB_PATH` | SQLite database path | `./ollamagoweb.db` | No |
| `OLLAMA_HOST` | Ollama server URL | `http://localhost:11434` | No |
| `ENCRYPTION_KEY` | **Required** - Encryption key for API keys | - | Yes (production) |
| `AUTH_USER` | Admin username (optional) | - | No |
| `AUTH_PASSWORD` | Admin password (optional) | - | No |
| `TELEGRAM_BOT_TOKEN` | Telegram bot token from @BotFather | - | No |
| `TELEGRAM_ALLOWED_USERS` | Allowed Telegram user IDs | - | No |
| `brave_api_key` | Brave Search API key | - | No |

---

## 📁 Project Structure

```
ollamagoweb/
├── main.go              # Application entry point and routes
├── handlers.go          # HTTP request handlers
├── handlers_chat.go     # Chat management endpoints
├── handlers_memory.go   # Memory management endpoints
├── handlers_mcp.go      # MCP server endpoints
├── middleware.go        # Rate limiting, CSRF
├── utils.go             # Helper functions
├── database.go          # Database, migrations, pooling
├── crypto.go            # Encryption (AES-GCM)
├── auth.go              # Authentication system
├── provider.go          # Provider implementations
├── memory.go            # Memory extraction and management
├── summarizer.go        # Context summarization
├── telegram.go          # Telegram bot integration
├── search.go            # Brave Search integration
│
├── static/
│   ├── index.html       # Main chat interface
│   ├── css/
│   │   └── styles.css  # Styles, accessibility, animations
│   └── js/
│       ├── app.js      # Main application logic
│       ├── settings.js # Settings page logic
│       ├── state.js    # State management
│       └── ...
│
├── mcp/                 # MCP-related files
├── e2e/
│   ├── tests.spec.js   # Playwright E2E tests
│   └── playwright.config.js
│
├── .env.example         # Environment template
├── package.json        # E2E test scripts
├── README.md
└── go.mod
```

---

## 🎯 Key Highlights

| Feature | Description |
|----------|-------------|
| **Multi-provider** | Ollama, Groq, DeepInfra, OpenRouter, more |
| **Telegram Bot** | Full-featured bot with session linking and typing indicators |
| **Memory System** | LLM-based extraction with categories and confidence scoring |
| **Rolling Summary** | Token-efficient long conversations |
| **System Prompts** | Chat-specific AI instructions |
| **Brave Search** | `/search <query>` for real-time results |
| **Auto-save** | Conversations saved automatically |
| **Message editing** | Edit and track message history |
| **Encrypted secrets** | AES-GCM encryption |
| **Responsive UI** | Desktop and mobile support |
| **Theme support** | Light and dark modes |
| **Keyboard shortcuts** | Efficient keyboard navigation |
| **Export formats** | HTML and JSON export |
| **Rate limiting** | Abuse prevention |
| **CSRF protection** | Request validation |
| **Authentication** | Optional session-based auth |
| **MCP Integration** | Model Context Protocol tool integration |
| **Unit tests** | Core tests passing |
| **E2E tests** | Playwright integration tests |

---

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests: `go test -v .`
5. Submit a pull request

---

## 📄 License

Apache 2.0 License - See LICENSE file for details
