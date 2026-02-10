package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/ollama/ollama/api"
)

var (
	telegramBot      *tgbotapi.BotAPI
	telegramCtx      context.Context
	telegramCancel   context.CancelFunc
	telegramSessions = make(map[int64]string)
	telegramMutex    sync.RWMutex
	allowedUsers     []int64
)

func InitTelegramBot() {
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	if botToken == "" {
		log.Println("Telegram bot not configured (TELEGRAM_BOT_TOKEN not set)")
		return
	}

	log.Println("Initializing Telegram bot...")

	var err error
	telegramBot, err = tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Printf("Failed to create Telegram bot: %v", err)
		return
	}

	telegramCtx, telegramCancel = context.WithCancel(context.Background())

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := telegramBot.GetUpdatesChan(u)

	go func() {
		for {
			update, ok := <-updates
			if !ok {
				break
			}
			message := update.Message
			if message == nil {
				continue
			}
			go handleTelegramMessage(message)
		}
	}()

	log.Println("Telegram bot started and listening for messages...")
}

func initAllowedUsers() {
	allowedUsersEnv := os.Getenv("TELEGRAM_ALLOWED_USERS")
	if allowedUsersEnv == "" {
		allowedUsers = []int64{}
		return
	}

	ids := strings.Split(allowedUsersEnv, ",")
	allowedUsers = make([]int64, 0, len(ids))

	for _, idStr := range ids {
		idStr = strings.TrimSpace(idStr)
		if idStr == "" {
			continue
		}
		var userID int64
		_, err := fmt.Sscanf(idStr, "%d", &userID)
		if err != nil {
			log.Printf("Warning: Invalid user ID in allowlist: %s", idStr)
			continue
		}
		allowedUsers = append(allowedUsers, userID)
	}

	log.Printf("Telegram allowlist configured with %d user(s)", len(allowedUsers))
}

func isUserAllowed(userID int64) bool {
	if len(allowedUsers) == 0 {
		return true
	}

	for _, allowedID := range allowedUsers {
		if allowedID == userID {
			return true
		}
	}

	return false
}

func handleTelegramMessage(message *tgbotapi.Message) {
	if message.Text == "" {
		return
	}

	userID := message.From.ID
	chatID := message.Chat.ID

	log.Printf("Telegram message from user %d: %s", userID, message.Text)

	if !isUserAllowed(userID) {
		log.Printf("Unauthorized access attempt from user %d", userID)
		sendTelegramMessage(chatID, "🚫 Access Denied: You are not authorized to use this bot.")
		return
	}

	if strings.HasPrefix(message.Text, "/") {
		handleTelegramCommand(message, userID, chatID)
		return
	}

	sessionID := getTelegramSession(userID)
	response := generateResponseForSession(sessionID, message.Text)

	sendTelegramMessage(chatID, response)
}

func handleTelegramCommand(message *tgbotapi.Message, userID, chatID int64) {
	command := strings.TrimPrefix(message.Text, "/")
	parts := strings.Fields(command)
	cmd := parts[0]

	switch cmd {
	case "start":
		sessionID := createTelegramSession(userID)
		msg := fmt.Sprintf(
			"👋 Welcome to OllamaGoWeb Bot!\n\n"+
				"Your session ID: `%s`\n\n"+
				"Commands:\n"+
				"/start - Start a new session\n"+
				"/help - Show this help\n"+
				"/memories - View your memories\n"+
				"/clear - Clear conversation history\n"+
				"/settings - Show your settings",
			sessionID,
		)
		sendTelegramMessage(chatID, msg)

	case "help":
		msg :=
			"📖 Available Commands:\n\n" +
				"/start - Start a new session\n" +
				"/help - Show this help\n" +
				"/memories - View your saved memories\n" +
				"/clear - Clear current conversation\n" +
				"/settings - Show your current settings"
		sendTelegramMessage(chatID, msg)

	case "memories":
		sessionID := getTelegramSession(userID)
		memories, err := GetMemories(db, sessionID)
		if err != nil || len(memories) == 0 {
			sendTelegramMessage(chatID, "📭 No memories saved yet.")
			return
		}

		var sb strings.Builder
		sb.WriteString("📋 Your Memories:\n\n")
		for i, mem := range memories {
			if i >= 10 {
				sb.WriteString("\n...and more")
				break
			}
			sb.WriteString(fmt.Sprintf("• %s: %s\n", mem.Key, mem.Value))
		}
		sendTelegramMessage(chatID, sb.String())

	case "clear":
		_ = getTelegramSession(userID)
		newSessionID := fmt.Sprintf("telegram_%d_%d", userID, time.Now().Unix())
		telegramMutex.Lock()
		telegramSessions[userID] = newSessionID
		telegramMutex.Unlock()

		sendTelegramMessage(chatID, "🧹 Conversation cleared! Starting a new session.")

	case "settings":
		providerName := "Unknown"
		modelName := "Unknown"

		if _, config, err := GetActiveProvider(db); err == nil && config != nil {
			providerName = config.Name
			modelName = config.Model
		}

		msg := fmt.Sprintf(
			"⚙️ Current Settings:\n\n"+
				"Memory: ✅ Enabled\n"+
				"Provider: %s\n"+
				"Model: %s",
			providerName, modelName,
		)
		sendTelegramMessage(chatID, msg)

	default:
		sendTelegramMessage(chatID, fmt.Sprintf("❓ Unknown command: /%s\n\nUse /help for available commands.", cmd))
	}
}

