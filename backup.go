package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi"
)

func RegisterBackupRoutes(r chi.Router, db *sql.DB) {
	r.Get("/api/backup", getBackup(db))
	r.Post("/api/restore", restoreBackup(db))
}

type BackupData struct {
	Version    int          `json:"version"`
	ExportedAt string       `json:"exported_at"`
	Chats      []BackupChat `json:"chats"`
}

type BackupChat struct {
	ID           int64           `json:"id"`
	Title        string          `json:"title"`
	SystemPrompt string          `json:"system_prompt,omitempty"`
	IsPinned     bool            `json:"is_pinned"`
	CreatedAt    string          `json:"created_at"`
	UpdatedAt    string          `json:"updated_at"`
	Messages     []BackupMessage `json:"messages"`
}

type BackupMessage struct {
	ID           int64  `json:"id"`
	Role         string `json:"role"`
	Content      string `json:"content"`
	ModelName    string `json:"model_name,omitempty"`
	TokensUsed   int    `json:"tokens_used,omitempty"`
	VersionGroup string `json:"version_group,omitempty"`
	CreatedAt    string `json:"created_at"`
}

func getBackup(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query(`
			SELECT id, title, COALESCE(system_prompt, ''), is_pinned,
			       COALESCE(created_at, datetime('now')),
			       COALESCE(updated_at, datetime('now'))
			FROM chats
			ORDER BY updated_at DESC
		`)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "Failed to fetch chats")
			return
		}
		defer rows.Close()

		var chats []BackupChat
		for rows.Next() {
			var c BackupChat
			if err := rows.Scan(&c.ID, &c.Title, &c.SystemPrompt, &c.IsPinned, &c.CreatedAt, &c.UpdatedAt); err != nil {
				continue
			}

			msgRows, err := db.Query(`
				SELECT id, role, content,
				       COALESCE(model_name, ''),
				       COALESCE(tokens_used, 0),
				       COALESCE(version_group, ''),
				       COALESCE(created_at, datetime('now'))
				FROM messages
				WHERE chat_id = ?
				ORDER BY id ASC
			`, c.ID)
			if err != nil {
				continue
			}

			var messages []BackupMessage
			for msgRows.Next() {
				var m BackupMessage
				var modelName, versionGroup sql.NullString
				if err := msgRows.Scan(&m.ID, &m.Role, &m.Content, &modelName, &m.TokensUsed, &versionGroup, &m.CreatedAt); err != nil {
					continue
				}
				m.ModelName = modelName.String
				m.VersionGroup = versionGroup.String
				messages = append(messages, m)
			}
			msgRows.Close()

			c.Messages = messages
			chats = append(chats, c)
		}

		backup := BackupData{
			Version:    1,
			ExportedAt: time.Now().Format(time.RFC3339),
			Chats:      chats,
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename=ollamagoweb-backup.json")

		json.NewEncoder(w).Encode(backup)
	}
}

func restoreBackup(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var backup BackupData
		if err := json.NewDecoder(r.Body).Decode(&backup); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid backup file format")
			return
		}

		if backup.Version != 1 {
			WriteError(w, http.StatusBadRequest, "Unsupported backup version")
			return
		}

		imported := 0
		skipped := 0

		for _, chat := range backup.Chats {
			var existingID int64
			err := db.QueryRow("SELECT id FROM chats WHERE title = ? AND updated_at = ?",
				chat.Title, chat.UpdatedAt).Scan(&existingID)

			if err == nil {
				skipped++
				continue
			}

			if err != sql.ErrNoRows {
				continue
			}

			result, err := db.Exec(`
				INSERT INTO chats (id, title, system_prompt, is_pinned, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?)
			`, chat.ID, chat.Title, chat.SystemPrompt, chat.IsPinned, chat.CreatedAt, chat.UpdatedAt)
			if err != nil {
				continue
			}

			var chatID int64
			if chat.ID > 0 {
				chatID = chat.ID
			} else {
				chatID, _ = result.LastInsertId()
			}

			for _, msg := range chat.Messages {
				_, err := db.Exec(`
					INSERT INTO messages (id, chat_id, role, content, model_name, tokens_used, version_group, created_at)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?)
				`, msg.ID, chatID, msg.Role, msg.Content, msg.ModelName, msg.TokensUsed, msg.VersionGroup, msg.CreatedAt)
				if err != nil {
					fmt.Println("Error importing message:", err)
				}
			}

			imported++
		}

		WriteJSON(w, map[string]interface{}{
			"status":   "success",
			"imported": imported,
			"skipped":  skipped,
			"message":  fmt.Sprintf("Imported %d chats, skipped %d duplicates", imported, skipped),
		})
	}
}
