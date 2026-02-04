package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

// InitDB initializes the SQLite database connection
func InitDB() *sql.DB {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./ollamagoweb.db"
	}

	db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)")
	if err != nil {
		log.Fatal("Failed to open database:", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Test the connection
	if err := db.Ping(); err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	log.Println("Database connected:", dbPath)
	return db
}

// RunMigrations creates the required tables if they don't exist
func RunMigrations(db *sql.DB) {
	migrations := []string{
		// Providers table
		`CREATE TABLE IF NOT EXISTS providers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			type TEXT NOT NULL CHECK(type IN ('ollama', 'openai_compatible')),
			base_url TEXT,
			api_key TEXT,
			is_active INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// Models table
		`CREATE TABLE IF NOT EXISTS models (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			provider_id INTEGER NOT NULL,
			model_name TEXT NOT NULL,
			is_default INTEGER DEFAULT 0,
			FOREIGN KEY (provider_id) REFERENCES providers(id) ON DELETE CASCADE
		)`,

		// Settings table
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,

		// Chats table for autosave
		`CREATE TABLE IF NOT EXISTS chats (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL,
			provider_name TEXT,
			model_name TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// Messages table for chat history
		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			chat_id INTEGER NOT NULL,
			role TEXT NOT NULL CHECK(role IN ('user', 'assistant')),
			content TEXT NOT NULL,
			model_name TEXT,
			tokens_used INTEGER,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (chat_id) REFERENCES chats(id) ON DELETE CASCADE
		)`,

		// Sessions table for persistent authentication
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// MCP Servers table
		`CREATE TABLE IF NOT EXISTS mcp_servers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			server_type TEXT NOT NULL CHECK(server_type IN ('http', 'stdio')),
			endpoint_url TEXT,
			command TEXT,
			args TEXT,
			env_vars TEXT,
			is_enabled INTEGER DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// Indexes
		`CREATE INDEX IF NOT EXISTS idx_models_provider ON models(provider_id)`,
		`CREATE INDEX IF NOT EXISTS idx_providers_active ON providers(is_active)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_chat ON messages(chat_id)`,
		`CREATE INDEX IF NOT EXISTS idx_chats_updated ON chats(updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_chats_pinned ON chats(is_pinned, updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_unsummarized ON messages(chat_id, is_summarized) WHERE is_summarized = 0`,
		`CREATE INDEX IF NOT EXISTS idx_messages_role ON messages(chat_id, role)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at)`,
		`CREATE INDEX IF NOT EXISTS idx_mcp_servers_enabled ON mcp_servers(is_enabled)`,
	}

	for _, migration := range migrations {
		_, err := db.Exec(migration)
		if err != nil {
			log.Fatal("Migration failed:", err)
		}
	}

	// Add columns only if they don't exist (schema upgrades)
	columnsToAdd := map[string][]struct {
		Table  string
		Column string
		Schema string
	}{
		"messages": {
			{"messages", "model_name", "TEXT"},
			{"messages", "tokens_used", "INTEGER"},
			{"messages", "version_group", "TEXT"},
			{"messages", "is_summarized", "INTEGER DEFAULT 0"},
		},
		"chats": {
			{"chats", "system_prompt", "TEXT"},
			{"chats", "summary", "TEXT"},
			{"chats", "is_pinned", "INTEGER DEFAULT 0"},
		},
	}

	for table, columns := range columnsToAdd {
		for _, col := range columns {
			if !columnExists(db, table, col.Column) {
				_, err := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, col.Column, col.Schema))
				if err != nil {
					log.Printf("Warning: Failed to add column %s.%s: %v\n", table, col.Column, err)
				}
			}
		}
	}

	// Migrate existing unencrypted API keys to encrypted format
	migrateAPIKeys(db)

	log.Println("Database migrations completed")
}

