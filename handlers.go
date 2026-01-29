package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"text/template"
	"time"

	"github.com/go-chi/chi"
)

// ProviderResponse represents a provider in API responses
type ProviderResponse struct {
	ID        int64           `json:"id"`
	Name      string          `json:"name"`
	Type      string          `json:"type"`
	BaseURL   string          `json:"base_url,omitempty"`
	HasAPIKey bool            `json:"has_api_key"`
	IsActive  bool            `json:"is_active"`
	Models    []ModelResponse `json:"models"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
}

// ModelResponse represents a model in API responses
type ModelResponse struct {
	ID        int64  `json:"id"`
	ModelName string `json:"model_name"`
	IsDefault bool   `json:"is_default"`
}

// ProviderRequest represents the request body for creating/updating providers
type ProviderRequest struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	BaseURL string   `json:"base_url,omitempty"`
	APIKey  string   `json:"api_key,omitempty"`
	Models  []string `json:"models,omitempty"`
}

// settingsPage renders the settings HTML page
func settingsPage(w http.ResponseWriter, r *http.Request) {
	t, err := template.ParseFiles("static/settings.html")
	if err != nil {
		http.Error(w, "Settings page not found", http.StatusNotFound)
		return
	}
	t.Execute(w, nil)
}

// getProviders returns all providers with their models
func getProviders(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT id, name, type, COALESCE(base_url, ''), api_key IS NOT NULL AND api_key != '', is_active, created_at, updated_at
		FROM providers
		ORDER BY is_active DESC, name ASC
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	providers := []ProviderResponse{}
	for rows.Next() {
		var p ProviderResponse
		var createdAt, updatedAt time.Time
		err := rows.Scan(&p.ID, &p.Name, &p.Type, &p.BaseURL, &p.HasAPIKey, &p.IsActive, &createdAt, &updatedAt)
		if err != nil {
			log.Println("Error scanning provider:", err)
			continue
		}
		p.CreatedAt = createdAt.Format(time.RFC3339)
		p.UpdatedAt = updatedAt.Format(time.RFC3339)

		// Get models for this provider
		p.Models = getModelsForProvider(p.ID)
		providers = append(providers, p)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(providers)
}

// getModelsForProvider returns models for a specific provider
func getModelsForProvider(providerID int64) []ModelResponse {
	rows, err := db.Query(`
		SELECT id, model_name, is_default
		FROM models
		WHERE provider_id = ?
		ORDER BY is_default DESC, model_name ASC
	`, providerID)
	if err != nil {
		log.Println("Error getting models:", err)
		return []ModelResponse{}
	}
	defer rows.Close()

	models := []ModelResponse{}
	for rows.Next() {
		var m ModelResponse
		if err := rows.Scan(&m.ID, &m.ModelName, &m.IsDefault); err != nil {
			log.Println("Error scanning model:", err)
			continue
		}
		models = append(models, m)
	}
	return models
}

// createProvider adds a new provider
func createProvider(w http.ResponseWriter, r *http.Request) {
	var req ProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.Type == "" {
		http.Error(w, "Name and type are required", http.StatusBadRequest)
		return
	}

	if req.Type != "ollama" && req.Type != "openai_compatible" {
		http.Error(w, "Invalid provider type", http.StatusBadRequest)
		return
	}

	// For OpenAI-compatible, require base_url and api_key
	if req.Type == "openai_compatible" && (req.BaseURL == "" || req.APIKey == "") {
		http.Error(w, "Base URL and API key required for OpenAI-compatible providers", http.StatusBadRequest)
		return
	}

	// Encrypt the API key before storing
	encryptedAPIKey := ""
	if req.APIKey != "" {
		var err error
		encryptedAPIKey, err = Encrypt(req.APIKey)
		if err != nil {
			log.Println("Error encrypting API key:", err)
			http.Error(w, "Failed to secure API key", http.StatusInternalServerError)
			return
		}
	}

	result, err := db.Exec(`
		INSERT INTO providers (name, type, base_url, api_key, is_active)
		VALUES (?, ?, ?, ?, 0)
	`, req.Name, req.Type, req.BaseURL, encryptedAPIKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	providerID, _ := result.LastInsertId()

	// Add any models specified
	for i, model := range req.Models {
		isDefault := 0
		if i == 0 {
			isDefault = 1
		}
		db.Exec(`INSERT INTO models (provider_id, model_name, is_default) VALUES (?, ?, ?)`,
			providerID, model, isDefault)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":      providerID,
		"message": "Provider created successfully",
	})
}

// updateProvider updates an existing provider
func updateProvider(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid provider ID", http.StatusBadRequest)
		return
	}

	var req ProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Build update query dynamically
	query := "UPDATE providers SET updated_at = CURRENT_TIMESTAMP"
	args := []interface{}{}

	if req.Name != "" {
		query += ", name = ?"
		args = append(args, req.Name)
	}
	if req.Type != "" {
		query += ", type = ?"
		args = append(args, req.Type)
	}
	if req.BaseURL != "" {
		query += ", base_url = ?"
		args = append(args, req.BaseURL)
	}
	if req.APIKey != "" {
		encryptedAPIKey, err := Encrypt(req.APIKey)
		if err != nil {
			log.Println("Error encrypting API key:", err)
			http.Error(w, "Failed to secure API key", http.StatusInternalServerError)
			return
		}
		query += ", api_key = ?"
		args = append(args, encryptedAPIKey)
	}

	query += " WHERE id = ?"
	args = append(args, id)

	_, err = db.Exec(query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Provider updated successfully"})
}

// deleteProvider removes a provider and its models (cascade)
func deleteProvider(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid provider ID", http.StatusBadRequest)
		return
	}

	// Check if this is the only provider
	var count int
	db.QueryRow("SELECT COUNT(*) FROM providers").Scan(&count)
	if count <= 1 {
		http.Error(w, "Cannot delete the last provider", http.StatusBadRequest)
		return
	}

	// Check if this is the active provider
	var isActive int
	db.QueryRow("SELECT is_active FROM providers WHERE id = ?", id).Scan(&isActive)

	_, err = db.Exec("DELETE FROM providers WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// If we deleted the active provider, make another one active
	if isActive == 1 {
		db.Exec("UPDATE providers SET is_active = 1 WHERE id = (SELECT id FROM providers LIMIT 1)")
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Provider deleted successfully"})
}

// activateProvider sets a provider as the active one
func activateProvider(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid provider ID", http.StatusBadRequest)
		return
	}

	// Deactivate all providers
	db.Exec("UPDATE providers SET is_active = 0")

	// Activate the selected provider
	_, err = db.Exec("UPDATE providers SET is_active = 1, updated_at = CURRENT_TIMESTAMP WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Provider activated successfully"})
}

// getModels returns models for a specific provider
func getModels(w http.ResponseWriter, r *http.Request) {
	providerIDStr := chi.URLParam(r, "providerId")
	providerID, err := strconv.ParseInt(providerIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid provider ID", http.StatusBadRequest)
		return
	}

	models := getModelsForProvider(providerID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models)
}

// fetchModelsFromAPI auto-discovers models from the provider's API
func fetchModelsFromAPI(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid provider ID", http.StatusBadRequest)
		return
	}

	// Get provider config
	var providerType, baseURL, apiKey string
	err = db.QueryRow(`
		SELECT type, COALESCE(base_url, ''), COALESCE(api_key, '')
		FROM providers WHERE id = ?
	`, id).Scan(&providerType, &baseURL, &apiKey)
	if err != nil {
		http.Error(w, "Provider not found", http.StatusNotFound)
		return
	}

	// Decrypt the API key
	if apiKey != "" {
		decryptedKey, err := Decrypt(apiKey)
		if err == nil {
			apiKey = decryptedKey
		}
	}

	ctx := context.Background()
	var models []ModelInfo

	switch providerType {
	case "ollama":
		provider, err := NewOllamaProvider("")
		if err != nil {
			http.Error(w, "Failed to connect to Ollama: "+err.Error(), http.StatusInternalServerError)
			return
		}
		models, err = provider.FetchModels(ctx)
		if err != nil {
			http.Error(w, "Failed to fetch models: "+err.Error(), http.StatusInternalServerError)
			return
		}

	case "openai_compatible":
		provider := NewOpenAIProvider(baseURL, apiKey, "")
		models, err = provider.FetchModels(ctx)
		if err != nil {
			http.Error(w, "Failed to fetch models: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models)
}

// addModel adds a model to a provider
func addModel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProviderID int64  `json:"provider_id"`
		ModelName  string `json:"model_name"`
		IsDefault  bool   `json:"is_default"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ProviderID == 0 || req.ModelName == "" {
		http.Error(w, "Provider ID and model name are required", http.StatusBadRequest)
		return
	}

	// If setting as default, unset other defaults first
	if req.IsDefault {
		db.Exec("UPDATE models SET is_default = 0 WHERE provider_id = ?", req.ProviderID)
	}

	result, err := db.Exec(`
		INSERT INTO models (provider_id, model_name, is_default) VALUES (?, ?, ?)
	`, req.ProviderID, req.ModelName, req.IsDefault)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	modelID, _ := result.LastInsertId()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":      modelID,
		"message": "Model added successfully",
	})
}

