package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"text/template"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/joho/godotenv"
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

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Static files
	r.Handle("/static/*", http.StripPrefix("/static",
		http.FileServer(http.Dir("./static"))))

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
	r.Delete("/api/chats/{id}", deleteChat)
	r.Get("/api/chats/{id}/system-prompt", getSystemPrompt)
	r.Put("/api/chats/{id}/system-prompt", updateSystemPrompt)

	// Message API routes
	r.Put("/api/messages/{id}", updateMessage)
	r.Delete("/api/messages/{id}", deleteMessage)

	// Model switching
	r.Post("/api/switch-model", switchModel)

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

	// Get chat history
	var history []api.Message
	if prompt.ChatID > 0 {
		rows, err := db.Query(`
			SELECT role, content 
			FROM messages 
			WHERE chat_id = ? 
			ORDER BY created_at ASC
		`, prompt.ChatID)
		if err != nil {
			log.Println("Error fetching history:", err)
		} else {
			defer rows.Close()
			for rows.Next() {
				var role, content string
				if err := rows.Scan(&role, &content); err == nil {
					history = append(history, api.Message{
						Role:    role,
						Content: content,
					})
				}
			}
		}
	}

	// Avoid duplicating the last user message if it's already in the database
	if len(history) > 0 {
		lastMsg := history[len(history)-1]
		if lastMsg.Role == "user" && lastMsg.Content == prompt.Input {
			history = history[:len(history)-1]
		}
	}

	log.Printf("Sending %d history messages to provider", len(history))

	ctx := r.Context()
	if err := provider.Generate(ctx, history, prompt.Input, systemPrompt, w); err != nil {
		log.Println("Generation error:", err)
		// Don't write error if we've already started writing
		// The error will be logged server-side
	}
}

// truncate shortens a string to maxLen characters
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
