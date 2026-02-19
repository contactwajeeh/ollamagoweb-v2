package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/contactwajeeh/ollamagoweb-v2/mcp"
	"github.com/ollama/ollama/api"
)

type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
	ServerID    int64                  `json:"server_id,omitempty"`
}

type ToolCall struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
	ServerID  int64                  `json:"server_id,omitempty"`
}

type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error"`
}

const MaxToolIterations = 5

func GetAllEnabledMCPTools(ctx context.Context) ([]Tool, error) {
	rows, err := db.Query(`
		SELECT id, name, server_type, endpoint_url, command, args, env_vars, is_enabled
		FROM mcp_servers
		WHERE is_enabled = 1
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query MCP servers: %w", err)
	}
	defer rows.Close()

	var servers []*mcp.MCPServer
	for rows.Next() {
		var s mcp.MCPServer
		var endpointURL, command, args, envVars sql.NullString
		err := rows.Scan(&s.ID, &s.Name, &s.ServerType, &endpointURL, &command, &args, &envVars, &s.IsEnabled)
		if err != nil {
			log.Printf("Error scanning MCP server: %v", err)
			continue
		}
		s.EndpointURL = endpointURL.String
		s.Command = command.String
		s.Args = args.String
		s.EnvVars = envVars.String
		servers = append(servers, &s)
	}

	client := mcp.GetMCPClient()
	if client == nil {
		return nil, fmt.Errorf("MCP client not initialized")
	}

	mcpTools, err := client.GetAllEnabledTools(ctx, servers)
	if err != nil {
		return nil, fmt.Errorf("failed to get MCP tools: %w", err)
	}

	tools := make([]Tool, len(mcpTools))
	for i, t := range mcpTools {
		tools[i] = Tool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
			ServerID:    t.ServerID,
		}
	}

	return tools, nil
}

func ExecuteToolCall(ctx context.Context, toolCall ToolCall) (string, error) {
	client := mcp.GetMCPClient()
	if client == nil {
		return "", fmt.Errorf("MCP client not initialized")
	}

	result, err := client.CallTool(ctx, toolCall.ServerID, toolCall.Name, toolCall.Arguments)
	if err != nil {
		return "", fmt.Errorf("tool execution failed: %w", err)
	}

	return string(result), nil
}

func FormatToolsForOpenAI(tools []Tool) []map[string]interface{} {
	formatted := make([]map[string]interface{}, len(tools))
	for i, t := range tools {
		formatted[i] = map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.InputSchema,
			},
		}
	}
	return formatted
}

func FormatToolsForOllama(tools []Tool) []map[string]interface{} {
	formatted := make([]map[string]interface{}, len(tools))
	for i, t := range tools {
		formatted[i] = map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.InputSchema,
			},
		}
	}
	return formatted
}

type ToolExecutionCallback func(toolName string, status string)

func RunAgenticLoop(
	ctx context.Context,
	provider Provider,
	tools []Tool,
	history []api.Message,
	prompt string,
	systemPrompt string,
	callback ToolExecutionCallback,
) (string, error) {
	if len(tools) == 0 {
		return provider.GenerateNonStreaming(ctx, history, prompt, systemPrompt)
	}

	messages := make([]api.Message, len(history))
	copy(messages, history)

	messages = append(messages, api.Message{
		Role:    "user",
		Content: prompt,
	})

	for iteration := 0; iteration < MaxToolIterations; iteration++ {
		log.Printf("Agentic loop iteration %d", iteration+1)

		response, toolCalls, err := provider.GenerateWithTools(ctx, messages, systemPrompt, tools)
		if err != nil {
			return "", fmt.Errorf("generation failed: %w", err)
		}

		if len(toolCalls) == 0 {
			return response, nil
		}

		log.Printf("LLM requested %d tool calls", len(toolCalls))

		messages = append(messages, api.Message{
			Role:    "assistant",
			Content: response,
		})

		for _, tc := range toolCalls {
			var serverID int64
			for _, t := range tools {
				if t.Name == tc.Name {
					serverID = t.ServerID
					break
				}
			}

			tc.ServerID = serverID

			if callback != nil {
				callback(tc.Name, "calling")
			}

			result, err := ExecuteToolCall(ctx, tc)
			if callback != nil {
				if err != nil {
					callback(tc.Name, "error")
				} else {
					callback(tc.Name, "completed")
				}
			}

			toolResultContent := result
			if err != nil {
				toolResultContent = fmt.Sprintf("Error: %v", err)
			}

			resultJSON, _ := json.Marshal(map[string]interface{}{
				"tool_call_id": tc.ID,
				"name":         tc.Name,
				"result":       toolResultContent,
			})

			messages = append(messages, api.Message{
				Role:    "tool",
				Content: string(resultJSON),
			})
		}
	}

	return provider.GenerateNonStreaming(ctx, messages, "", systemPrompt)
}

func ExtractToolCallsFromResponse(response map[string]interface{}) []ToolCall {
	choices, ok := response["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return nil
	}

	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return nil
	}

	message, ok := choice["message"].(map[string]interface{})
	if !ok {
		return nil
	}

	toolCalls, ok := message["tool_calls"].([]interface{})
	if !ok {
		return nil
	}

	var calls []ToolCall
	for _, tc := range toolCalls {
		tcMap, ok := tc.(map[string]interface{})
		if !ok {
			continue
		}

		function, ok := tcMap["function"].(map[string]interface{})
		if !ok {
			continue
		}

		name, _ := function["name"].(string)
		argsStr, _ := function["arguments"].(string)
		id, _ := tcMap["id"].(string)

		var args map[string]interface{}
		if argsStr != "" {
			json.Unmarshal([]byte(argsStr), &args)
		}

		calls = append(calls, ToolCall{
			ID:        id,
			Name:      name,
			Arguments: args,
		})
	}

	return calls
}

func HasToolCalls(response string) bool {
	return strings.Contains(response, `"tool_calls"`) || strings.Contains(response, `"function"`)
}
