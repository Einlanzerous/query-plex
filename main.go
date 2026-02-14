package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var (
	plexURL   string
	plexToken string
	client    = &http.Client{Timeout: 15 * time.Second}
)

func main() {
	plexURL = strings.TrimRight(os.Getenv("PLEX_URL"), "/")
	plexToken = os.Getenv("PLEX_TOKEN")

	if plexURL == "" || plexToken == "" {
		log.Fatal("PLEX_URL and PLEX_TOKEN environment variables are required")
	}

	if err := pingPlex(); err != nil {
		log.Fatalf("Plex connectivity check failed: %v", err)
	}
	log.Println("Plex connection verified successfully")

	s := server.NewMCPServer(
		"plex-mcp-go",
		"1.0.0",
		server.WithToolCapabilities(false),
	)

	searchTool := mcp.NewTool("search_media",
		mcp.WithDescription("Search the Plex library for movies or TV shows using fuzzy matching"),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Search query (e.g. 'batman', 'breaking bad')"),
		),
		mcp.WithString("media_type",
			mcp.Description("Filter by media type"),
			mcp.Enum("movie", "show"),
		),
	)

	onDeckTool := mcp.NewTool("get_on_deck",
		mcp.WithDescription("Get the 'On Deck' / 'Continue Watching' list for the current user"),
	)

	pickMovieTool := mcp.NewTool("pick_something",
		mcp.WithDescription("Pick a random movie from the library"),
	)

	newSeriesTool := mcp.NewTool("start_a_new_series",
		mcp.WithDescription("Pick a random TV show to start watching"),
	)

	watchAnimeTool := mcp.NewTool("watch_anime",
		mcp.WithDescription("Pick a random anime series to start watching"),
	)

	s.AddTool(searchTool, handleSearchMedia)
	s.AddTool(onDeckTool, handleGetOnDeck)
	s.AddTool(pickMovieTool, handlePickRandom("Movies"))
	s.AddTool(newSeriesTool, handlePickRandom("TV Shows"))
	s.AddTool(watchAnimeTool, handlePickRandom("Anime"))

	log.Println("Starting plex-mcp-go server on stdio...")
	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

// pingPlex verifies Plex connectivity and token validity.
func pingPlex() error {
	resp, err := plexRequest("/identity")
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("invalid PLEX_TOKEN (401 Unauthorized)")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d from Plex", resp.StatusCode)
	}
	return nil
}

// plexRequest makes an authenticated GET request to the Plex API.
func plexRequest(path string) (*http.Response, error) {
	u := plexURL + path
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Plex-Token", plexToken)
	req.Header.Set("Accept", "application/json")
	return client.Do(req)
}

// handleSearchMedia implements the search_media tool.
func handleSearchMedia(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := req.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError("missing required parameter: query"), nil
	}

	mediaType := req.GetString("media_type", "")

	endpoint := fmt.Sprintf("/hubs/search?query=%s&limit=10", url.QueryEscape(query))

	resp, err := plexRequest(endpoint)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Plex API request failed: %v", err)), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return mcp.NewToolResultError(fmt.Sprintf("Plex API returned status %d", resp.StatusCode)), nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to read response: %v", err)), nil
	}

	var searchResp hubSearchResponse
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to parse response: %v", err)), nil
	}

	var results []mediaItem
	for _, hub := range searchResp.MediaContainer.Hub {
		hubType := strings.ToLower(hub.Type)

		if mediaType != "" && hubType != mediaType {
			continue
		}
		if hubType != "movie" && hubType != "show" {
			continue
		}

		for _, m := range hub.Metadata {
			results = append(results, mediaItem{
				Title: m.Title,
				Year:  m.Year,
				Type:  hubType,
			})
		}
	}

	if len(results) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("No results found for '%s'.", query)), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Search results for '%s':\n\n", query))
	for i, item := range results {
		yearStr := "N/A"
		if item.Year > 0 {
			yearStr = fmt.Sprintf("%d", item.Year)
		}
		sb.WriteString(fmt.Sprintf("%d. %s (%s) [%s] - Available\n", i+1, item.Title, yearStr, item.Type))
	}

	return mcp.NewToolResultText(sb.String()), nil
}

