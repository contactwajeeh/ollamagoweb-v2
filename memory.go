package main

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Memory struct {
	ID         int64  `json:"id"`
	SessionID  string `json:"session_id"`
	Key        string `json:"key"`
	Value      string `json:"value"`
	Category   string `json:"category"`
	Confidence int    `json:"confidence"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

func SetMemory(db *sql.DB, sessionID, key, value, category string, confidence int) error {
	query := `
		INSERT INTO user_memories (session_id, key, value, category, confidence)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(session_id, key) DO UPDATE SET
			value = ?, confidence = ?, updated_at = CURRENT_TIMESTAMP
	`
	_, err := db.Exec(query, sessionID, key, value, category, confidence, value, confidence)
	return err
}

func GetMemories(db *sql.DB, sessionID string) ([]Memory, error) {
	rows, err := db.Query(`
		SELECT id, session_id, key, value, category, confidence, created_at, updated_at
		FROM user_memories
		WHERE session_id = ?
		ORDER BY created_at DESC
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []Memory
	for rows.Next() {
		var m Memory
		var createdAt, updatedAt time.Time
		err := rows.Scan(&m.ID, &m.SessionID, &m.Key, &m.Value, &m.Category, &m.Confidence, &createdAt, &updatedAt)
		if err != nil {
			continue
		}
		m.CreatedAt = createdAt.Format(time.RFC3339)
		m.UpdatedAt = updatedAt.Format(time.RFC3339)
		memories = append(memories, m)
	}
	return memories, nil
}

func DeleteMemory(db *sql.DB, sessionID, key string) error {
	_, err := db.Exec("DELETE FROM user_memories WHERE session_id = ? AND key = ?", sessionID, key)
	return err
}

func FormatMemoriesForPrompt(memories []Memory) string {
	if len(memories) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n=== USER MEMORY ===\n")
	for _, m := range memories {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", m.Key, m.Value))
	}
	sb.WriteString("=== END MEMORY ===\n")
	return sb.String()
}

func ExtractAndStoreMemory(db *sql.DB, sessionID, userMessage string) {
	lowerMsg := strings.ToLower(userMessage)

	if strings.Contains(lowerMsg, "prefer") {
		if strings.Contains(lowerMsg, "concise") {
			SetMemory(db, sessionID, "response_style", "concise", "preference", 80)
		}
		if strings.Contains(lowerMsg, "detailed") {
			SetMemory(db, sessionID, "response_style", "detailed", "preference", 80)
		}
	}

	if strings.Contains(lowerMsg, "speak in spanish") {
		SetMemory(db, sessionID, "language", "spanish", "preference", 90)
	}

	if strings.Contains(lowerMsg, "my name is") {
		parts := strings.Split(userMessage, "my name is")
		if len(parts) > 1 {
			name := strings.TrimSpace(strings.Split(parts[1], ".")[0])
			SetMemory(db, sessionID, "name", name, "fact", 95)
		}
	}
}
