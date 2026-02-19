package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/ollama/ollama/api"
)

const (
	OpenSkillsRepo   = "besoeasy/open-skills"
	OpenSkillsBranch = "main"
	SkillsCacheTTL   = 1 * time.Hour
)

type OpenSkill struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Content     string    `json:"content"`
	URL         string    `json:"url"`
	FetchedAt   time.Time `json:"fetched_at"`
}

var skillNameRegex = regexp.MustCompile(`(?m)^name:\s*(.+)$`)
var skillDescRegex = regexp.MustCompile(`(?m)^description:\s*"?(.+?)"?\s*$`)

func FetchSkillsFromGitHub(ctx context.Context) ([]OpenSkill, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/contents/skills?ref=%s", OpenSkillsRepo, OpenSkillsBranch)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch skills list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, string(body))
	}

	var dirs []struct {
		Name string `json:"name"`
		Type string `json:"type"`
		URL  string `json:"url"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&dirs); err != nil {
		return nil, fmt.Errorf("failed to parse GitHub response: %w", err)
	}

	var skills []OpenSkill
	for _, dir := range dirs {
		if dir.Type != "dir" {
			continue
		}

		skillURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/skills/%s/SKILL.md", OpenSkillsRepo, OpenSkillsBranch, dir.Name)

		skillReq, err := http.NewRequestWithContext(ctx, "GET", skillURL, nil)
		if err != nil {
			log.Printf("Error creating request for skill %s: %v", dir.Name, err)
			continue
		}

		skillResp, err := client.Do(skillReq)
		if err != nil {
			log.Printf("Error fetching skill %s: %v", dir.Name, err)
			continue
		}

		if skillResp.StatusCode != http.StatusOK {
			skillResp.Body.Close()
			continue
		}

		content, err := io.ReadAll(skillResp.Body)
		skillResp.Body.Close()
		if err != nil {
			log.Printf("Error reading skill %s: %v", dir.Name, err)
			continue
		}

		contentStr := string(content)
		name := dir.Name
		description := ""

		if match := skillNameRegex.FindStringSubmatch(contentStr); len(match) > 1 {
			name = strings.TrimSpace(match[1])
		}

		if match := skillDescRegex.FindStringSubmatch(contentStr); len(match) > 1 {
			description = strings.TrimSpace(match[1])
		}

		if description == "" {
			description = fmt.Sprintf("Open Skill: %s", name)
		}

		skills = append(skills, OpenSkill{
			Name:        name,
			Description: description,
			Content:     contentStr,
			URL:         skillURL,
			FetchedAt:   time.Now(),
		})
	}

	return skills, nil
}

func GetCachedSkills(ctx context.Context) ([]OpenSkill, error) {
	rows, err := db.Query(`
		SELECT name, description, content, url, fetched_at
		FROM open_skills_cache
		WHERE fetched_at > ?
	`, time.Now().Add(-SkillsCacheTTL))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var skills []OpenSkill
	for rows.Next() {
		var s OpenSkill
		if err := rows.Scan(&s.Name, &s.Description, &s.Content, &s.URL, &s.FetchedAt); err != nil {
			continue
		}
		skills = append(skills, s)
	}

	if len(skills) > 0 {
		return skills, nil
	}

	return RefreshSkillsCache(ctx)
}

func RefreshSkillsCache(ctx context.Context) ([]OpenSkill, error) {
	skills, err := FetchSkillsFromGitHub(ctx)
	if err != nil {
		return nil, err
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	_, err = tx.Exec("DELETE FROM open_skills_cache")
	if err != nil {
		return nil, err
	}

	stmt, err := tx.Prepare(`
		INSERT INTO open_skills_cache (name, description, content, url, fetched_at)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	for _, s := range skills {
		_, err = stmt.Exec(s.Name, s.Description, s.Content, s.URL, s.FetchedAt)
		if err != nil {
			log.Printf("Error caching skill %s: %v", s.Name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	log.Printf("Cached %d Open Skills", len(skills))
	return skills, nil
}

func ConvertSkillsToTools(skills []OpenSkill) []Tool {
	tools := make([]Tool, len(skills))
	for i, s := range skills {
		tools[i] = Tool{
			Name:        fmt.Sprintf("skill_%s", sanitizeSkillName(s.Name)),
			Description: s.Description,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "The task or question to execute using this skill",
					},
				},
				"required": []string{"query"},
			},
			ServerID: -1,
		}
	}
	return tools
}

