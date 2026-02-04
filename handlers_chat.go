package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi"
)

type ChatResponse struct {
	ID           int64             `json:"id"`
	Title        string            `json:"title"`
	ProviderName string            `json:"provider_name,omitempty"`
	ModelName    string            `json:"model_name,omitempty"`
	SystemPrompt string            `json:"system_prompt,omitempty"`
	Messages     []MessageResponse `json:"messages,omitempty"`
	IsPinned     bool              `json:"is_pinned"`
	CreatedAt    string            `json:"created_at"`
	UpdatedAt    string            `json:"updated_at"`
}

type MessageResponse struct {
	ID           int64  `json:"id"`
	Role         string `json:"role"`
	Content      string `json:"content"`
	ModelName    string `json:"model_name,omitempty"`
	TokensUsed   int    `json:"tokens_used,omitempty"`
	VersionGroup string `json:"version_group,omitempty"`
	CreatedAt    string `json:"created_at"`
}

func sanitizeSearchQuery(query string) string {
	sanitized := strings.ReplaceAll(query, "%", "")
	sanitized = strings.ReplaceAll(sanitized, "_", "")
	sanitized = strings.ReplaceAll(sanitized, "'", "''")
	return sanitized
}

func getChats(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT id, title, COALESCE(provider_name, ''), COALESCE(model_name, ''), created_at, updated_at, is_pinned
		FROM chats
		ORDER BY is_pinned DESC, updated_at DESC
		LIMIT 50
	`)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	chats := []ChatResponse{}
	for rows.Next() {
		var c ChatResponse
		var createdAt, updatedAt time.Time
		err := rows.Scan(&c.ID, &c.Title, &c.ProviderName, &c.ModelName, &createdAt, &updatedAt, &c.IsPinned)
		if err != nil {
			log.Println("Error scanning chat:", err)
			continue
		}
		c.CreatedAt = createdAt.Format(time.RFC3339)
		c.UpdatedAt = updatedAt.Format(time.RFC3339)
		chats = append(chats, c)
	}

	WriteJSON(w, chats)
}

func searchChats(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		getChats(w, r)
		return
	}

	sanitized := sanitizeSearchQuery(query)
	searchPattern := "%" + sanitized + "%"

	rows, err := db.Query(`
		SELECT DISTINCT c.id, c.title, COALESCE(c.provider_name, ''), COALESCE(c.model_name, ''), c.created_at, c.updated_at, c.is_pinned
		FROM chats c
		LEFT JOIN messages m ON c.id = m.chat_id
		WHERE c.title LIKE ? OR m.content LIKE ?
		ORDER BY c.is_pinned DESC, c.updated_at DESC
		LIMIT 50
	`, searchPattern, searchPattern)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	chats := []ChatResponse{}
	for rows.Next() {
		var c ChatResponse
		var createdAt, updatedAt time.Time
		err := rows.Scan(&c.ID, &c.Title, &c.ProviderName, &c.ModelName, &createdAt, &updatedAt, &c.IsPinned)
		if err != nil {
			log.Println("Error scanning chat:", err)
			continue
		}
		c.CreatedAt = createdAt.Format(time.RFC3339)
		c.UpdatedAt = updatedAt.Format(time.RFC3339)
		chats = append(chats, c)
	}

	WriteJSON(w, chats)
}

func getChat(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid chat ID")
		return
	}

	var chat ChatResponse
	var createdAt, updatedAt time.Time
	err = db.QueryRow(`
		SELECT id, title, COALESCE(provider_name, ''), COALESCE(model_name, ''), COALESCE(system_prompt, ''), created_at, updated_at, is_pinned
		FROM chats WHERE id = ?
	`, id).Scan(&chat.ID, &chat.Title, &chat.ProviderName, &chat.ModelName, &chat.SystemPrompt, &createdAt, &updatedAt, &chat.IsPinned)
	if err == sql.ErrNoRows {
		WriteError(w, http.StatusNotFound, "Chat not found")
		return
	}
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	chat.CreatedAt = createdAt.Format(time.RFC3339)
	chat.UpdatedAt = updatedAt.Format(time.RFC3339)

	limit := 100
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 500 {
			limit = parsed
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	rows, err := db.Query(`
		SELECT id, role, content, COALESCE(model_name, ''), COALESCE(tokens_used, 0), COALESCE(version_group, ''), created_at
		FROM messages
		WHERE chat_id = ?
		ORDER BY created_at ASC
		LIMIT ? OFFSET ?
	`, id, limit, offset)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
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

	WriteJSON(w, chat)
}

func createChat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Title == "" {
		req.Title = "New Chat"
	}

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
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	chatID, err := result.LastInsertId()
	if err != nil {
		log.Println("Error getting last insert ID:", err)
	}

	WriteJSON(w, map[string]interface{}{
		"id":    chatID,
		"title": req.Title,
	})
}

func addMessage(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	chatID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid chat ID")
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
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Role != "user" && req.Role != "assistant" {
		WriteError(w, http.StatusBadRequest, "Invalid role")
		return
	}

	result, err := db.Exec(`
		INSERT INTO messages (chat_id, role, content, model_name, tokens_used, version_group) VALUES (?, ?, ?, ?, ?, ?)
	`, chatID, req.Role, req.Content, req.ModelName, req.TokensUsed, req.VersionGroup)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var msgCount int
	err = db.QueryRow("SELECT COUNT(*) FROM messages WHERE chat_id = ?", chatID).Scan(&msgCount)
	if err != nil {
		log.Println("Error counting messages:", err)
	}
	if msgCount == 1 && req.Role == "user" {
		title := req.Content
		if len(title) > 50 {
			title = title[:47] + "..."
		}
		_, err := db.Exec("UPDATE chats SET title = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", title, chatID)
		if err != nil {
			log.Println("Error updating chat title:", err)
		}
	} else {
		_, err := db.Exec("UPDATE chats SET updated_at = CURRENT_TIMESTAMP WHERE id = ?", chatID)
		if err != nil {
			log.Println("Error updating chat timestamp:", err)
		}
	}

	messageID, err := result.LastInsertId()
	if err != nil {
		log.Println("Error getting last insert ID:", err)
	}

	WriteJSON(w, map[string]interface{}{
		"id": messageID,
	})
}

func deleteChat(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid chat ID")
		return
	}

	_, err = db.Exec("DELETE FROM chats WHERE id = ?", id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	WriteJSON(w, map[string]string{"message": "Chat deleted successfully"})
}

func renameChat(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid chat ID")
		return
	}

	var req struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Title == "" {
		WriteError(w, http.StatusBadRequest, "Title is required")
		return
	}

	_, err = db.Exec("UPDATE chats SET title = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", req.Title, id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	WriteJSON(w, map[string]string{
		"message": "Chat renamed successfully",
		"title":   req.Title,
	})
}

func getCurrentChat(w http.ResponseWriter, r *http.Request) {
	var chatID int64
	err := db.QueryRow(`SELECT id FROM chats ORDER BY updated_at DESC LIMIT 1`).Scan(&chatID)

	if err == sql.ErrNoRows {
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
			WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		chatID, err = result.LastInsertId()
		if err != nil {
			log.Println("Error getting last insert ID:", err)
		}
	} else if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	r2 := r.Clone(r.Context())
	chi.RouteContext(r2.Context()).URLParams.Add("id", strconv.FormatInt(chatID, 10))
	getChat(w, r2)
}

func updateSystemPrompt(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid chat ID")
		return
	}

	var req struct {
		SystemPrompt string `json:"system_prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	_, err = db.Exec("UPDATE chats SET system_prompt = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", req.SystemPrompt, id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	WriteJSON(w, map[string]string{
		"message":       "System prompt updated",
		"system_prompt": req.SystemPrompt,
	})
}