// deleteModel removes a model from a provider
func deleteModel(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid model ID", http.StatusBadRequest)
		return
	}

	_, err = db.Exec("DELETE FROM models WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Model deleted successfully"})
}

// setDefaultModel sets a model as the default for its provider
func setDefaultModel(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid model ID", http.StatusBadRequest)
		return
	}

	// Get provider ID for this model
	var providerID int64
	err = db.QueryRow("SELECT provider_id FROM models WHERE id = ?", id).Scan(&providerID)
	if err != nil {
		http.Error(w, "Model not found", http.StatusNotFound)
		return
	}

	// Unset other defaults for this provider
	db.Exec("UPDATE models SET is_default = 0 WHERE provider_id = ?", providerID)

	// Set this model as default
	_, err = db.Exec("UPDATE models SET is_default = 1 WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Default model updated successfully"})
}

// getSetting returns a specific setting value
func getSetting(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")

	var value string
	err := db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		// Return default values
		switch key {
		case "theme":
			value = "light"
		case "temperature":
			value = "0.7"
		case "max_tokens":
			value = "4096"
		case "brave_api_key":
			value = ""
		default:
			http.Error(w, "Setting not found", http.StatusNotFound)
			return
		}
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Mask sensitive keys
	if key == "brave_api_key" && value != "" {
		value = "********"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"key": key, "value": value})
}

