package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/ollama/ollama/api"
)

const (
	SummaryThreshold = 10 // Trigger summarization when we have 10+ unsummarized messages
	SummaryBatchSize = 10 // Convert 10 messages into a summary
)

// StringResponseWriter mocks http.ResponseWriter to capture output
type StringResponseWriter struct {
	strings.Builder
	header http.Header
}

func NewStringResponseWriter() *StringResponseWriter {
	return &StringResponseWriter{
		header: make(http.Header),
	}
}

func (w *StringResponseWriter) Header() http.Header {
	return w.header
}

func (w *StringResponseWriter) WriteHeader(statusCode int) {
	// No-op
}

func (w *StringResponseWriter) Flush() {
	// No-op, satisfy http.Flusher
}

// MaybeTriggerSummarization checks if a chat needs summarization and runs it in background
func MaybeTriggerSummarization(db *sql.DB, chatID int64) {
	var count int
	// Check how many messages are NOT summarized yet
	// We only count assistant/user messages, ignoring system
	err := db.QueryRow("SELECT COUNT(*) FROM messages WHERE chat_id = ? AND is_summarized = 0 AND role IN ('user', 'assistant')", chatID).Scan(&count)
	if err != nil {
		log.Println("Error checking summarization trigger:", err)
		return
	}

	// If we have enough unsummarized messages, trigger the worker
	// We want to keep at least some recent context raw, so typically we trigger
	// when we have Threshold + Buffer. But simpler: Trigger when > Threshold,
	// and the summarizer itself will decide what to pick.
	if count >= SummaryThreshold { // e.g. 10 messages
		go summarizeChat(db, chatID)
	}
}

func summarizeChat(db *sql.DB, chatID int64) {
	log.Printf("Starting background summarization for chat %d...", chatID)

	// 1. Get the active provider to generate the summary
	provider, _, err := GetActiveProvider(db)
	if err != nil {
		log.Println("Summarization skipped: No active provider")
		return
	}

	// 2. Fetch current summary
	var currentSummary sql.NullString
	err = db.QueryRow("SELECT summary FROM chats WHERE id = ?", chatID).Scan(&currentSummary)
	if err != nil {
		log.Println("Error fetching current summary:", err)
		return
	}

	// 3. Fetch the oldest BATCH of unsummarized messages
	// We preserve the order by ID ASC.
	rows, err := db.Query(`
		SELECT id, role, content 
		FROM messages 
		WHERE chat_id = ? AND is_summarized = 0 AND role IN ('user', 'assistant')
		ORDER BY id ASC 
		LIMIT ?`, chatID, SummaryBatchSize)
	if err != nil {
		log.Println("Error fetching messages for summary:", err)
		return
	}
	defer rows.Close()

	type msg struct {
		ID      int64
		Role    string
		Content string
	}
	var batch []msg
	var batchIDs []int64

	for rows.Next() {
		var m msg
		if err := rows.Scan(&m.ID, &m.Role, &m.Content); err != nil {
			continue
		}
		batch = append(batch, m)
		batchIDs = append(batchIDs, m.ID)
	}

	if len(batch) < SummaryBatchSize {
		// Not enough messages to form a full batch? 
		// Actually MaybeTriggerSummarization check should cover this, but safe to check.
		// If we are strictly rolling, we can proceed.
		// But maybe we want to always leave the LAST few messages unsummarized for immediate context?
		// If we updated ALL 'is_summarized=0', we would leave 0 raw messages.
		// This logic fetches the OLDEST unsummarized. So if we have 15 unsummarized,
		// and batch size is 10, we summarize the old 10, leaving 5 raw. This is perfect.
		if len(batch) == 0 {
			return 
		}
	}

	// 4. Construct the prompt
	var conversationText string
	for _, m := range batch {
		role := "User"
		if m.Role == "assistant" {
			role = "Assistant"
		}
		conversationText += fmt.Sprintf("%s: %s\n", role, m.Content)
	}

	var prompt string
	if currentSummary.String != "" {
		prompt = fmt.Sprintf(`You are a helpful context compressor. 
Current Conversation Summary:
"""%s"""

New Conversation Chunk to Integrate:
"""%s"""

Task: Create a cohesive, concise summary that merges the "New Conversation Chunk" into the "Current Conversation Summary". Preserves key facts, names, decisions, and context. The output should be a plain text narrative.
Updated Summary:`, currentSummary.String, conversationText)
	} else {
		prompt = fmt.Sprintf(`You are a helpful context compressor.
Conversation Chunk:
"""%s"""

Task: Create a concise summary of this conversation chunk. Preserve key facts, names, and user intent. The output should be a plain text narrative.
Summary:`, conversationText)
	}

	// 5. Generate Summary
	writer := NewStringResponseWriter()
	// We pass empty history because the prompt contains everything needed
	ctx := context.Background()
	err = provider.Generate(ctx, []api.Message{}, prompt, "", writer)
	if err != nil {
		log.Println("Error generating summary:", err)
		return
	}

	newSummary := strings.TrimSpace(writer.String())
	
	// Remove any artifacts like "Here is the summary:" if model chats too much (simple cleanup)
	// For reasoning models, we might get <think> blocks. We should probably strip them?
	// But our basic text extraction should work.
	
	// 6. Update Database
	tx, err := db.Begin()
	if err != nil {
		log.Println("Error starting transaction:", err)
		return
	}

	// Save new summary
	_, err = tx.Exec("UPDATE chats SET summary = ? WHERE id = ?", newSummary, chatID)
	if err != nil {
		tx.Rollback()
		log.Println("Error updating chat summary:", err)
		return
	}

	// Mark messages as summarized
	// building "ID IN (?,?,?)" query
	query := "UPDATE messages SET is_summarized = 1 WHERE id IN ("
	args := make([]interface{}, len(batchIDs))
	for i, id := range batchIDs {
		if i > 0 {
			query += ","
		}
		query += "?"
		args[i] = id
	}
	query += ")"

	_, err = tx.Exec(query, args...)
	if err != nil {
		tx.Rollback()
		log.Println("Error marking messages summarized:", err)
		return
	}

	if err := tx.Commit(); err != nil {
		log.Println("Error committing summary transaction:", err)
		return
	}

	log.Printf("Successfully summarized %d messages for chat %d", len(batch), chatID)
}
