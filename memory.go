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
	extractionPrompt := fmt.Sprintf(`You are a memory extraction assistant. Analyze the following user message and extract any important information that should be remembered.

User message: "%s"

Extract memories in these categories:
- reminder: Appointments, meetings, tasks, deadlines, things to remember
- fact: Personal information, preferences, important details about the user
- preference: How the user likes things done, communication style, formatting preferences
- entity: People, organizations, locations mentioned

Return a JSON array of memories with this structure:
[
  {
    "key": "unique_identifier (e.g., reminder_meeting_ram_5pm)",
    "value": "detailed description (e.g., Meeting with Ram at 5 PM EST)",
    "category": "category_name",
    "confidence": 85 (1-100, higher = more confident)"
  }
]

Guidelines:
- Only extract if information is genuinely useful to remember
- For reminders, include date/time/location if mentioned
- Create descriptive but concise keys
- Confidence 90-100 for explicit statements, 70-89 for implied information
- If no memories to extract, return empty array []

Return ONLY the JSON array, no other text.`, userMessage)

	extractionMessages := []api.Message{
		{
			Role:    "system",
			Content: "You are a memory extraction assistant. Always respond with valid JSON arrays.",
		},
		{
			Role:    "user",
			Content: extractionPrompt,
		},
	}

	wr := newResponseWriter()
	if err := provider.Generate(context.Background(), extractionMessages, "", "", wr); err != nil {
		log.Printf("Error extracting memories: %v", err)
		return
	}

	response := strings.TrimSpace(wr.String())

	var extracted []ExtractedMemory
	if err := json.Unmarshal([]byte(response), &extracted); err != nil {
		log.Printf("Error parsing memory extraction response: %v, Response: %s", err, response)
		return
	}

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
				log.Printf("Extracted and stored memory: %s = %s", mem.Key, mem.Value)
			}
		}
	}
}

type responseWriter struct {
	builder    *strings.Builder
	statusCode int
	headers    http.Header
}

func newResponseWriter() *responseWriter {
	return &responseWriter{
		builder: &strings.Builder{},
		headers: make(http.Header),
	}
}

func (w *responseWriter) Write(p []byte) (n int, err error) {
	return w.builder.Write(p)
}

func (w *responseWriter) WriteHeader(code int) {
	w.statusCode = code
}

func (w *responseWriter) Header() http.Header {
	return w.headers
}

func (w *responseWriter) String() string {
	return w.builder.String()
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