// updateSetting updates a specific setting value
func updateSetting(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")

	var req struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Ignore if value is the mask
	if key == "brave_api_key" && req.Value == "********" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "Setting updated successfully (unchanged)"})
		return
	}

	// Encrypt sensitive keys
	if key == "brave_api_key" && req.Value != "" {
		encrypted, err := Encrypt(req.Value)
		if err != nil {
			http.Error(w, "Failed to encrypt key: "+err.Error(), http.StatusInternalServerError)
			return
		}
		req.Value = encrypted
	}

	_, err := db.Exec(`
		INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, req.Value)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Setting updated successfully"})
}

// getActiveProviderInfo returns info about the currently active provider with all its models
func getActiveProviderInfo(w http.ResponseWriter, r *http.Request) {
	_, config, err := GetActiveProvider(db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get all models for this provider
	models := getModelsForProvider(config.ID)
	modelNames := make([]string, 0, len(models))
	for _, m := range models {
		modelNames = append(modelNames, m.ModelName)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":           config.ID,
		"name":         config.Name,
		"type":         config.Type,
		"model":        config.Model,
		"models":       modelNames,
	})
}

// switchModel changes the default model for the active provider
func switchModel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model string `json:"model"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Model == "" {
		http.Error(w, "Model name is required", http.StatusBadRequest)
		return
	}

	// Get active provider
	_, config, err := GetActiveProvider(db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Find the model and set it as default
	var modelID int64
	err = db.QueryRow(`
		SELECT id FROM models WHERE provider_id = ? AND model_name = ?
	`, config.ID, req.Model).Scan(&modelID)
	if err != nil {
		http.Error(w, "Model not found", http.StatusNotFound)
		return
	}

	// Unset other defaults and set this one
	db.Exec("UPDATE models SET is_default = 0 WHERE provider_id = ?", config.ID)
	db.Exec("UPDATE models SET is_default = 1 WHERE id = ?", modelID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Model switched successfully",
		"model":   req.Model,
	})
}

// ChatResponse represents a chat in API responses
type ChatResponse struct {
	ID           int64             `json:"id"`
	Title        string            `json:"title"`
	ProviderName string            `json:"provider_name,omitempty"`
	ModelName    string            `json:"model_name,omitempty"`
	SystemPrompt string            `json:"system_prompt,omitempty"`
	Messages     []MessageResponse `json:"messages,omitempty"`
	CreatedAt    string            `json:"created_at"`
	UpdatedAt    string            `json:"updated_at"`
}

// MessageResponse represents a message in API responses
type MessageResponse struct {
	ID           int64  `json:"id"`
	Role         string `json:"role"`
	Content      string `json:"content"`
	ModelName    string `json:"model_name,omitempty"`
	TokensUsed   int    `json:"tokens_used,omitempty"`
	VersionGroup string `json:"version_group,omitempty"`
	CreatedAt    string `json:"created_at"`
}

