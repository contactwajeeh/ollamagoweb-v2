package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type MCPServer struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	ServerType  string `json:"server_type"`
	EndpointURL string `json:"endpoint_url,omitempty"`
	Command     string `json:"command,omitempty"`
	Args        string `json:"args,omitempty"`
	EnvVars     string `json:"env_vars,omitempty"`
	IsEnabled   bool   `json:"is_enabled"`
}

type MCPTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

type MCPClient struct {
	mu       sync.RWMutex
	sessions map[int64]*mcpSession
}

type mcpSession struct {
	client    *http.Client
	endpoint  string
	serverID  int64
	lastError error
}

var mcpClient *MCPClient

func InitMCPClient() {
	mcpClient = &MCPClient{
		sessions: make(map[int64]*mcpSession),
	}
	log.Println("MCP client initialized")
}

func GetMCPClient() *MCPClient {
	return mcpClient
}

func (c *MCPClient) ConnectServer(ctx context.Context, server *MCPServer) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.sessions[server.ID]; ok {
		return nil
	}

	if server.ServerType != "http" {
		return fmt.Errorf("only HTTP transport is supported in this implementation")
	}

	c.sessions[server.ID] = &mcpSession{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		endpoint: server.EndpointURL,
		serverID: server.ID,
	}

	log.Printf("Connected to MCP server: %s (ID: %d)", server.Name, server.ID)
	return nil
}

func (c *MCPClient) ListTools(ctx context.Context, serverID int64) ([]MCPTool, error) {
	c.mu.RLock()
	session, ok := c.sessions[serverID]
	c.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no active session for server ID: %d", serverID)
	}

	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
		"params":  map[string]interface{}{},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", session.endpoint, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := session.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	var response map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if errorResp, ok := response["error"].(map[string]interface{}); ok {
		return nil, fmt.Errorf("MCP error: %v", errorResp)
	}

	result, ok := response["result"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid response format")
	}

	toolsArr, ok := result["tools"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("no tools in response")
	}

	tools := make([]MCPTool, 0, len(toolsArr))
	for _, t := range toolsArr {
		toolMap, ok := t.(map[string]interface{})
		if !ok {
			continue
		}

		tool := MCPTool{
			Name:        getString(toolMap, "name"),
			Description: getString(toolMap, "description"),
			InputSchema: getMap(toolMap, "inputSchema"),
		}
		tools = append(tools, tool)
	}

	return tools, nil
}

func (c *MCPClient) GetAllEnabledTools(ctx context.Context, servers []*MCPServer) ([]MCPTool, error) {
	var allTools []MCPTool

	for _, server := range servers {
		if !server.IsEnabled {
			continue
		}

		if err := c.ConnectServer(ctx, server); err != nil {
			log.Printf("Warning: failed to connect to MCP server %s: %v", server.Name, err)
			continue
		}

		tools, err := c.ListTools(ctx, server.ID)
		if err != nil {
			log.Printf("Warning: failed to list tools from %s: %v", server.Name, err)
			continue
		}

		for i := range tools {
			tools[i].Name = fmt.Sprintf("%s_%s", sanitizeName(server.Name), tools[i].Name)
		}

		allTools = append(allTools, tools...)
	}

	return allTools, nil
}

func (c *MCPClient) CallTool(ctx context.Context, serverID int64, name string, arguments map[string]interface{}) ([]byte, error) {
	c.mu.RLock()
	session, ok := c.sessions[serverID]
	c.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no active session for server ID: %d", serverID)
	}

	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      time.Now().UnixNano(),
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      name,
			"arguments": arguments,
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", session.endpoint, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := session.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	var response map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if errorResp, ok := response["error"].(map[string]interface{}); ok {
		return nil, fmt.Errorf("MCP error: %v", errorResp)
	}

	result, ok := response["result"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid response format")
	}

	contentArr, ok := result["content"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("no content in response")
	}

	var responseBuilder strings.Builder
	for _, c := range contentArr {
		contentMap, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if text, ok := contentMap["text"].(string); ok {
			responseBuilder.WriteString(text)
		}
	}

	return []byte(responseBuilder.String()), nil
}

func (c *MCPClient) DisconnectServer(serverID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.sessions[serverID]; ok {
		delete(c.sessions, serverID)
		log.Printf("Disconnected MCP server ID: %d", serverID)
	}
}

func (c *MCPClient) DisconnectAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.sessions = make(map[int64]*mcpSession)
	log.Println("Disconnected all MCP servers")
}

func sanitizeName(name string) string {
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ToLower(name)
	name = strings.TrimSpace(name)
	if len(name) > 20 {
		name = name[:20]
	}
	return name
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getMap(m map[string]interface{}, key string) map[string]interface{} {
	if v, ok := m[key].(map[string]interface{}); ok {
		return v
	}
	return make(map[string]interface{})
}
