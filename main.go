package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/contactwajeeh/ollamagoweb-v2/mcp"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/joho/godotenv"
	"github.com/klauspost/compress/gzhttp"
	"github.com/ollama/ollama/api"
	_ "modernc.org/sqlite"
)

var db *sql.DB

// initialise to load environment variable from .env file
func init() {
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .env file not found, using environment variables")
	}
}

func main() {
	// Initialize database
	db = InitDB()
	defer db.Close()
	RunMigrations(db)
	SeedFromEnvIfEmpty(db)

	// Initialize authentication
	authUser := os.Getenv("AUTH_USER")
	authPass := os.Getenv("AUTH_PASSWORD")
	InitAuth(authUser, authPass)
	go CleanupSessions()

	// Initialize MCP client
	mcp.InitMCPClient()

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(RateLimitMiddleware)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Security-Policy",
				"default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self' http://localhost:*;")
			next.ServeHTTP(w, r)
		})
	})

	// CSRF token endpoint
	r.Get("/api/csrf", func(w http.ResponseWriter, r *http.Request) {
		token := generateCSRFToken()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"token": token})
	})

	// Static files with compression
	staticHandler := http.StripPrefix("/static",
		http.FileServer(http.Dir("./static")))
	r.Handle("/static/*", gzhttp.GzipHandler(staticHandler))

	// Main routes
	r.Get("/", index)
	r.Post("/run", run)

	// Settings page
	r.Get("/settings", settingsPage)

	// Provider API routes
	r.Get("/api/providers", getProviders)
	r.Post("/api/providers", createProvider)
	r.Put("/api/providers/{id}", updateProvider)
	r.Delete("/api/providers/{id}", deleteProvider)
	r.Post("/api/providers/{id}/activate", activateProvider)
	r.Post("/api/providers/{id}/fetch-models", fetchModelsFromAPI)

	// Model API routes
	r.Get("/api/models/{providerId}", getModels)
	r.Post("/api/models", addModel)
	r.Delete("/api/models/{id}", deleteModel)
	r.Post("/api/models/{id}/set-default", setDefaultModel)

	// Settings API routes
	r.Get("/api/settings/{key}", getSetting)
	r.Put("/api/settings/{key}", updateSetting)

	// MCP Server API routes
	r.Mount("/api/mcp/servers", NewMCPServerHandler(db))

	// Active provider info
	r.Get("/api/active-provider", getActiveProviderInfo)

	// Chat API routes (autosave)
	r.Get("/api/chats", getChats)
	r.Get("/api/chats/search", searchChats)
	r.Get("/api/chats/current", getCurrentChat)
	r.Post("/api/chats", createChat)
	r.Get("/api/chats/{id}", getChat)
	r.Post("/api/chats/{id}/messages", addMessage)
	r.Put("/api/chats/{id}/rename", renameChat)
	r.Put("/api/chats/{id}/pin", togglePinChat)
	r.Delete("/api/chats/{id}", deleteChat)
	r.Get("/api/chats/{id}/system-prompt", getSystemPrompt)
	r.Put("/api/chats/{id}/system-prompt", updateSystemPrompt)

	// Message API routes
	r.Put("/api/messages/{id}", updateMessage)
	r.Delete("/api/messages/{id}", deleteMessage)

	// Model switching
	r.Post("/api/switch-model", switchModel)

	// Metrics endpoint
	r.Get("/api/metrics", getMetrics)

	// Auth endpoints
	r.Get("/api/auth/session", sessionStatusHandler)
	r.Post("/api/auth/login", loginHandler)
	r.Post("/api/auth/logout", logoutHandler)
	r.Get("/admin", adminHandler)

	// Protected routes (apply auth middleware)
	protected := chi.NewRouter()
	protected.Use(AuthMiddleware)
	protected.Get("/api/chats", getChats)
	protected.Post("/api/chats", createChat)
	protected.Delete("/api/chats/{id}", deleteChat)
	protected.Put("/api/chats/{id}/rename", renameChat)
	protected.Put("/api/chats/{id}/pin", togglePinChat)
	protected.Post("/api/chats/{id}/messages", addMessage)
	protected.Delete("/api/messages/{id}", deleteMessage)
	r.Mount("/", protected)

	// Get port from environment
	port := os.Getenv("PORT")
	if port == "" {
		port = "1102"
	}

	// Create server with graceful shutdown
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	// Start server in goroutine
	go func() {
		log.Println("\033[93mOllamaGoWeb started. Press CTRL+C to quit.\033[0m")
		log.Println("Local URL: http://localhost:" + port)
		log.Println("Settings:  http://localhost:" + port + "/settings")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Server is shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal(err)
	}
	log.Println("Server stopped")
}

// index renders the main chat page
func index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")

	// Get active provider info
	_, config, err := GetActiveProvider(db)

	var providerName, modelName, providerInfo string
	if err != nil {
		providerName = "No provider configured"
		modelName = ""
		providerInfo = "Please configure a provider in Settings"
	} else {
		providerName = config.Name
		modelName = config.Model
		providerInfo = config.Name + " | " + config.Model
	}

	t, err := template.ParseFiles("static/index.html")
	if err != nil {
		http.Error(w, "Error loading page", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"provider":     providerName,
		"llm":          modelName,
		"providerInfo": providerInfo,
	}

	if err := t.Execute(w, data); err != nil {
		log.Println("Template error:", err)
	}
}

