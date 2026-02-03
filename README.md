# OllamaGoWeb Features

This is an improved/enhanced fork of https://github.com/ml2068/ollamagoweb

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
- **Automatic chat saving** - All conversations are automatically saved to the database
- **Chat history sidebar** - Browse and search through previous conversations
- **Rename chats** - Click the edit icon to rename any conversation inline
- **Delete chats** - Remove conversations with 5-second undo option
- **Pinned Chats** - Pin important conversations to the top of the list
- **Floating New Chat** - "New Chat" button (FAB) available on mobile
- **Web Search** - Use `/search <query>` to enrich prompts with Brave Search results

### Message Features
- **Copy-to-clipboard** - One-click copy for code blocks and messages
- **Edit user messages** - Click the pencil icon to modify sent messages inline
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

### Context & Memory
- **Full Conversation Memory** - The LLM remembers previous messages
- **Sliding Window Context** - Manages context window intelligently
- **Token Budgeting** - Intelligent history management within token limits
- **Rolling Summary** - Compresses older messages into concise summaries

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
- **Encrypted API keys** - All API keys encrypted with AES-256-GCM
- **Encryption enforcement** - Application fails if `ENCRYPTION_KEY` not set
- **Secure key migration** - Existing keys are automatically encrypted

---

## 🔐 Authentication

### Session-Based Authentication
- **Optional authentication** - Enable via environment variables
- **Secure session cookies** - HttpOnly, Secure, SameSite
- **SHA-256 password hashing** - Secure credential storage
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
```bash
# Enable authentication
export AUTH_USER=admin
export AUTH_PASSWORD=your-secure-password
```

Without `AUTH_USER` and `AUTH_PASSWORD`, the application runs in public mode (no authentication required).

---

## 📡 WebSocket Support

### Real-Time Features
- **Live connections** - Persistent WebSocket connection at `/ws`
- **Chat rooms** - Join specific chat rooms for targeted updates
- **Typing indicators** - See when other users are typing
- **Message broadcasting** - Real-time message delivery
- **Auto-reconnect** - Built-in ping/pong heartbeat (30s)

### WebSocket Protocol

**Connect:**
```javascript
const ws = new WebSocket('ws://localhost:1102/ws');
```

**Send Messages:**
```javascript
// Join a chat room
ws.send(JSON.stringify({
  type: 'join_chat',
  payload: { chat_id: 123 }
}));

// Leave a chat room
ws.send(JSON.stringify({
  type: 'leave_chat'
}));

// Send typing indicator
ws.send(JSON.stringify({
  type: 'typing'
}));
```

**Receive Messages:**
```javascript
ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);
  switch (msg.type) {
    case 'new_message':
      // New message received
      break;
    case 'user_typing':
      // User is typing
      break;
    case 'chat_updated':
      // Chat metadata changed
      break;
  }
};
```

### Hub Functions
- `WSNotify(messageType, payload)` - Broadcast message to all connected clients
- `BroadcastChatUpdate(chatID, type, data)` - Broadcast to specific chat room
- `BroadcastMessage(chatID, message)` - Broadcast new message
- `WSIsConnected()` - Get count of connected clients

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
API keys and sensitive configuration are encrypted using AES-256-GCM.

**Required for production:**
```bash
# Generate a secure key (Linux/Mac)
openssl rand -hex 32

# Set the environment variable
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
- **Centralized state management** - `state.js` module
- **Modular API client** - `api.js` with typed endpoints
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

### WebSocket

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/ws` | WebSocket connection |

---

## 🚀 Getting Started

### Prerequisites
- Go 1.21 or later
- Node.js (for E2E tests)
- Ollama (for local models) or API key for cloud providers

### Quick Start
```bash
# Clone the repository
git clone <repo-url>
cd ollamagoweb

# Copy environment template
cp .env.example .env

# Edit .env with your configuration
nano .env

# Run the application
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

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | Server port | `1102` |
| `DB_PATH` | SQLite database path | `./ollamagoweb.db` |
| `OLLAMA_HOST` | Ollama server URL | `http://localhost:11434` |
| `ENCRYPTION_KEY` | **Required** - Encryption key for API keys | - |
| `AUTH_USER` | Admin username (optional) | - |
| `AUTH_PASSWORD` | Admin password (optional) | - |

---

## 📁 Project Structure

```
ollamagoweb/
├── main.go              # Application entry point and routes
├── handlers.go          # HTTP request handlers
├── middleware.go        # Rate limiting, CSRF
├── utils.go             # Helper functions
├── database.go          # Database, migrations, pooling
├── crypto.go            # Encryption (AES-256-GCM)
├── auth.go              # Authentication system
├── websocket.go         # WebSocket hub and handlers
├── provider.go          # Provider implementations
├── search.go            # Brave Search integration
├── summarizer.go        # Context summarization
│
├── static/
│   ├── index.html       # Main chat interface
│   ├── css/
│   │   └── styles.css  # Styles, accessibility, animations
│   └── js/
│       ├── app.js      # Main application logic
│       ├── state.js    # ChatState class (centralized state)
│       ├── api.js      # API client modules
│       └── settings.js # Settings page logic
│
├── e2e/
│   ├── tests.spec.js   # Playwright E2E tests
│   └── playwright.config.js
│
├── .env.example         # Environment template
├── .gitignore
├── package.json        # E2E test scripts
├── README.md
└── go.mod
```

---

## 🎯 Key Highlights

| Feature | Description |
|---------|-------------|
| **Multi-provider** | Ollama, Groq, DeepInfra, OpenRouter, more |
| **Brave Search** | `/search <query>` for real-time results |
| **Auto-save** | Conversations saved automatically |
| **Message editing** | Edit and track message history |
| **Encrypted secrets** | AES-256-GCM encryption |
| **Responsive UI** | Desktop and mobile support |
| **Theme support** | Light and dark modes |
| **Rolling Summary** | Token-efficient long conversations |
| **Keyboard shortcuts** | Efficient keyboard navigation |
| **Export formats** | HTML and JSON export |
| **Rate limiting** | Abuse prevention |
| **CSRF protection** | Request validation |
| **Authentication** | Optional session-based auth |
| **WebSocket** | Real-time updates |
| **Unit tests** | Core tests passing |
| **E2E tests** | Playwright integration tests |


## 🤝 Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests: `go test -v .`
5. Submit a pull request

---

## 📄 License

Apache 2.0 License - See LICENSE file for details
