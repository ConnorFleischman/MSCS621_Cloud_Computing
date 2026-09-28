package newsapi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go-mongo-docker/models"
)

// Article represents one NewsAPI response item.
type Article = models.Article

// Request defines the query parameters used for a NewsAPI call.
type Request struct {
	APIKey       string
	Topic        string
	Country      string
	Days         int
	Limit        int
	Page         int
	Timeout      time.Duration
	MongoTimeout time.Duration
}

const (
	defaultAPITimeout   = 15 * time.Second
	defaultMongoTimeout = 10 * time.Second
)

func NormalizeRequestTimeouts(req Request) Request {
	if req.Timeout <= 0 {
		req.Timeout = defaultAPITimeout
	}
	if req.MongoTimeout <= 0 {
		req.MongoTimeout = defaultMongoTimeout
	}
	return req
}

func FormatTimeoutError(kind string, timeout time.Duration, err error) error {
	if err == nil {
		return fmt.Errorf("%s timed out after %s", kind, timeout)
	}
	return fmt.Errorf("%s timed out after %s: %w", kind, timeout, err)
}

// FetchArticles searches dated articles by topic, or country headlines when no topic is supplied.
func FetchArticles(req Request) ([]Article, error) {
	req = NormalizeRequestTimeouts(req)
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
	if req.Page > 0 {
		params.Set("page", strconv.Itoa(req.Page))
	}

	if strings.TrimSpace(req.Topic) != "" {
		params.Set("q", strings.TrimSpace(req.Topic))
	}

	now := time.Now().UTC()
	startDate := now.AddDate(0, 0, -(req.Days - 1))
	params.Set("from", startDate.Format("2006-01-02"))
	params.Set("to", now.Format(time.RFC3339))

	endpoint := "https://newsapi.org/v2/top-headlines?" + params.Encode()
	if strings.TrimSpace(req.Topic) != "" {
		params.Del("country")
		params.Set("sortBy", "publishedAt")
		endpoint = "https://newsapi.org/v2/everything?" + params.Encode()
	}

	ctx, cancel := context.WithTimeout(context.Background(), req.Timeout)
	defer cancel()

	client := &http.Client{Timeout: req.Timeout}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build request failed: %w", err)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return nil, FormatTimeoutError("NewsAPI", req.Timeout, err)
		}
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var apiErr models.NewsAPIResponse
		if err := json.NewDecoder(resp.Body).Decode(&apiErr); err == nil && apiErr.Message != "" {
			return nil, fmt.Errorf("NewsAPI request failed: %s (%s)", resp.Status, apiErr.Message)
		}
		return nil, fmt.Errorf("NewsAPI request failed: %s", resp.Status)
	}

	var payload models.NewsAPIResponse
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