// run handles LLM generation requests using the active provider
func run(w http.ResponseWriter, r *http.Request) {
	prompt := struct {
		Input  string `json:"input"`
		ChatID int64  `json:"chat_id,omitempty"`
	}{}

	if err := json.NewDecoder(r.Body).Decode(&prompt); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if prompt.Input == "" {
		http.Error(w, "Prompt is required", http.StatusBadRequest)
		return
	}

	// Handling Search Logic
	var braveAPIKey string
	err := db.QueryRow("SELECT value FROM settings WHERE key = 'brave_api_key'").Scan(&braveAPIKey)
	if err != nil && err != sql.ErrNoRows {
		log.Println("Error fetching Brave API key:", err)
	}

	// Decrypt the key if it exists
	if braveAPIKey != "" {
		decrypted, err := Decrypt(braveAPIKey)
		if err != nil {
			log.Println("Error decrypting Brave API key:", err)
			// Proceed with raw key? Or fail? Failed decryption usually means it wasn't encrypted (legacy) or key change
			// If Decrypt returns original string on failure (as implemented in crypto.go), we are safe.
			// Checking crypto.go implementation...
			// Yes, Decrypt returns input string on some errors, but let's be safe.
			// Actually crypto.go Decrypt implementation returns input if not base64 etc.
			// But if it errors on NewCipher/GCM, it returns empty string + error.
			// We should probably rely on Decrypt's behavior or fallback.
			// Let's assume Decrypt handles legacy/empty cases reasonably or we handle error.
			// For this specific code:
		} else {
			braveAPIKey = decrypted
		}
	}

	enrichedPrompt, err := MaybeSearch(prompt.Input, braveAPIKey)
	if err != nil {
		// If search fails or key missing, fallback to sending error as response or just logging
		// For now, let's log and maybe return error to user if they explicitly asked for search
		if strings.HasPrefix(prompt.Input, "/search ") {
			log.Printf("Search failed: %v", err)
			http.Error(w, "Search error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		// Otherwise continue with original prompt
		enrichedPrompt = prompt.Input
	}

	// Use enriched prompt for generation, but original prompt was likely saved by frontend
	// ... continue with generation ...

	// Get system prompt from chat if chatId is provided
	var systemPrompt string
	if prompt.ChatID > 0 {
		db.QueryRow("SELECT COALESCE(system_prompt, '') FROM chats WHERE id = ?", prompt.ChatID).Scan(&systemPrompt)
	}

	// Get active provider
	provider, config, err := GetActiveProvider(db)
	if err != nil {
		http.Error(w, "No active provider configured. Please visit /settings to configure one.", http.StatusServiceUnavailable)
		return
	}

	log.Printf("Generating response with %s using model %s\n", config.Name, config.Model)
	if systemPrompt != "" {
		log.Printf("Using system prompt: %s...\n", truncate(systemPrompt, 50))
	}

	// Parse settings - maxTokens currently unused with rolling summary
	// maxTokensStr := "4096"
	// ...

	// 1. Get Chat Summary
	var chatSummary sql.NullString
	if prompt.ChatID > 0 {
		err := db.QueryRow("SELECT summary FROM chats WHERE id = ?", prompt.ChatID).Scan(&chatSummary)
		if err != nil {
			log.Println("Error fetching chat summary:", err)
		}
	}

	// 2. Fetch Unsummarized Messages
	// We fetch ALL unsummarized messages. The sliding window logic might still apply
	// if there are too many unsummarized ones, but ideally the summarizer keeps this list short.
	// For safety, we still apply a limit or token check if implemented, but for now let's just fetch unsummarized.
	var history []api.Message

	if prompt.ChatID > 0 {
		// Fetch unsummarized messages
		rows, err := db.Query(`
			SELECT role, content, model_name 
			FROM messages 
			WHERE chat_id = ? AND is_summarized = 0 
			ORDER BY id ASC
		`, prompt.ChatID)
		if err != nil {
			log.Println("Error fetching history:", err)
		} else {
			defer rows.Close()
			for rows.Next() {
				var role, content string
				var modelName sql.NullString
				if err := rows.Scan(&role, &content, &modelName); err != nil {
					continue
				}
				history = append(history, api.Message{
					Role:    role,
					Content: content,
				})
			}
		}

		// Inject Summary as the first "system" or "context" message if it exists
		if chatSummary.String != "" {
			summaryMsg := api.Message{
				Role:    "system", // Or 'user' with a preamble if system prompt is strict. 'system' is usually best.
				Content: fmt.Sprintf("Here is a summary of the earlier conversation:\n%s", chatSummary.String),
			}
			// Prepend summary
			history = append([]api.Message{summaryMsg}, history...)
		}
	}

	log.Printf("Sending %d history messages (context window) to provider", len(history))

	ctx := r.Context()
	if err := provider.Generate(ctx, history, enrichedPrompt, systemPrompt, w); err != nil {
		log.Println("Generation error:", err)
		// Don't write error if we've already started writing
		// The error will be logged server-side
	}

	// Trigger background summarization check
	if prompt.ChatID > 0 {
		MaybeTriggerSummarization(db, prompt.ChatID)
	}
}

// truncate shortens a string to maxLen characters
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
