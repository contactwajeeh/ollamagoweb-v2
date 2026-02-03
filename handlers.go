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

type ModelResponse struct {
	ID        int64  `json:"id"`
	ModelName string `json:"model_name"`
	IsDefault bool   `json:"is_default"`
}

type ProviderRequest struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	BaseURL string   `json:"base_url,omitempty"`
	APIKey  string   `json:"api_key,omitempty"`
	Models  []string `json:"models,omitempty"`
}

type Metrics struct {
	ChatsTotal     int     `json:"chats_total"`
	MessagesTotal  int     `json:"messages_total"`
	ProvidersTotal int     `json:"providers_total"`
	ModelsTotal    int     `json:"models_total"`
	UptimeSeconds  float64 `json:"uptime_seconds"`
	Version        string  `json:"version"`
}

var startTime = time.Now()

func settingsPage(w http.ResponseWriter, r *http.Request) {
	t, err := template.ParseFiles("static/settings.html")
	if err != nil {
		http.Error(w, "Settings page not found", http.StatusNotFound)
		return
	}
	t.Execute(w, nil)
}

func getProviders(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT id, name, type, COALESCE(base_url, ''), api_key IS NOT NULL AND api_key != '', is_active, created_at, updated_at
		FROM providers
		ORDER BY is_active DESC, name ASC
	`)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
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
		p.Models = getModelsForProvider(p.ID)
		providers = append(providers, p)
	}

	WriteJSON(w, providers)
}

func getModelsForProvider(providerID int64) []ModelResponse {
	rows, err := db.Query(`
		SELECT id, model_name, is_default
		FROM models
		WHERE provider_id = ?
		ORDER BY is_default DESC, model_name ASC
	`, providerID)
	if err != nil {
		return []ModelResponse{}
	}
	defer rows.Close()

	models := []ModelResponse{}
	for rows.Next() {
		var m ModelResponse
		if err := rows.Scan(&m.ID, &m.ModelName, &m.IsDefault); err != nil {
			continue
		}
		models = append(models, m)
	}
	return models
}

func createProvider(w http.ResponseWriter, r *http.Request) {
	var req ProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Name == "" || req.Type == "" {
		WriteError(w, http.StatusBadRequest, "Name and type are required")
		return
	}

	if req.Type != "ollama" && req.Type != "openai_compatible" {
		WriteError(w, http.StatusBadRequest, "Invalid provider type")
		return
	}

	if req.Type == "openai_compatible" && (req.BaseURL == "" || req.APIKey == "") {
		WriteError(w, http.StatusBadRequest, "Base URL and API key required for OpenAI-compatible providers")
		return
	}

	encryptedAPIKey := ""
	if req.APIKey != "" {
		var err error
		encryptedAPIKey, err = Encrypt(req.APIKey)
		if err != nil {
			log.Println("Error encrypting API key:", err)
			WriteError(w, http.StatusInternalServerError, "Failed to secure API key")
			return
		}
	}

	result, err := db.Exec(`
		INSERT INTO providers (name, type, base_url, api_key, is_active)
		VALUES (?, ?, ?, ?, 0)
	`, req.Name, req.Type, req.BaseURL, encryptedAPIKey)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	providerID, _ := result.LastInsertId()

	for i, model := range req.Models {
		isDefault := 0
		if i == 0 {
			isDefault = 1
		}
		db.Exec(`INSERT INTO models (provider_id, model_name, is_default) VALUES (?, ?, ?)`,
			providerID, model, isDefault)
	}

	WriteJSON(w, map[string]interface{}{
		"id":      providerID,
		"message": "Provider created successfully",
	})
}

func updateProvider(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid provider ID")
		return
	}

	var req ProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

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
			WriteError(w, http.StatusInternalServerError, "Failed to secure API key")
			return
		}
		query += ", api_key = ?"
		args = append(args, encryptedAPIKey)
	}

	query += " WHERE id = ?"
	args = append(args, id)

	_, err = db.Exec(query, args...)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	WriteJSON(w, map[string]string{"message": "Provider updated successfully"})
}

func deleteProvider(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid provider ID")
		return
	}

	var count int
	db.QueryRow("SELECT COUNT(*) FROM providers").Scan(&count)
	if count <= 1 {
		WriteError(w, http.StatusBadRequest, "Cannot delete the last provider")
		return
	}

	var isActive int
	db.QueryRow("SELECT is_active FROM providers WHERE id = ?", id).Scan(&isActive)

	_, err = db.Exec("DELETE FROM providers WHERE id = ?", id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if isActive == 1 {
		db.Exec("UPDATE providers SET is_active = 1 WHERE id = (SELECT id FROM providers LIMIT 1)")
	}

	WriteJSON(w, map[string]string{"message": "Provider deleted successfully"})
}

func activateProvider(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid provider ID")
		return
	}

	db.Exec("UPDATE providers SET is_active = 0")
	_, err = db.Exec("UPDATE providers SET is_active = 1, updated_at = CURRENT_TIMESTAMP WHERE id = ?", id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	WriteJSON(w, map[string]string{"message": "Provider activated successfully"})
}

func getModels(w http.ResponseWriter, r *http.Request) {
	providerIDStr := chi.URLParam(r, "providerId")
	providerID, err := strconv.ParseInt(providerIDStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid provider ID")
		return
	}

	models := getModelsForProvider(providerID)
	WriteJSON(w, models)
}

func fetchModelsFromAPI(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid provider ID")
		return
	}

	var providerType, baseURL, apiKey string
	err = db.QueryRow(`
		SELECT type, COALESCE(base_url, ''), COALESCE(api_key, '')
		FROM providers WHERE id = ?
	`, id).Scan(&providerType, &baseURL, &apiKey)
	if err != nil {
		WriteError(w, http.StatusNotFound, "Provider not found")
		return
	}

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
			WriteError(w, http.StatusInternalServerError, "Failed to connect to Ollama: "+err.Error())
			return
		}
		models, err = provider.FetchModels(ctx)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "Failed to fetch models: "+err.Error())
			return
		}

	case "openai_compatible":
		provider := NewOpenAIProvider(baseURL, apiKey, "")
		models, err = provider.FetchModels(ctx)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "Failed to fetch models: "+err.Error())
			return
		}
	}

	WriteJSON(w, models)
}

func addModel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProviderID int64  `json:"provider_id"`
		ModelName  string `json:"model_name"`
		IsDefault  bool   `json:"is_default"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.ProviderID == 0 || req.ModelName == "" {
		WriteError(w, http.StatusBadRequest, "Provider ID and model name are required")
		return
	}

	if req.IsDefault {
		db.Exec("UPDATE models SET is_default = 0 WHERE provider_id = ?", req.ProviderID)
	}

	result, err := db.Exec(`
		INSERT INTO models (provider_id, model_name, is_default) VALUES (?, ?, ?)
	`, req.ProviderID, req.ModelName, req.IsDefault)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	modelID, _ := result.LastInsertId()

	WriteJSON(w, map[string]interface{}{
		"id":      modelID,
		"message": "Model added successfully",
	})
}

