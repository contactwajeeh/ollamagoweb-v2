package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/ollama/ollama/api"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

var (
	llmCache   = make(map[string]*openai.LLM)
	llmCacheMu sync.RWMutex
)

// Provider interface defines the contract for LLM providers
type Provider interface {
	Generate(ctx context.Context, history []api.Message, prompt string, systemPrompt string, w http.ResponseWriter) error
	FetchModels(ctx context.Context) ([]ModelInfo, error)
}

// ModelInfo represents a model returned from the API
type ModelInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	OwnedBy string `json:"owned_by,omitempty"`
}

// ProviderConfig holds the configuration for a provider
type ProviderConfig struct {
	ID       int64
	Name     string
	Type     string
	BaseURL  string
	APIKey   string
	IsActive bool
	Model    string // Currently selected model
}

// OllamaProvider handles Ollama API calls
type OllamaProvider struct {
	client *api.Client
	model  string
}

// OpenAIProvider handles OpenAI-compatible API calls (Groq, DeepInfra, OpenRouter, etc.)
type OpenAIProvider struct {
	baseURL string
	apiKey  string
	model   string
}

// NewOllamaProvider creates a new Ollama provider
func NewOllamaProvider(model string) (*OllamaProvider, error) {
	client, err := api.ClientFromEnvironment()
	if err != nil {
		return nil, fmt.Errorf("failed to create Ollama client: %w", err)
	}
	return &OllamaProvider{
		client: client,
		model:  model,
	}, nil
}

// NewOpenAIProvider creates a new OpenAI-compatible provider
func NewOpenAIProvider(baseURL, apiKey, model string) *OpenAIProvider {
	return &OpenAIProvider{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
	}
}

func getCachedLLM(baseURL, apiKey, model string) (*openai.LLM, error) {
	cacheKey := baseURL + "|" + apiKey + "|" + model

	llmCacheMu.RLock()
	if llm, ok := llmCache[cacheKey]; ok {
		llmCacheMu.RUnlock()
		return llm, nil
	}
	llmCacheMu.RUnlock()

	llmCacheMu.Lock()
	defer llmCacheMu.Unlock()

	if llm, ok := llmCache[cacheKey]; ok {
		return llm, nil
	}

	llm, err := openai.New(
		openai.WithModel(model),
		openai.WithBaseURL(baseURL),
		openai.WithToken(apiKey),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAI client: %w", err)
	}

	llmCache[cacheKey] = llm
	return llm, nil
}

// Generate streams a response from Ollama
func (p *OllamaProvider) Generate(ctx context.Context, history []api.Message, prompt string, systemPrompt string, w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	f, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported")
	}

	messages := append([]api.Message{}, history...)
	messages = append(messages, api.Message{
		Role:    "user",
		Content: prompt,
	})

	if systemPrompt != "" {
		// Prepend system prompt if not present (Ollama usually handles this via Modelfile, but explicit is good)
		// Or simpler: use the System field in ChatRequest if we want to force it
	}

	req := &api.ChatRequest{
		Model:    p.model,
		Messages: messages,
	}

	// Add system prompt to request if valid
	if systemPrompt != "" {
		// For Chat API, systems instructions are usually first message with role "system"
		// If history already has it, we might duplicate.
		// Safe bet: Prepend if it's not the first message
		if len(messages) > 0 && messages[0].Role != "system" {
			req.Messages = append([]api.Message{{Role: "system", Content: systemPrompt}}, req.Messages...)
		}
	}

	var finalMetrics api.Metrics
	var evalCount int

	respFunc := func(resp api.ChatResponse) error {
		w.Write([]byte(resp.Message.Content))
		f.Flush()
		if resp.Done {
			finalMetrics = resp.Metrics
			evalCount = resp.Metrics.EvalCount
			speed := float64(resp.Metrics.EvalCount) / resp.Metrics.EvalDuration.Seconds()
			log.Printf("Ollama metrics - Speed: %.2f tokens/s\n", speed)
		}
		return nil
	}

	err := p.client.Chat(ctx, req, respFunc)
	if err != nil {
		return err
	}

	// Send analytics at the end as a special JSON block (same format as OpenAI)
	analyticsData := map[string]interface{}{
		"model": p.model,
	}

	if evalCount > 0 {
		speed := float64(finalMetrics.EvalCount) / finalMetrics.EvalDuration.Seconds()
		analyticsData["usage"] = map[string]interface{}{
			"prompt_tokens":     finalMetrics.PromptEvalCount,
			"completion_tokens": finalMetrics.EvalCount,
			"total_tokens":      finalMetrics.PromptEvalCount + finalMetrics.EvalCount,
		}
		analyticsData["speed"] = fmt.Sprintf("%.1f tokens/s", speed)
	}

	analyticsJSON, _ := json.Marshal(analyticsData)
	w.Write([]byte("\n\n__ANALYTICS__" + string(analyticsJSON)))
	f.Flush()

	return nil
}

// FetchModels gets available models from Ollama
func (p *OllamaProvider) FetchModels(ctx context.Context) ([]ModelInfo, error) {
	list, err := p.client.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list Ollama models: %w", err)
	}

	models := make([]ModelInfo, 0, len(list.Models))
	for _, m := range list.Models {
		models = append(models, ModelInfo{
			ID:   m.Name,
			Name: m.Name,
		})
	}
	return models, nil
}

// UsageStats holds token usage information
type UsageStats struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// GenerateResponse wraps the response with analytics
type GenerateResponse struct {
	Content string      `json:"content"`
	Model   string      `json:"model"`
	Usage   *UsageStats `json:"usage,omitempty"`
}

