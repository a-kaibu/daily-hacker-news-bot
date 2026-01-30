package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"time"
)

type NewsItem struct {
	ID                 int    `json:"id"`
	TitleJa            string `json:"titleJa"`
	URL                string `json:"url"`
	Score              int    `json:"score"`
	Rank               int    `json:"rank"`
	CommentSummaryHtml string `json:"commentSummaryHtml"`
}

type DiscordWebhookPayload struct {
	Embeds []DiscordEmbed `json:"embeds"`
}

type DiscordEmbed struct {
	Title  string         `json:"title"`
	URL    string         `json:"url,omitempty"`
	Color  int            `json:"color"`
	Fields []DiscordField `json:"fields"`
}

type DiscordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

func main() {
	webhookURL := os.Getenv("DISCORD_WEBHOOK_URL")
	if webhookURL == "" {
		log.Fatal("DISCORD_WEBHOOK_URL environment variable is required")
	}

	baseURL := os.Getenv("DATA_SOURCE_URL")
	if baseURL == "" {
		log.Fatal("DATA_SOURCE_URL environment variable is required")
	}

	date := time.Now().Format("2006-01-02")
	if len(os.Args) > 1 {
		date = os.Args[1]
	}
	dataURL := fmt.Sprintf("%s/%s.txt", baseURL, date)

	log.Printf("Fetching data from: %s", dataURL)

	items, err := fetchAndParseNews(dataURL)
	if err != nil {
		log.Fatalf("Failed to fetch news: %v", err)
	}

	log.Printf("Found %d news items", len(items))

	sort.Slice(items, func(i, j int) bool {
		return items[i].Score > items[j].Score
	})

	top20 := items
	if len(top20) > 20 {
		top20 = top20[:20]
	}

	if err := sendToDiscord(webhookURL, date, top20); err != nil {
		log.Fatalf("Failed to send to Discord: %v", err)
	}

	log.Println("Successfully sent news to Discord!")
}

func fetchAndParseNews(url string) ([]NewsItem, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return parseRSCData(body)
}

func parseRSCData(data []byte) ([]NewsItem, error) {
	str := string(data)
	var items []NewsItem

	searchStr := `"item":`
	idx := 0
	for {
		pos := indexOf(str[idx:], searchStr)
		if pos == -1 {
			break
		}
		pos += idx + len(searchStr)

		for pos < len(str) && (str[pos] == ' ' || str[pos] == '\t') {
			pos++
		}

		if pos >= len(str) || str[pos] != '{' {
			idx = pos
			continue
		}

		jsonStr := extractJSON(str[pos:])
		if jsonStr == "" {
			idx = pos + 1
			continue
		}

		var item NewsItem
		if err := json.Unmarshal([]byte(jsonStr), &item); err != nil {
			idx = pos + 1
			continue
		}

		if item.TitleJa != "" && item.URL != "" {
			items = append(items, item)
		}

		idx = pos + len(jsonStr)
	}

	return items, nil
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func extractJSON(s string) string {
	if len(s) == 0 || s[0] != '{' {
		return ""
	}

	depth := 0
	inString := false
	escaped := false

	for i, c := range s {
		if escaped {
			escaped = false
			continue
		}

		if c == '\\' && inString {
			escaped = true
			continue
		}

		if c == '"' {
			inString = !inString
			continue
		}

		if inString {
			continue
		}

		switch c {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[:i+1]
			}
		}
	}

	return ""
}

func sendToDiscord(webhookURL, date string, items []NewsItem) error {
	for i := 0; i < len(items); i += 3 {
		end := min(i + 3, len(items))

		batch := items[i:end]
		var fields []DiscordField

		for j, item := range batch {
			rank := i + j + 1
			name := fmt.Sprintf("%d位 | Score: %d", rank, item.Score)

			title := truncate(item.TitleJa, 200)
			value := fmt.Sprintf("[%s](%s)", title, item.URL)

			if item.CommentSummaryHtml != "" {
				summary := truncate(stripHTML(item.CommentSummaryHtml), 800)
				value = fmt.Sprintf("%s\n%s", value, summary)
			}

			fields = append(fields, DiscordField{
				Name:  name,
				Value: value,
			})
		}

		title := fmt.Sprintf("Hacker News 日本語まとめ (%s)", date)
		if i > 0 {
			title = fmt.Sprintf("Hacker News 日本語まとめ (%s) - 続き", date)
		}

		embed := DiscordEmbed{
			Title:  title,
			Color:  0xFF6600,
			Fields: fields,
		}

		payload := DiscordWebhookPayload{Embeds: []DiscordEmbed{embed}}
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}

		resp, err := http.Post(webhookURL, "application/json", bytes.NewReader(jsonData))
		if err != nil {
			return fmt.Errorf("failed to send webhook: %w", err)
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, string(body))
		}
		resp.Body.Close()

		if i+5 < len(items) {
			time.Sleep(500 * time.Millisecond)
		}
	}

	return nil
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-3]) + "..."
}

func stripHTML(s string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	result := re.ReplaceAllString(s, "")
	result = regexp.MustCompile(`\s+`).ReplaceAllString(result, " ")
	return result
}