func sanitizeSkillName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ReplaceAll(name, " ", "_")
	name = regexp.MustCompile(`[^a-z0-9_]`).ReplaceAllString(name, "")
	if len(name) > 30 {
		name = name[:30]
	}
	return name
}

func ExecuteSkill(ctx context.Context, skillName string, query string) (string, error) {
	skills, err := GetCachedSkills(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get skills: %w", err)
	}

	var targetSkill *OpenSkill
	for _, s := range skills {
		if sanitizeSkillName(s.Name) == skillName || s.Name == skillName {
			targetSkill = &s
			break
		}
	}

	if targetSkill == nil {
		return "", fmt.Errorf("skill not found: %s", skillName)
	}

	return fmt.Sprintf("Skill: %s\n\nDescription: %s\n\nDocumentation:\n%s\n\nUser Query: %s\n\nPlease use the skill documentation above to help the user with their query.",
		targetSkill.Name, targetSkill.Description, targetSkill.Content, query), nil
}

func GetSkillDescriptions(ctx context.Context) (map[string]string, error) {
	skills, err := GetCachedSkills(ctx)
	if err != nil {
		return nil, err
	}

	desc := make(map[string]string)
	for _, s := range skills {
		desc[sanitizeSkillName(s.Name)] = s.Description
	}
	return desc, nil
}

func RunAgenticLoopWithSkills(
	ctx context.Context,
	provider Provider,
	mcpTools []Tool,
	skills []OpenSkill,
	history []api.Message,
	prompt string,
	systemPrompt string,
	callback ToolExecutionCallback,
) (string, error) {
	skillTools := ConvertSkillsToTools(skills)
	allTools := append(mcpTools, skillTools...)

	if len(allTools) == 0 {
		return provider.GenerateNonStreaming(ctx, history, prompt, systemPrompt)
	}

	messages := make([]api.Message, len(history))
	copy(messages, history)

	messages = append(messages, api.Message{
		Role:    "user",
		Content: prompt,
	})

	for iteration := 0; iteration < MaxToolIterations; iteration++ {
		log.Printf("Agentic loop iteration %d with %d tools", iteration+1, len(allTools))

		response, toolCalls, err := provider.GenerateWithTools(ctx, messages, systemPrompt, allTools)
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
			if callback != nil {
				callback(tc.Name, "calling")
			}

			var result string
			var execErr error

			if strings.HasPrefix(tc.Name, "skill_") {
				skillName := strings.TrimPrefix(tc.Name, "skill_")
				query, _ := tc.Arguments["query"].(string)
				result, execErr = ExecuteSkill(ctx, skillName, query)
				tc.ServerID = -1
			} else {
				for _, t := range allTools {
					if t.Name == tc.Name {
						tc.ServerID = t.ServerID
						break
					}
				}
				result, execErr = ExecuteToolCall(ctx, tc)
			}

			if callback != nil {
				if execErr != nil {
					callback(tc.Name, "error")
				} else {
					callback(tc.Name, "completed")
				}
			}

			if execErr != nil {
				result = fmt.Sprintf("Error: %v", execErr)
			}

			resultJSON, _ := json.Marshal(map[string]interface{}{
				"tool_call_id": tc.ID,
				"name":         tc.Name,
				"result":       result,
			})

			messages = append(messages, api.Message{
				Role:    "tool",
				Content: string(resultJSON),
			})
		}
	}

	return provider.GenerateNonStreaming(ctx, messages, "", systemPrompt)
}