// Generate gets a response from OpenAI-compatible API
func (p *OpenAIProvider) Generate(ctx context.Context, history []api.Message, prompt string, systemPrompt string, w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	f, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported")
	}

	llm, err := getCachedLLM(p.baseURL, p.apiKey, p.model)
	if err != nil {
		return err
	}

	// Build messages array
	messages := []llms.MessageContent{}

	// Add system prompt if provided
	if systemPrompt != "" {
		messages = append(messages, llms.MessageContent{
			Role: llms.ChatMessageTypeSystem,
			Parts: []llms.ContentPart{
				llms.TextContent{Text: systemPrompt},
			},
		})
	}

	// Add history
	for _, msg := range history {
		role := llms.ChatMessageTypeHuman
		if msg.Role == "assistant" {
			role = llms.ChatMessageTypeAI
		} else if msg.Role == "system" {
			role = llms.ChatMessageTypeSystem
		}
		messages = append(messages, llms.MessageContent{
			Role: role,
			Parts: []llms.ContentPart{
				llms.TextContent{Text: msg.Content},
			},
		})
	}

	// Add user message
	messages = append(messages, llms.MessageContent{
		Role: llms.ChatMessageTypeHuman,
		Parts: []llms.ContentPart{
			llms.TextContent{Text: prompt},
		},
	})

	opts := []llms.CallOption{
		llms.WithMaxTokens(4096),
		llms.WithTemperature(0.7),
		llms.WithTopP(0.9),
	}

	// Use streaming if available
	resp, err := llm.GenerateContent(ctx, messages, opts...)
	if err != nil {
		return fmt.Errorf("failed to generate content: %w", err)
	}

	for _, c := range resp.Choices {
		w.Write([]byte(c.Content))
		f.Flush()
	}

	// Send analytics at the end as a special JSON block
	// Format: \n\n__ANALYTICS__{"model":"...", "usage":{...}}
	analyticsData := map[string]interface{}{
		"model": p.model,
	}

	// Try to extract token usage from GenerationInfo if available
	if len(resp.Choices) > 0 && resp.Choices[0].GenerationInfo != nil {
		genInfo := resp.Choices[0].GenerationInfo
		usage := make(map[string]interface{})

		// Try various key formats that different providers might use
		if pt, ok := genInfo["PromptTokens"]; ok {
			if v, ok := pt.(int); ok {
				usage["prompt_tokens"] = v
			}
		}
		if ct, ok := genInfo["CompletionTokens"]; ok {
			if v, ok := ct.(int); ok {
				usage["completion_tokens"] = v
			}
		}
		if tt, ok := genInfo["TotalTokens"]; ok {
			if v, ok := tt.(int); ok {
				usage["total_tokens"] = v
			}
		}

		// Alternative key names
		if pt, ok := genInfo["prompt_tokens"]; ok {
			usage["prompt_tokens"] = pt
		}
		if ct, ok := genInfo["completion_tokens"]; ok {
			usage["completion_tokens"] = ct
		}
		if tt, ok := genInfo["total_tokens"]; ok {
			usage["total_tokens"] = tt
		}

		if len(usage) > 0 {
			analyticsData["usage"] = usage
		}
	}

	analyticsJSON, _ := json.Marshal(analyticsData)
	w.Write([]byte("\n\n__ANALYTICS__" + string(analyticsJSON)))
	f.Flush()

	log.Printf("OpenAI response - Model: %s\n", p.model)

	return nil
}

// FetchModels gets available models from OpenAI-compatible API
func (p *OpenAIProvider) FetchModels(ctx context.Context) ([]ModelInfo, error) {
	url := strings.TrimSuffix(p.baseURL, "/") + "/models"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	models := make([]ModelInfo, 0, len(result.Data))
	for _, m := range result.Data {
		models = append(models, ModelInfo{
			ID:      m.ID,
			Name:    m.ID,
			OwnedBy: m.OwnedBy,
		})
	}

	return models, nil
}

// GetActiveProvider retrieves the currently active provider from the database
func GetActiveProvider(db *sql.DB) (Provider, *ProviderConfig, error) {
	var config ProviderConfig

	// Get active provider
	err := db.QueryRow(`
		SELECT p.id, p.name, p.type, COALESCE(p.base_url, ''), COALESCE(p.api_key, '')
		FROM providers p
		WHERE p.is_active = 1
		LIMIT 1
	`).Scan(&config.ID, &config.Name, &config.Type, &config.BaseURL, &config.APIKey)

	if err == sql.ErrNoRows {
		return nil, nil, fmt.Errorf("no active provider configured")
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get active provider: %w", err)
	}

	// Decrypt the API key
	if config.APIKey != "" {
		decryptedKey, err := Decrypt(config.APIKey)
		if err != nil {
			log.Println("Warning: Could not decrypt API key, using as-is:", err)
		} else {
			config.APIKey = decryptedKey
		}
	}

	// Get default model for this provider
	err = db.QueryRow(`
		SELECT model_name FROM models
		WHERE provider_id = ? AND is_default = 1
		LIMIT 1
	`, config.ID).Scan(&config.Model)

	if err == sql.ErrNoRows {
		// Try to get any model
		err = db.QueryRow(`
			SELECT model_name FROM models
			WHERE provider_id = ?
			LIMIT 1
		`, config.ID).Scan(&config.Model)
	}

	if err != nil {
		return nil, nil, fmt.Errorf("no model configured for provider: %w", err)
	}

	config.IsActive = true

	// Create the appropriate provider
	var provider Provider
	switch config.Type {
	case "ollama":
		p, err := NewOllamaProvider(config.Model)
		if err != nil {
			return nil, nil, err
		}
		provider = p
	case "openai_compatible":
		provider = NewOpenAIProvider(config.BaseURL, config.APIKey, config.Model)
	default:
		return nil, nil, fmt.Errorf("unknown provider type: %s", config.Type)
	}

	return provider, &config, nil
}