// getChats returns all chats (without messages for list view)
func getChats(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT id, title, COALESCE(provider_name, ''), COALESCE(model_name, ''), created_at, updated_at
		FROM chats
		ORDER BY updated_at DESC
		LIMIT 50
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	chats := []ChatResponse{}
	for rows.Next() {
		var c ChatResponse
		var createdAt, updatedAt time.Time
		err := rows.Scan(&c.ID, &c.Title, &c.ProviderName, &c.ModelName, &createdAt, &updatedAt)
		if err != nil {
			log.Println("Error scanning chat:", err)
			continue
		}
		c.CreatedAt = createdAt.Format(time.RFC3339)
		c.UpdatedAt = updatedAt.Format(time.RFC3339)
		chats = append(chats, c)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(chats)
}

// searchChats searches chats by title and message content
func searchChats(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		// Return all chats if no query
		getChats(w, r)
		return
	}

	searchPattern := "%" + query + "%"

	// Search in both chat titles and message content
	rows, err := db.Query(`
		SELECT DISTINCT c.id, c.title, COALESCE(c.provider_name, ''), COALESCE(c.model_name, ''), c.created_at, c.updated_at
		FROM chats c
		LEFT JOIN messages m ON c.id = m.chat_id
		WHERE c.title LIKE ? OR m.content LIKE ?
		ORDER BY c.updated_at DESC
		LIMIT 50
	`, searchPattern, searchPattern)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	chats := []ChatResponse{}
	for rows.Next() {
		var c ChatResponse
		var createdAt, updatedAt time.Time
		err := rows.Scan(&c.ID, &c.Title, &c.ProviderName, &c.ModelName, &createdAt, &updatedAt)
		if err != nil {
			log.Println("Error scanning chat:", err)
			continue
		}
		c.CreatedAt = createdAt.Format(time.RFC3339)
		c.UpdatedAt = updatedAt.Format(time.RFC3339)
		chats = append(chats, c)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(chats)
}

// getChat returns a single chat with all its messages
func getChat(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid chat ID", http.StatusBadRequest)
		return
	}

	var chat ChatResponse
	var createdAt, updatedAt time.Time
	err = db.QueryRow(`
		SELECT id, title, COALESCE(provider_name, ''), COALESCE(model_name, ''), COALESCE(system_prompt, ''), created_at, updated_at
		FROM chats WHERE id = ?
	`, id).Scan(&chat.ID, &chat.Title, &chat.ProviderName, &chat.ModelName, &chat.SystemPrompt, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		http.Error(w, "Chat not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	chat.CreatedAt = createdAt.Format(time.RFC3339)
	chat.UpdatedAt = updatedAt.Format(time.RFC3339)

	// Get messages
	rows, err := db.Query(`
		SELECT id, role, content, COALESCE(model_name, ''), COALESCE(tokens_used, 0), COALESCE(version_group, ''), created_at
		FROM messages
		WHERE chat_id = ?
		ORDER BY created_at ASC
	`, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	chat.Messages = []MessageResponse{}
	for rows.Next() {
		var m MessageResponse
		var msgCreatedAt time.Time
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &m.ModelName, &m.TokensUsed, &m.VersionGroup, &msgCreatedAt); err != nil {
			continue
		}
		m.CreatedAt = msgCreatedAt.Format(time.RFC3339)
		chat.Messages = append(chat.Messages, m)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(chat)
}

// createChat creates a new chat
func createChat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title string `json:"title"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Title == "" {
		req.Title = "New Chat"
	}

	// Get current provider info
	_, config, _ := GetActiveProvider(db)
	var providerName, modelName string
	if config != nil {
		providerName = config.Name
		modelName = config.Model
	}

	result, err := db.Exec(`
		INSERT INTO chats (title, provider_name, model_name) VALUES (?, ?, ?)
	`, req.Title, providerName, modelName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	chatID, _ := result.LastInsertId()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":    chatID,
		"title": req.Title,
	})
}

// addMessage adds a message to a chat
func addMessage(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	chatID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid chat ID", http.StatusBadRequest)
		return
	}

	var req struct {
		Role         string `json:"role"`
		Content      string `json:"content"`
		ModelName    string `json:"model_name,omitempty"`
		TokensUsed   int    `json:"tokens_used,omitempty"`
		VersionGroup string `json:"version_group,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Role != "user" && req.Role != "assistant" {
		http.Error(w, "Invalid role", http.StatusBadRequest)
		return
	}

	result, err := db.Exec(`
		INSERT INTO messages (chat_id, role, content, model_name, tokens_used, version_group) VALUES (?, ?, ?, ?, ?, ?)
	`, chatID, req.Role, req.Content, req.ModelName, req.TokensUsed, req.VersionGroup)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Update chat's updated_at and title if first message
	var msgCount int
	db.QueryRow("SELECT COUNT(*) FROM messages WHERE chat_id = ?", chatID).Scan(&msgCount)
	if msgCount == 1 && req.Role == "user" {
		// Use first user message as title (truncated)
		title := req.Content
		if len(title) > 50 {
			title = title[:47] + "..."
		}
		db.Exec("UPDATE chats SET title = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", title, chatID)
	} else {
		db.Exec("UPDATE chats SET updated_at = CURRENT_TIMESTAMP WHERE id = ?", chatID)
	}

	messageID, _ := result.LastInsertId()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id": messageID,
	})
}

