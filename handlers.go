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
		SELECT p.id, p.name, p.type, COALESCE(p.base_url, ''), p.api_key IS NOT NULL AND p.api_key != '', p.is_active, p.created_at, p.updated_at
		FROM providers p
		ORDER BY p.is_active DESC, p.name ASC
	`)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	type providerWithModels struct {
		ID        int64
		Name      string
		Type      string
		BaseURL   string
		HasAPIKey bool
		IsActive  bool
		CreatedAt time.Time
		UpdatedAt time.Time
	}

	var providersWithIDs []providerWithModels

	for rows.Next() {
		var p providerWithModels
		err := rows.Scan(&p.ID, &p.Name, &p.Type, &p.BaseURL, &p.HasAPIKey, &p.IsActive, &p.CreatedAt, &p.UpdatedAt)
		if err != nil {
			log.Println("Error scanning provider:", err)
			continue
		}
		providersWithIDs = append(providersWithIDs, p)
	}

	if err := rows.Err(); err != nil {
		WriteError(w, http.StatusInternalServerError, "Error iterating providers: "+err.Error())
		return
	}

	if len(providersWithIDs) == 0 {
		WriteJSON(w, []ProviderResponse{})
		return
	}

	providerIDs := make([]int64, len(providersWithIDs))
	for i, p := range providersWithIDs {
		providerIDs[i] = p.ID
	}

	modelsByProviderID := make(map[int64][]ModelResponse)
	modelRows, err := db.Query(`
		SELECT id, model_name, is_default, provider_id
		FROM models
		WHERE provider_id IN (`+placeholders(len(providerIDs))+`)
		ORDER BY is_default DESC, model_name ASC
	`, intsToInterfaces(providerIDs...)...)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer modelRows.Close()

	for modelRows.Next() {
		var m ModelResponse
		var providerID int64
		if err := modelRows.Scan(&m.ID, &m.ModelName, &m.IsDefault, &providerID); err != nil {
			log.Println("Error scanning model:", err)
			continue
		}
		modelsByProviderID[providerID] = append(modelsByProviderID[providerID], m)
	}

	providers := make([]ProviderResponse, 0, len(providersWithIDs))
	for _, p := range providersWithIDs {
		providers = append(providers, ProviderResponse{
			ID:        p.ID,
			Name:      p.Name,
			Type:      p.Type,
			BaseURL:   p.BaseURL,
			HasAPIKey: p.HasAPIKey,
			IsActive:  p.IsActive,
			CreatedAt: p.CreatedAt.Format(time.RFC3339),
			UpdatedAt: p.UpdatedAt.Format(time.RFC3339),
			Models:    modelsByProviderID[p.ID],
		})
	}

	WriteJSON(w, providers)
}

func intsToInterfaces(ints ...int64) []interface{} {
	ifaces := make([]interface{}, len(ints))
	for i, v := range ints {
		ifaces[i] = v
	}
	return ifaces
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	result := "?"
	for i := 1; i < n; i++ {
		result += ", ?"
	}
	return result
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

	providerID, err := result.LastInsertId()
	if err != nil {
		log.Println("Error getting last insert ID:", err)
	}

	for i, model := range req.Models {
		isDefault := 0
		if i == 0 {
			isDefault = 1
		}
		_, err := db.Exec(`INSERT INTO models (provider_id, model_name, is_default) VALUES (?, ?, ?)`,
			providerID, model, isDefault)
		if err != nil {
			log.Println("Error inserting model:", err)
		}
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
	err = db.QueryRow("SELECT COUNT(*) FROM providers").Scan(&count)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if count <= 1 {
		WriteError(w, http.StatusBadRequest, "Cannot delete the last provider")
		return
	}

	var isActive int
	err = db.QueryRow("SELECT is_active FROM providers WHERE id = ?", id).Scan(&isActive)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	_, err = db.Exec("DELETE FROM providers WHERE id = ?", id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if isActive == 1 {
		_, err = db.Exec("UPDATE providers SET is_active = 1 WHERE id = (SELECT id FROM providers LIMIT 1)")
		if err != nil {
			log.Println("Error setting new active provider:", err)
		}
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

	_, err = db.Exec("UPDATE providers SET is_active = 0")
	if err != nil {
		log.Println("Error deactivating all providers:", err)
	}
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
		_, err := db.Exec("UPDATE models SET is_default = 0 WHERE provider_id = ?", req.ProviderID)
		if err != nil {
			log.Println("Error clearing default models:", err)
		}
	}

	result, err := db.Exec(`
		INSERT INTO models (provider_id, model_name, is_default) VALUES (?, ?, ?)
	`, req.ProviderID, req.ModelName, req.IsDefault)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	modelID, err := result.LastInsertId()
	if err != nil {
		log.Println("Error getting last insert ID:", err)
	}

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

	_, err = db.Exec("UPDATE models SET is_default = 0 WHERE provider_id = ?", providerID)
	if err != nil {
		log.Println("Error clearing default models:", err)
	}
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

	_, err = db.Exec("UPDATE models SET is_default = 0 WHERE provider_id = ?", config.ID)
	if err != nil {
		log.Println("Error clearing default models:", err)
	}
	_, err = db.Exec("UPDATE models SET is_default = 1 WHERE id = ?", modelID)
	if err != nil {
		log.Println("Error setting default model:", err)
	}

	WriteJSON(w, map[string]string{
		"message": "Model switched successfully",
		"model":   req.Model,
	})
}

func getMetrics(w http.ResponseWriter, r *http.Request) {
	var chatCount int
	err := db.QueryRow("SELECT COUNT(*) FROM chats").Scan(&chatCount)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var messageCount int
	err = db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&messageCount)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var providerCount int
	err = db.QueryRow("SELECT COUNT(*) FROM providers").Scan(&providerCount)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var modelCount int
	err = db.QueryRow("SELECT COUNT(*) FROM models").Scan(&modelCount)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

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
