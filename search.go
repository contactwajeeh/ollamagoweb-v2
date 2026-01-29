package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// BraveSearchResponse represents the JSON response from Brave Search API
type BraveSearchResponse struct {
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			Url         string `json:"url"`
		} `json:"results"`
	} `json:"web"`
}

// MaybeSearch checks if the query triggers a search and returns an enriched prompt
func MaybeSearch(query string, apiKey string) (string, error) {
	// Check for /search prefix
	if !strings.HasPrefix(strings.TrimSpace(query), "/search ") {
		return query, nil
	}

	if apiKey == "" {
		return "", fmt.Errorf("Brave API key is not configured")
	}

	// Extract search term
	searchTerm := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(query), "/search "))
	if searchTerm == "" {
		return query, nil
	}

	// Perform search
	results, err := performBraveSearch(searchTerm, apiKey)
	if err != nil {
		return "", fmt.Errorf("search failed: %w", err)
	}

	if len(results) == 0 {
		return "", fmt.Errorf("no search results found for '%s'", searchTerm)
	}

	// Build enriched prompt
	var initialContext strings.Builder
	initialContext.WriteString("Context from search results:\n")
	for i, res := range results {
		// Limit to top 3 results
		if i >= 3 {
			break
		}
		initialContext.WriteString(fmt.Sprintf("- %s: %s (%s)\n", res.Title, res.Description, res.Url))
	}
	initialContext.WriteString("\nUser Query: " + searchTerm)

	return initialContext.String(), nil
}

func performBraveSearch(query string, apiKey string) ([]struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Url         string `json:"url"`
}, error) {
	endpoint := "https://api.search.brave.com/res/v1/web/search"
	
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Add("q", query)
	q.Add("count", "3")
	req.URL.RawQuery = q.Encode()

	req.Header.Add("X-Subscription-Token", apiKey)
	req.Header.Add("Accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Brave API returned status %d", resp.StatusCode)
	}

	var braveResp BraveSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&braveResp); err != nil {
		return nil, err
	}

	return braveResp.Web.Results, nil
}