// handleGetOnDeck implements the get_on_deck tool.
func handleGetOnDeck(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resp, err := plexRequest("/library/onDeck")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Plex API request failed: %v", err)), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return mcp.NewToolResultError(fmt.Sprintf("Plex API returned status %d", resp.StatusCode)), nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to read response: %v", err)), nil
	}

	var onDeckResp onDeckResponse
	if err := json.Unmarshal(body, &onDeckResp); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to parse response: %v", err)), nil
	}

	items := onDeckResp.MediaContainer.Metadata
	if len(items) == 0 {
		return mcp.NewToolResultText("Your On Deck list is empty."), nil
	}

	var sb strings.Builder
	sb.WriteString("On Deck / Continue Watching:\n\n")
	for i, item := range items {
		line := fmt.Sprintf("%d. %s", i+1, item.Title)
		if item.GrandparentTitle != "" {
			line = fmt.Sprintf("%d. %s - %s", i+1, item.GrandparentTitle, item.Title)
			if item.ParentIndex > 0 && item.Index > 0 {
				line += fmt.Sprintf(" (S%02dE%02d)", item.ParentIndex, item.Index)
			}
		}
		if item.Year > 0 {
			line += fmt.Sprintf(" (%d)", item.Year)
		}
		if item.ViewOffset > 0 {
			progress := "in progress"
			if item.Duration > 0 {
				pct := float64(item.ViewOffset) / float64(item.Duration) * 100
				progress = fmt.Sprintf("%.0f%% watched", pct)
			}
			line += fmt.Sprintf(" [%s]", progress)
		}
		sb.WriteString(line + "\n")
	}

	return mcp.NewToolResultText(sb.String()), nil
}

// findSectionKey looks up a library section key by its title.
func findSectionKey(title string) (string, error) {
	resp, err := plexRequest("/library/sections")
	if err != nil {
		return "", fmt.Errorf("failed to fetch library sections: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Plex API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var sectionsResp sectionsResponse
	if err := json.Unmarshal(body, &sectionsResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	for _, dir := range sectionsResp.MediaContainer.Directory {
		if strings.EqualFold(dir.Title, title) {
			return dir.Key, nil
		}
	}
	return "", fmt.Errorf("library section %q not found", title)
}

// handlePickRandom returns a handler that picks a random item from the named library section.
func handlePickRandom(sectionTitle string) server.ToolHandlerFunc {
	return func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := findSectionKey(sectionTitle)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		endpoint := fmt.Sprintf("/library/sections/%s/all?sort=random&limit=1", key)
		resp, err := plexRequest(endpoint)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Plex API request failed: %v", err)), nil
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return mcp.NewToolResultError(fmt.Sprintf("Plex API returned status %d", resp.StatusCode)), nil
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to read response: %v", err)), nil
		}

		var randomResp randomPickResponse
		if err := json.Unmarshal(body, &randomResp); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to parse response: %v", err)), nil
		}

		items := randomResp.MediaContainer.Metadata
		if len(items) == 0 {
			return mcp.NewToolResultText(fmt.Sprintf("No items found in the %s library.", sectionTitle)), nil
		}

		item := items[0]
		yearStr := ""
		if item.Year > 0 {
			yearStr = fmt.Sprintf(" (%d)", item.Year)
		}
		rating := ""
		if item.Rating > 0 {
			rating = fmt.Sprintf(" - %.1f/10", item.Rating)
		}
		summary := ""
		if item.Summary != "" {
			summary = fmt.Sprintf("\n\n%s", item.Summary)
		}

		result := fmt.Sprintf("Random pick from %s:\n\n%s%s%s%s",
			sectionTitle, item.Title, yearStr, rating, summary)

		return mcp.NewToolResultText(result), nil
	}
}

// --- Plex API response types ---

type hubSearchResponse struct {
	MediaContainer struct {
		Hub []struct {
			Type     string `json:"type"`
			Metadata []struct {
				Title string `json:"title"`
				Year  int    `json:"year"`
			} `json:"Metadata"`
		} `json:"Hub"`
	} `json:"MediaContainer"`
}

type onDeckResponse struct {
	MediaContainer struct {
		Metadata []struct {
			Title            string `json:"title"`
			GrandparentTitle string `json:"grandparentTitle"`
			Year             int    `json:"year"`
			ParentIndex      int    `json:"parentIndex"`
			Index            int    `json:"index"`
			ViewOffset       int64  `json:"viewOffset"`
			Duration         int64  `json:"duration"`
		} `json:"Metadata"`
	} `json:"MediaContainer"`
}

type sectionsResponse struct {
	MediaContainer struct {
		Directory []struct {
			Key   string `json:"key"`
			Type  string `json:"type"`
			Title string `json:"title"`
		} `json:"Directory"`
	} `json:"MediaContainer"`
}

type randomPickResponse struct {
	MediaContainer struct {
		Metadata []struct {
			Title   string  `json:"title"`
			Year    int     `json:"year"`
			Rating  float64 `json:"rating"`
			Summary string  `json:"summary"`
		} `json:"Metadata"`
	} `json:"MediaContainer"`
}

type mediaItem struct {
	Title string
	Year  int
	Type  string
}