func deleteModel(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid model ID")
		return
	}

	_, err = db.Exec("DELETE FROM models WHERE id = ?", id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	WriteJSON(w, map[string]string{"message": "Model deleted successfully"})
}

func setDefaultModel(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid model ID")
		return
	}

	var providerID int64
	err = db.QueryRow("SELECT provider_id FROM models WHERE id = ?", id).Scan(&providerID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "Model not found")
		return
	}

	db.Exec("UPDATE models SET is_default = 0 WHERE provider_id = ?", providerID)
	_, err = db.Exec("UPDATE models SET is_default = 1 WHERE id = ?", id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	WriteJSON(w, map[string]string{"message": "Default model updated successfully"})
}

func getSetting(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")

	var value string
	err := db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
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
			WriteError(w, http.StatusNotFound, "Setting not found")
			return
		}
	} else if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if key == "brave_api_key" && value != "" {
		value = "********"
	}

	WriteJSON(w, map[string]string{"key": key, "value": value})
}

func updateSetting(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")

	var req struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if key == "brave_api_key" && req.Value == "********" {
		WriteJSON(w, map[string]string{"message": "Setting updated successfully (unchanged)"})
		return
	}

	if key == "brave_api_key" && req.Value != "" {
		encrypted, err := Encrypt(req.Value)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "Failed to encrypt key: "+err.Error())
			return
		}
		req.Value = encrypted
	}

	_, err := db.Exec(`
		INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, req.Value)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	WriteJSON(w, map[string]string{"message": "Setting updated successfully"})
}

func getActiveProviderInfo(w http.ResponseWriter, r *http.Request) {
	_, config, err := GetActiveProvider(db)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	models := getModelsForProvider(config.ID)
	modelNames := make([]string, 0, len(models))
	for _, m := range models {
		modelNames = append(modelNames, m.ModelName)
	}

	WriteJSON(w, map[string]interface{}{
		"id":     config.ID,
		"name":   config.Name,
		"type":   config.Type,
		"model":  config.Model,
		"models": modelNames,
	})
}

func switchModel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model string `json:"model"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Model == "" {
		WriteError(w, http.StatusBadRequest, "Model name is required")
		return
	}

	_, config, err := GetActiveProvider(db)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var modelID int64
	err = db.QueryRow(`
		SELECT id FROM models WHERE provider_id = ? AND model_name = ?
	`, config.ID, req.Model).Scan(&modelID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "Model not found")
		return
	}

	db.Exec("UPDATE models SET is_default = 0 WHERE provider_id = ?", config.ID)
	db.Exec("UPDATE models SET is_default = 1 WHERE id = ?", modelID)

	WriteJSON(w, map[string]string{
		"message": "Model switched successfully",
		"model":   req.Model,
	})
}

func getMetrics(w http.ResponseWriter, r *http.Request) {
	var chatCount int
	db.QueryRow("SELECT COUNT(*) FROM chats").Scan(&chatCount)

	var messageCount int
	db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&messageCount)

	var providerCount int
	db.QueryRow("SELECT COUNT(*) FROM providers").Scan(&providerCount)

	var modelCount int
	db.QueryRow("SELECT COUNT(*) FROM models").Scan(&modelCount)

	metrics := Metrics{
		ChatsTotal:     chatCount,
		MessagesTotal:  messageCount,
		ProvidersTotal: providerCount,
		ModelsTotal:    modelCount,
		UptimeSeconds:  time.Since(startTime).Seconds(),
		Version:        "1.0.0",
	}

	WriteJSON(w, metrics)
}