// deleteChat removes a chat and all its messages
func deleteChat(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid chat ID", http.StatusBadRequest)
		return
	}

	_, err = db.Exec("DELETE FROM chats WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Chat deleted successfully"})
}

// renameChat updates a chat's title
func renameChat(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid chat ID", http.StatusBadRequest)
		return
	}

	var req struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Title == "" {
		http.Error(w, "Title is required", http.StatusBadRequest)
		return
	}

	_, err = db.Exec("UPDATE chats SET title = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", req.Title, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Chat renamed successfully",
		"title":   req.Title,
	})
}

// getCurrentChat returns the most recent chat or creates one
func getCurrentChat(w http.ResponseWriter, r *http.Request) {
	var chatID int64
	err := db.QueryRow(`
		SELECT id FROM chats ORDER BY updated_at DESC LIMIT 1
	`).Scan(&chatID)

	if err == sql.ErrNoRows {
		// Create a new chat
		_, config, _ := GetActiveProvider(db)
		var providerName, modelName string
		if config != nil {
			providerName = config.Name
			modelName = config.Model
		}

		result, err := db.Exec(`
			INSERT INTO chats (title, provider_name, model_name) VALUES ('New Chat', ?, ?)
		`, providerName, modelName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		chatID, _ = result.LastInsertId()
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Now get the full chat with messages
	r2 := r.Clone(r.Context())
	chi.RouteContext(r2.Context()).URLParams.Add("id", strconv.FormatInt(chatID, 10))
	getChat(w, r2)
}

// updateSystemPrompt updates the system prompt for a chat
func updateSystemPrompt(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid chat ID", http.StatusBadRequest)
		return
	}

	var req struct {
		SystemPrompt string `json:"system_prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	_, err = db.Exec("UPDATE chats SET system_prompt = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", req.SystemPrompt, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message":       "System prompt updated",
		"system_prompt": req.SystemPrompt,
	})
}

// updateMessage updates a message's content or version_group
func updateMessage(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid message ID", http.StatusBadRequest)
		return
	}

	var req struct {
		Content      string `json:"content,omitempty"`
		VersionGroup string `json:"version_group,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Content == "" && req.VersionGroup == "" {
		http.Error(w, "Content or version_group is required", http.StatusBadRequest)
		return
	}

	// Build dynamic update query
	var result sql.Result
	if req.Content != "" && req.VersionGroup != "" {
		result, err = db.Exec("UPDATE messages SET content = ?, version_group = ? WHERE id = ?", req.Content, req.VersionGroup, id)
	} else if req.Content != "" {
		result, err = db.Exec("UPDATE messages SET content = ? WHERE id = ?", req.Content, id)
	} else {
		result, err = db.Exec("UPDATE messages SET version_group = ? WHERE id = ?", req.VersionGroup, id)
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Message not found", http.StatusNotFound)
		return
	}

	// Also update the chat's updated_at timestamp
	db.Exec("UPDATE chats SET updated_at = CURRENT_TIMESTAMP WHERE id = (SELECT chat_id FROM messages WHERE id = ?)", id)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Message updated",
		"id":      id,
	})
}

// deleteMessage deletes a message
func deleteMessage(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid message ID", http.StatusBadRequest)
		return
	}

	// Get the chat ID before deleting
	var chatID int64
	err = db.QueryRow("SELECT chat_id FROM messages WHERE id = ?", id).Scan(&chatID)
	if err == sql.ErrNoRows {
		http.Error(w, "Message not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Delete the message
	_, err = db.Exec("DELETE FROM messages WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Update chat timestamp
	db.Exec("UPDATE chats SET updated_at = CURRENT_TIMESTAMP WHERE id = ?", chatID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Message deleted",
		"id":      id,
		"chat_id": chatID,
	})
}

// getSystemPrompt returns the system prompt for a chat
func getSystemPrompt(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid chat ID", http.StatusBadRequest)
		return
	}

	var systemPrompt string
	err = db.QueryRow("SELECT COALESCE(system_prompt, '') FROM chats WHERE id = ?", id).Scan(&systemPrompt)
	if err == sql.ErrNoRows {
		http.Error(w, "Chat not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"system_prompt": systemPrompt,
	})
}
