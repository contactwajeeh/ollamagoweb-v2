package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/ollama/ollama/api"
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

type ExtractedMemory struct {
	Key        string `json:"key"`
	Value      string `json:"value"`
	Category   string `json:"category"`
	Confidence int    `json:"confidence"`
}

func ExtractMemoriesWithLLM(db *sql.DB, sessionID, userMessage string, provider Provider, history []api.Message) {
	log.Printf("Starting LLM memory extraction for message: %s", userMessage)

	extractionPrompt := fmt.Sprintf(`You are a memory extraction assistant. Analyze the following user message and extract any important information that should be remembered.

User message: "%s"

Extract memories in these categories:
- reminder: Appointments, meetings, tasks, deadlines, things to remember
- fact: Personal information, preferences, important details about user
- preference: How the user likes things done, communication style, formatting preferences
- entity: People, organizations, locations mentioned

For each memory you find, create a JSON object with these exact fields:
- key: a short unique identifier (use underscores, e.g., "reminder_meeting_ram_5pm")
- value: full information to remember (e.g., "Meeting with Ram at 5 PM EST")
- category: one of: reminder, fact, preference, entity
- confidence: a number from 70-100 (90-100 for explicit statements, 70-89 for implied information)

Return your response as a JSON array containing all the memories you found.

Examples:
Input: "Remind me about my meeting with Ram at 5 PM EST"
Output: [{"key":"reminder_meeting_ram_5pm","value":"Meeting with Ram at 5 PM EST","category":"reminder","confidence":95}]

Input: "My name is John and I prefer concise responses"
Output: [{"key":"name","value":"John","category":"fact","confidence":95},{"key":"response_style","value":"concise","category":"preference","confidence":90}]

If no memories found, return an empty array: []

Respond ONLY with a JSON array. No markdown, no explanation.`, userMessage)

	wr := newResponseWriter()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	provider.Generate(ctx, nil, extractionPrompt, "You are a JSON extraction assistant. Always respond with valid JSON arrays only.", wr)

	response := strings.TrimSpace(wr.String())
	log.Printf("LLM extraction response (first 500 chars): %s", truncateString(response, 500))

	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSpace(response)
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	startIdx := strings.Index(response, "[")
	endIdx := strings.LastIndex(response, "]")
	var extracted []ExtractedMemory
	if startIdx != -1 && endIdx != -1 && startIdx < endIdx {
		jsonStr := response[startIdx : endIdx+1]
		if err := json.Unmarshal([]byte(jsonStr), &extracted); err == nil {
			for _, mem := range extracted {
				if mem.Key != "" && mem.Value != "" {
					category := mem.Category
					if category == "" {
						category = "fact"
					}
					confidence := mem.Confidence
					if confidence <= 0 {
						confidence = 80
					}
					if err := SetMemory(db, sessionID, mem.Key, mem.Value, category, confidence); err != nil {
						log.Printf("Error storing extracted memory: %v", err)
					} else {
						log.Printf("✓ Extracted and stored memory: [%s] %s = %s", mem.Category, mem.Key, mem.Value)
					}
				}
			}
		}
	}

	if len(extracted) == 0 {
		log.Printf("No memories extracted from message")
	}
}

type responseWriter struct {
	strings.Builder
}

func newResponseWriter() *responseWriter {
	return &responseWriter{}
}

func (w *responseWriter) Write(p []byte) (n int, err error) {
	return w.Builder.Write(p)
}

func (w *responseWriter) WriteHeader(statusCode int) {
}

func (w *responseWriter) Header() http.Header {
	return nil
}

func (w *responseWriter) Flush() {
}

func SearchMemories(db *sql.DB, sessionID, query string) ([]Memory, error) {
	searchPattern := "%" + query + "%"

	rows, err := db.Query(`
		SELECT id, session_id, key, value, category, confidence, created_at, updated_at
		FROM user_memories
		WHERE session_id = ? AND (
			key LIKE ? OR value LIKE ? OR category LIKE ?
		)
		ORDER BY created_at DESC
		LIMIT 50
	`, sessionID, searchPattern, searchPattern, searchPattern)
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

func IsMemoryEnabled(db *sql.DB) bool {
	var value string
	err := db.QueryRow("SELECT value FROM settings WHERE key = ?", "memory_enabled").Scan(&value)
	if err == sql.ErrNoRows {
		return true
	}
	if err != nil {
		log.Printf("Error checking memory_enabled setting: %v", err)
		return true
	}

	return value == "1" || strings.ToLower(value) == "true" || strings.ToLower(value) == "yes"
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
