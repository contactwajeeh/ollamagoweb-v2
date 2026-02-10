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

	extractionPrompt := fmt.Sprintf(`You are a memory extraction assistant. Your job is to extract important information from the user's message that should be remembered for future conversations.

Analyze this user message:
"%s"

Extract any memories from these categories:
- reminder: appointments, meetings, tasks, deadlines, things to remember
- fact: personal information, important details about the user
- preference: how the user likes things done, communication style
- entity: people, organizations, locations mentioned

For each memory you find, create a JSON object with these exact fields:
- key: a short unique identifier (use underscores, e.g., "reminder_meeting_ram_5pm")
- value: the full information to remember (e.g., "Meeting with Ram at 5 PM EST")
- category: one of: reminder, fact, preference, entity
- confidence: a number from 70-100 (90-100 for explicit statements, 70-89 for implied)

Return your response as a JSON array containing all the memories you found.

Examples:
Input: "Remind me about my meeting with Ram at 5 PM EST"
Output: [{"key":"reminder_meeting_ram_5pm","value":"Meeting with Ram at 5 PM EST","category":"reminder","confidence":95}]

Input: "My name is John and I prefer concise responses"
Output: [{"key":"name","value":"John","category":"fact","confidence":95},{"key":"response_style","value":"concise","category":"preference","confidence":90}]

If no memories found, return an empty array: []

Respond ONLY with the JSON array. No markdown, no explanation.`, userMessage)

	wr := newResponseWriter()
	if err := provider.Generate(context.Background(), nil, extractionPrompt, "You are a JSON extraction assistant. Always respond with valid JSON arrays only.", wr); err != nil {
		log.Printf("Error extracting memories: %v", err)
		return
	}

	response := strings.TrimSpace(wr.String())
	log.Printf("Raw LLM extraction response (first 500 chars): %s", truncateString(response, 500))

	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimPrefix(response, "JSON:")
	response = strings.TrimPrefix(response, "json:")
	response = strings.TrimSpace(response)
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	startIdx := strings.Index(response, "[")
	endIdx := strings.LastIndex(response, "]")

	if startIdx == -1 || endIdx == -1 || startIdx >= endIdx {
		log.Printf("Error: Could not find JSON array in response. Start: %d, End: %d", startIdx, endIdx)
		return
	}

	jsonStr := response[startIdx : endIdx+1]
	log.Printf("Extracted JSON: %s", jsonStr)

	var extracted []ExtractedMemory
	if err := json.Unmarshal([]byte(jsonStr), &extracted); err != nil {
		log.Printf("Error parsing memory extraction response: %v", jsonStr[:min(len(jsonStr), 200)])
		return
	}

	if len(extracted) == 0 {
		log.Printf("No memories extracted from message")
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
				log.Printf("✓ Extracted and stored memory: [%s] %s = %s", mem.Category, mem.Key, mem.Value)
			}
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

type responseWriter struct {
	strings.Builder
	header http.Header
}

func newResponseWriter() *responseWriter {
	return &responseWriter{
		header: make(http.Header),
	}
}

func (w *responseWriter) Header() http.Header {
	return w.header
}

func (w *responseWriter) WriteHeader(statusCode int) {
	// No-op
}

func (w *responseWriter) Flush() {
	// No-op, satisfy http.Flusher
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