func columnExists(db *sql.DB, table, column string) bool {
	query := fmt.Sprintf("PRAGMA table_info(%s)", table)
	rows, err := db.Query(query)
	if err != nil {
		return false
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var type_ string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &type_, &notnull, &dflt, &pk); err != nil {
			continue
		}
		if name == column {
			return true
		}
	}
	return false
}

// migrateAPIKeys encrypts any existing unencrypted API keys
func migrateAPIKeys(db *sql.DB) {
	rows, err := db.Query("SELECT id, api_key FROM providers WHERE api_key IS NOT NULL AND api_key != ''")
	if err != nil {
		log.Println("Error checking API keys for migration:", err)
		return
	}
	defer rows.Close()

	type keyRow struct {
		ID     int64
		APIKey string
	}
	var keysToMigrate []keyRow

	for rows.Next() {
		var row keyRow
		if err := rows.Scan(&row.ID, &row.APIKey); err != nil {
			continue
		}
		// Check if already encrypted by trying MigrateAPIKey
		if !IsEncrypted(row.APIKey) {
			keysToMigrate = append(keysToMigrate, row)
		}
	}

	for _, row := range keysToMigrate {
		encrypted, err := Encrypt(row.APIKey)
		if err != nil {
			log.Printf("Warning: Could not encrypt API key for provider %d: %v\n", row.ID, err)
			continue
		}
		_, err = db.Exec("UPDATE providers SET api_key = ? WHERE id = ?", encrypted, row.ID)
		if err != nil {
			log.Printf("Warning: Could not update encrypted API key for provider %d: %v\n", row.ID, err)
		} else {
			log.Printf("Migrated API key for provider %d to encrypted format\n", row.ID)
		}
	}
}

// SeedFromEnvIfEmpty seeds the database with .env values if no providers exist
func SeedFromEnvIfEmpty(db *sql.DB) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM providers").Scan(&count)
	if err != nil {
		log.Println("Error checking providers:", err)
		return
	}

	if count > 0 {
		return // Already has providers
	}

	log.Println("No providers found, seeding from .env...")

	// Check if we have Ollama or OpenAI-compatible setup
	llmModel := os.Getenv("llm")
	baseURL := os.Getenv("baseUrl")
	apiKey := os.Getenv("apiKey")

	if llmModel == "" {
		llmModel = "llama3.1:8b" // Default model
	}

	var providerType, providerName string
	if baseURL != "" && apiKey != "" {
		// OpenAI-compatible provider (Groq, DeepInfra, OpenRouter, etc.)
		providerType = "openai_compatible"
		// Try to guess provider name from URL
		switch {
		case contains(baseURL, "groq.com"):
			providerName = "Groq"
		case contains(baseURL, "deepinfra.com"):
			providerName = "DeepInfra"
		case contains(baseURL, "openrouter.ai"):
			providerName = "OpenRouter"
		default:
			providerName = "OpenAI Compatible"
		}
	} else {
		// Ollama (local)
		providerType = "ollama"
		providerName = "Ollama (Local)"
		baseURL = os.Getenv("OLLAMA_HOST")
		if baseURL == "" {
			baseURL = "http://localhost:11434"
		}
	}

	// Insert the provider
	result, err := db.Exec(
		`INSERT INTO providers (name, type, base_url, api_key, is_active) VALUES (?, ?, ?, ?, 1)`,
		providerName, providerType, baseURL, apiKey,
	)
	if err != nil {
		log.Println("Error seeding provider:", err)
		return
	}

	providerID, _ := result.LastInsertId()

	// Add the model
	_, err = db.Exec(
		`INSERT INTO models (provider_id, model_name, is_default) VALUES (?, ?, 1)`,
		providerID, llmModel,
	)
	if err != nil {
		log.Println("Error seeding model:", err)
		return
	}

	// Set default theme
	_, err = db.Exec(`INSERT OR IGNORE INTO settings (key, value) VALUES ('theme', 'light')`)
	if err != nil {
		log.Println("Error seeding theme setting:", err)
	}

	log.Printf("Seeded provider '%s' with model '%s'\n", providerName, llmModel)
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
