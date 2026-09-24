package newsapi

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Article represents one NewsAPI response item.
type Article struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Source      struct {
		Name string `json:"name"`
	} `json:"source"`
	PublishedAt string `json:"publishedAt"`
}

// Request defines the query parameters used for a NewsAPI call.
type Request struct {
	APIKey string
	Topic  string
	Country string
	Days   int
	Limit  int
}

// FetchArticles calls the NewsAPI top-headlines endpoint using the supplied request settings.
func FetchArticles(req Request) ([]Article, error) {
	if strings.TrimSpace(req.APIKey) == "" {
		return nil, fmt.Errorf("NEWSAPI_API_KEY is not set")
	}
	if req.Days < 1 {
		req.Days = 1
	}
	if req.Limit < 1 {
		req.Limit = 1
	}
	if req.Country == "" {
		req.Country = "us"
	}

	params := url.Values{}
	params.Set("apiKey", req.APIKey)
	params.Set("country", req.Country)
	params.Set("pageSize", strconv.Itoa(req.Limit))
	if strings.TrimSpace(req.Topic) != "" {
		params.Set("q", strings.TrimSpace(req.Topic))
	}

	now := time.Now().UTC()
	startDate := now.AddDate(0, 0, -(req.Days - 1))
	params.Set("from", startDate.Format("2006-01-02"))
	params.Set("to", now.Format("2006-01-02"))

	endpoint := "https://newsapi.org/v2/top-headlines?" + params.Encode()
	resp, err := http.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var apiErr struct {
			Message string `json:"message"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&apiErr); err == nil && apiErr.Message != "" {
			return nil, fmt.Errorf("NewsAPI request failed: %s (%s)", resp.Status, apiErr.Message)
		}
		return nil, fmt.Errorf("NewsAPI request failed: %s", resp.Status)
	}

	var payload struct {
		Status       string    `json:"status"`
		TotalResults int       `json:"totalResults"`
		Articles     []Article `json:"articles"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode response failed: %w", err)
	}
	if payload.Status != "ok" {
		return nil, fmt.Errorf("unexpected API status: %s", payload.Status)
	}
	return payload.Articles, nil
}

// SaveArticles writes the returned articles into a folder as JSON documents.
func SaveArticles(dir string, articles []Article) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	for i, article := range articles {
		data, err := json.MarshalIndent(article, "", "  ")
		if err != nil {
			return err
		}
		fileName := fmt.Sprintf("article_%02d.json", i+1)
		if err := os.WriteFile(filepath.Join(dir, fileName), data, 0o644); err != nil {
			return err
		}
	}

	allData, err := json.MarshalIndent(articles, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "articles.json"), allData, 0o644); err != nil {
		return err
	}
	return nil
}

// LoadEnvFile loads a .env-style file into environment variables.
func LoadEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), "\"'")
		if _, exists := os.LookupEnv(key); !exists {
			if err := os.Setenv(key, value); err != nil {
				return err
			}
		}
	}

	return scanner.Err()
}
