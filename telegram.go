package main

import (
	"context"
	"database/sql"
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

	sendTypingIndicator(chatID)
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
				"Your session ID: %s\n\n"+
				"Commands:\n"+
				"/start - Start a new session\n"+
				"/help - Show this help\n"+
				"/memories - View your memories\n"+
				"/clear - Clear conversation history\n"+
				"/settings - Show your settings\n"+
				"/search <query> - Search the web\n"+
				"/skills - List available skills\n\n"+
				"🔗 Session Linking:\n"+
				"/link_session <id> <token> - Link Telegram to web session\n"+
				"/unlink_session - Unlink from web session\n"+
				"/session_info - Show session status",
			sessionID,
		)
		sendTelegramMessage(chatID, msg)

	case "help":
		msg := "📖 Available Commands:\n\n" +
			"💬 Chat:\n" +
			"  /start - Start a new session\n" +
			"  /clear - Clear current conversation\n" +
			"  /settings - Show your current settings\n" +
			"  /search <query> - Search the web for info\n\n" +
			"📚 Skills:\n" +
			"  /skills - List available Open Skills\n" +
			"  /refresh_skills - Refresh skills from repository\n\n" +
			"🧠 Memory:\n" +
			"  /memories - View your saved memories\n\n" +
			"🔗 Session Linking:\n" +
			"  /link_session <id> <token> - Link Telegram to web session\n" +
			"  /unlink_session - Unlink from web session\n" +
			"  /session_info - Show session status\n\n" +
			"❓ Get link token from web: GET /api/session/link-token"
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

	case "link_session":
		if len(parts) < 3 {
			sendTelegramMessage(chatID,
				"❌ Usage: /link_session <session_id> <link_token>\n\n"+
					"To get your link token:\n"+
					"1. Visit web app: http://localhost:1102\n"+
					"2. Open browser console (F12)\n"+
					"3. Run: fetch('/api/session/link-token').then(r=>r.json()).then(console.log)\n"+
					"4. Copy session_id and link_token")
			return
		}

		sessionIDToLink := parts[1]
		linkToken := parts[2]

		var dbSessionID sql.NullString
		var expiresAt time.Time
		var usedAt sql.NullTime

		err := db.QueryRow(`
			SELECT session_id, expires_at, used_at
			FROM session_link_tokens
			WHERE token = ?
		`, linkToken).Scan(&dbSessionID, &expiresAt, &usedAt)

		if err != nil {
			sendTelegramMessage(chatID, "❌ Invalid or expired link token. Please generate a new token on web.")
			return
		}

		if dbSessionID.String != sessionIDToLink {
			sendTelegramMessage(chatID, "❌ Session ID mismatch. Make sure you copied at correct session_id.")
			return
		}

		if time.Now().After(expiresAt) {
			sendTelegramMessage(chatID, "❌ Link token has expired (valid for 15 minutes). Please generate a new token.")
			return
		}

		if usedAt.Valid {
			sendTelegramMessage(chatID, "❌ This link token has already been used. Please generate a new token.")
			return
		}

		var sessionExists int
		err = db.QueryRow("SELECT COUNT(*) FROM sessions WHERE id = ?", sessionIDToLink).Scan(&sessionExists)
		if err != nil || sessionExists == 0 {
			sendTelegramMessage(chatID, "❌ Invalid session. Please check your session ID.")
			return
		}

		_, err = db.Exec(`
			INSERT OR REPLACE INTO telegram_users (telegram_user_id, session_id)
			VALUES (?, ?)
		`, userID, sessionIDToLink)

		if err != nil {
			sendTelegramMessage(chatID, "❌ Error linking session: "+err.Error())
			return
		}

		_, err = db.Exec("UPDATE session_link_tokens SET used_at = CURRENT_TIMESTAMP WHERE token = ?", linkToken)
		if err != nil {
			log.Printf("Warning: Failed to mark link token as used: %v", err)
		}

		sendTelegramMessage(chatID, "✅ Session Linked Successfully!\n\n🔗 Session ID: "+sessionIDToLink+"\n\nYour Telegram and web chats will now share:\n• Memories\n• Chat history\n• Context\n\nUse /session_info to see details.")

	case "unlink_session":
		_, err := db.Exec("DELETE FROM telegram_users WHERE telegram_user_id = ?", userID)
		if err != nil {
			sendTelegramMessage(chatID, "❌ Error unlinking: "+err.Error())
			return
		}
		sendTelegramMessage(chatID, "✅ Session Unlinked\n\nYour Telegram chats will now use a separate session. Memories and context will not be shared with web.")

	case "session_info":
		var linkedSessionID sql.NullString
		var linkedAt sql.NullTime

		err := db.QueryRow(`
			SELECT session_id, linked_at FROM telegram_users WHERE telegram_user_id = ?
		`, userID).Scan(&linkedSessionID, &linkedAt)

		if err == sql.ErrNoRows {
			currentSession := getTelegramSession(userID)
			msg := fmt.Sprintf("📱 Session Info\n\nStatus: 🔓 Unlinked\n\nCurrent Session ID: %s\n\nTo link with web, use:\n/link_session <session_id> <token>\n\nGet your link token from:\nGET /api/session/link-token", currentSession)
			sendTelegramMessage(chatID, msg)
			return
		}

		msg := fmt.Sprintf("🔗 Linked Session Info\n\nStatus: ✅ Linked\nSession ID: %s\nLinked at: %s\n\nYour memories and context are shared with web.",
			linkedSessionID.String, linkedAt.Time.Format("2006-01-02 15:04"))
		sendTelegramMessage(chatID, msg)

	case "search":
		if len(parts) < 2 {
			sendTelegramMessage(chatID, "❌ Usage: /search <query>\n\nExample: /search latest AI news")
			return
		}
		sessionID := getTelegramSession(userID)
		searchQuery := "/search " + strings.Join(parts[1:], " ")
		sendTypingIndicator(chatID)
		response := generateResponseForSession(sessionID, searchQuery)
		sendTelegramMessage(chatID, response)

	case "skills":
		ctx := context.Background()
		skills, err := GetCachedSkills(ctx)
		if err != nil || len(skills) == 0 {
			skills, err = RefreshSkillsCache(ctx)
			if err != nil {
				sendTelegramMessage(chatID, "❌ Failed to fetch skills. Please try again later.")
				return
			}
		}

		var sb strings.Builder
		sb.WriteString("📚 Available Open Skills:\n\n")
		for i, s := range skills {
			if i >= 20 {
				sb.WriteString(fmt.Sprintf("\n...and %d more skills", len(skills)-20))
				break
			}
			sb.WriteString(fmt.Sprintf("• %s\n  %s\n", s.Name, truncateString(s.Description, 50)))
		}
		sb.WriteString("\n💡 Just ask naturally and I'll use the right skill!")
		sendTelegramMessage(chatID, sb.String())

	case "refresh_skills":
		sendTelegramMessage(chatID, "🔄 Refreshing skills from Open Skills repository...")
		ctx := context.Background()
		skills, err := RefreshSkillsCache(ctx)
		if err != nil {
			sendTelegramMessage(chatID, fmt.Sprintf("❌ Failed to refresh skills: %v", err))
			return
		}
		sendTelegramMessage(chatID, fmt.Sprintf("✅ Refreshed %d skills!", len(skills)))

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
	var linkedSessionID string
	var linkedAt sql.NullTime

	err := db.QueryRow(`
		SELECT session_id, linked_at FROM telegram_users WHERE telegram_user_id = ?
	`, userID).Scan(&linkedSessionID, &linkedAt)

	if err == nil && linkedSessionID != "" {
		var exists int
		if checkErr := db.QueryRow("SELECT COUNT(*) FROM sessions WHERE id = ?", linkedSessionID).Scan(&exists); checkErr == nil && exists > 0 {
			log.Printf("Using linked session for Telegram user %d: %s (linked at %s)",
				userID, linkedSessionID, linkedAt.Time.Format("2006-01-02 15:04"))
			return linkedSessionID
		}
		log.Printf("Linked session %s for user %d no longer exists, using Telegram-only session", linkedSessionID, userID)
	}

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

	var braveAPIKey string
	err = db.QueryRow("SELECT value FROM settings WHERE key = 'brave_api_key'").Scan(&braveAPIKey)
	if err != nil && err != sql.ErrNoRows {
		log.Println("Error fetching Brave API key:", err)
	}

	if braveAPIKey != "" {
		decrypted, err := Decrypt(braveAPIKey)
		if err != nil {
			log.Println("Error decrypting Brave API key:", err)
		} else {
			braveAPIKey = decrypted
		}
	}

	enrichedPrompt, err := MaybeSearch(userMessage, braveAPIKey)
	if err != nil {
		if strings.HasPrefix(userMessage, "/search ") {
			return fmt.Sprintf("❌ Search error: %v", err)
		}
		enrichedPrompt = userMessage
	}

	chatID, err := getOrCreateChatForSession(sessionID)
	if err != nil {
		log.Printf("Error getting/creating chat for session: %v", err)
		return fmt.Sprintf("❌ Error getting chat: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if IsMemoryEnabled(db) {
		ExtractMemoriesWithLLM(db, sessionID, userMessage, provider, nil)
	}

	var chatSummary sql.NullString
	err = db.QueryRow("SELECT summary FROM chats WHERE id = ?", chatID).Scan(&chatSummary)
	if err != nil {
		log.Printf("Error fetching chat summary for Telegram session %s: %v", sessionID, err)
	}

	var history []api.Message

	if chatSummary.String != "" {
		history = append(history, api.Message{
			Role:    "system",
			Content: fmt.Sprintf("Here is a summary of earlier conversation:\n%s", chatSummary.String),
		})
	}

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
		WHERE chat_id = ? AND is_summarized = 0 AND role IN ('user', 'assistant')
		ORDER BY id ASC
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

	var systemPrompt string
	if chatID > 0 {
		db.QueryRow("SELECT COALESCE(system_prompt, '') FROM chats WHERE id = ?", chatID).Scan(&systemPrompt)
	}

	log.Printf("Telegram sending %d messages to provider (systemPrompt='%s')", len(history), truncateString(systemPrompt, 50))

	for i, msg := range history {
		log.Printf("  [%d] %s: %s", i, msg.Role, truncateString(msg.Content, 100))
	}

	tools, err := GetAllEnabledMCPTools(ctx)
	if err != nil {
		log.Printf("Warning: Failed to get MCP tools: %v", err)
		tools = nil
	}

	skills, err := GetCachedSkills(ctx)
	if err != nil {
		log.Printf("Warning: Failed to get Open Skills: %v", err)
		skills = nil
	}

	var toolExecutionMessages []string
	callback := func(toolName string, status string) {
		var msg string
		switch status {
		case "calling":
			msg = fmt.Sprintf("🔧 Calling tool: %s...", toolName)
		case "completed":
			msg = fmt.Sprintf("✅ Tool completed: %s", toolName)
		case "error":
			msg = fmt.Sprintf("❌ Tool error: %s", toolName)
		}
		toolExecutionMessages = append(toolExecutionMessages, msg)
		log.Printf("Telegram tool execution: %s", msg)
	}

	var response string
	if len(tools) > 0 || len(skills) > 0 {
		log.Printf("Telegram: Running agentic loop with %d tools and %d skills", len(tools), len(skills))
		response, err = RunAgenticLoopWithSkills(ctx, provider, tools, skills, history, enrichedPrompt, systemPrompt, callback)
	} else {
		response, err = provider.GenerateNonStreaming(ctx, history, enrichedPrompt, systemPrompt)
	}

	if err != nil {
		log.Printf("Error generating Telegram response: %v", err)
		return "❌ Error generating response. Please try again."
	}

	response = strings.TrimSpace(response)

	if idx := strings.Index(response, "__ANALYTICS__"); idx != -1 {
		response = strings.TrimSpace(response[:idx])
	}

	aiResponse := response
	if len(toolExecutionMessages) > 0 {
		aiResponse = strings.Join(toolExecutionMessages, "\n") + "\n\n" + aiResponse
	}
	log.Printf("Telegram LLM response (first 300 chars): %s", truncateString(response, 300))

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

	if chatID > 0 {
		MaybeTriggerSummarization(db, chatID)
	}

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

func sendTypingIndicator(chatID int64) {
	action := tgbotapi.NewChatAction(chatID, "typing")
	_, err := telegramBot.Request(action)
	if err != nil {
		log.Printf("Error sending typing indicator: %v", err)
	}
}