func createTelegramSession(userID int64) string {
	sessionID := fmt.Sprintf("telegram_%d_%d", userID, time.Now().Unix())

	telegramMutex.Lock()
	telegramSessions[userID] = sessionID
	telegramMutex.Unlock()

	return sessionID
}

func getTelegramSession(userID int64) string {
	telegramMutex.RLock()
	defer telegramMutex.RUnlock()

	sessionID, exists := telegramSessions[userID]
	if !exists {
		return createTelegramSession(userID)
	}
	return sessionID
}

func generateResponseForSession(sessionID, userMessage string) string {
	provider, config, err := GetActiveProvider(db)
	if err != nil {
		return "❌ Error: No active provider configured in web settings."
	}

	log.Printf("Generating response for Telegram session %s with provider: %s, model: %s", sessionID, config.Name, config.Model)

	chatID, err := getOrCreateChatForSession(sessionID)
	if err != nil {
		log.Printf("Error getting/creating chat for session: %v", err)
		return fmt.Sprintf("❌ Error getting chat: %v", err)
	}

	wr := newResponseWriter()
	wr2 := newResponseWriter()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

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

	if IsMemoryEnabled(db) {
		log.Printf("Extracting memories for user message: %s", userMessage)
		provider.Generate(ctx, nil, extractionPrompt, "You are a JSON extraction assistant. Always respond with valid JSON arrays only.", wr)

		response := strings.TrimSpace(wr.String())
		log.Printf("Memory extraction response (first 200 chars): %s", truncateString(response, 200))

		cleanedResponse := strings.TrimSpace(response)
		cleanedResponse = strings.TrimPrefix(cleanedResponse, "```json")
		cleanedResponse = strings.TrimPrefix(cleanedResponse, "```")
		cleanedResponse = strings.TrimSpace(cleanedResponse)
		cleanedResponse = strings.TrimSuffix(cleanedResponse, "```")
		cleanedResponse = strings.TrimSpace(cleanedResponse)

		startIdx := strings.Index(cleanedResponse, "[")
		endIdx := strings.LastIndex(cleanedResponse, "]")
		if startIdx != -1 && endIdx != -1 && startIdx < endIdx {
			jsonStr := cleanedResponse[startIdx : endIdx+1]
			var extracted []ExtractedMemory
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
							log.Printf("Extracted memory: [%s] %s = %s", mem.Category, mem.Key, mem.Value)
						}
					}
				}
			}
		}
	}

	var history []api.Message

	if IsMemoryEnabled(db) {
		memories, _ := GetMemories(db, sessionID)
		if len(memories) > 0 {
			memoryPrompt := FormatMemoriesForPrompt(memories)
			history = append(history, api.Message{
				Role:    "system",
				Content: fmt.Sprintf("You have access to the following information about this user:\n%s\nUse this information to personalize your responses.", memoryPrompt),
			})
		}
	}

	rows, err := db.Query(`
		SELECT role, content
		FROM messages
		WHERE chat_id = ?
		ORDER BY created_at DESC
		LIMIT 10
	`, chatID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var role, content string
			rows.Scan(&role, &content)
			history = append(history, api.Message{
				Role:    role,
				Content: content,
			})
		}
	}

	for i, j := 0, len(history)-1; i < j; i, j = i+1, j-1 {
		history[i], history[j] = history[j], history[i]
	}

	history = append(history, api.Message{
		Role:    "user",
		Content: userMessage,
	})

	log.Printf("Sending %d history messages to provider for generation", len(history))

	if err := provider.Generate(ctx, history, "", "", wr2); err != nil {
		log.Printf("Error generating Telegram response: %v", err)
		return "❌ Error generating response. Please try again."
	}

	response := strings.TrimSpace(wr2.String())
	log.Printf("Telegram LLM response (first 300 chars): %s", truncateString(response, 300))

	aiResponse := response

	if _, err := db.Exec(`
		INSERT INTO messages (chat_id, role, content, model_name)
		VALUES (?, 'user', ?, ?)
	`, chatID, userMessage, config.Model); err != nil {
		log.Printf("Error saving Telegram message to database: %v", err)
	}

	if _, err := db.Exec(`
		INSERT INTO messages (chat_id, role, content, model_name)
		VALUES (?, 'assistant', ?, ?)
	`, chatID, aiResponse, config.Model); err != nil {
		log.Printf("Error saving Telegram response to database: %v", err)
	}

	db.Exec("UPDATE chats SET updated_at = CURRENT_TIMESTAMP WHERE id = ?", chatID)

	return aiResponse
}

func getOrCreateChatForSession(sessionID string) (int64, error) {
	var chatID int64
	err := db.QueryRow("SELECT id FROM chats WHERE title = ?", sessionID).Scan(&chatID)

	if err == nil {
		return chatID, nil
	}

	if err != nil && err.Error() != "sql: no rows in result set" {
		return 0, err
	}

	_, config, err := GetActiveProvider(db)
	if err != nil {
		return 0, err
	}

	var providerName, modelName string
	if config != nil {
		providerName = config.Name
		modelName = config.Model
	}

	result, err := db.Exec(`
		INSERT INTO chats (title, provider_name, model_name)
		VALUES (?, ?, ?)
	`, sessionID, providerName, modelName)
	if err != nil {
		return 0, err
	}

	chatID, err = result.LastInsertId()
	return chatID, err
}

func sendTelegramMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"

	if _, err := telegramBot.Send(msg); err != nil {
		log.Printf("Error sending Telegram message: %v", err)
	}
}

func StopTelegramBot() {
	if telegramCancel != nil {
		telegramCancel()
	}
	log.Println("Telegram bot stopped")
}