func updateMessage(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid message ID")
		return
	}

	var req struct {
		Content      string `json:"content,omitempty"`
		VersionGroup string `json:"version_group,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Content == "" && req.VersionGroup == "" {
		WriteError(w, http.StatusBadRequest, "Content or version_group is required")
		return
	}

	var result sql.Result
	if req.Content != "" && req.VersionGroup != "" {
		result, err = db.Exec("UPDATE messages SET content = ?, version_group = ? WHERE id = ?", req.Content, req.VersionGroup, id)
	} else if req.Content != "" {
		result, err = db.Exec("UPDATE messages SET content = ? WHERE id = ?", req.Content, id)
	} else {
		result, err = db.Exec("UPDATE messages SET version_group = ? WHERE id = ?", req.VersionGroup, id)
	}

	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Println("Error getting rows affected:", err)
	}
	if rowsAffected == 0 {
		WriteError(w, http.StatusNotFound, "Message not found")
		return
	}

	_, err = db.Exec("UPDATE chats SET updated_at = CURRENT_TIMESTAMP WHERE id = (SELECT chat_id FROM messages WHERE id = ?)", id)
	if err != nil {
		log.Println("Error updating chat timestamp:", err)
	}

	WriteJSON(w, map[string]interface{}{
		"message": "Message updated",
		"id":      id,
	})
}

func deleteMessage(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid message ID")
		return
	}

	var chatID int64
	err = db.QueryRow("SELECT chat_id FROM messages WHERE id = ?", id).Scan(&chatID)
	if err == sql.ErrNoRows {
		WriteError(w, http.StatusNotFound, "Message not found")
		return
	}
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	_, err = db.Exec("DELETE FROM messages WHERE id = ?", id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	_, err = db.Exec("UPDATE chats SET updated_at = CURRENT_TIMESTAMP WHERE id = ?", chatID)
	if err != nil {
		log.Println("Error updating chat timestamp:", err)
	}

	WriteJSON(w, map[string]interface{}{
		"message": "Message deleted",
		"id":      id,
		"chat_id": chatID,
	})
}

func getSystemPrompt(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid chat ID")
		return
	}

	var systemPrompt string
	err = db.QueryRow("SELECT COALESCE(system_prompt, '') FROM chats WHERE id = ?", id).Scan(&systemPrompt)
	if err == sql.ErrNoRows {
		WriteError(w, http.StatusNotFound, "Chat not found")
		return
	}
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	WriteJSON(w, map[string]string{
		"system_prompt": systemPrompt,
	})
}

func togglePinChat(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid chat ID")
		return
	}

	var req struct {
		IsPinned bool `json:"is_pinned"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	_, err = db.Exec("UPDATE chats SET is_pinned = ? WHERE id = ?", req.IsPinned, id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	WriteJSON(w, map[string]interface{}{
		"message":   "Chat pin status updated",
		"is_pinned": req.IsPinned,
	})
}
