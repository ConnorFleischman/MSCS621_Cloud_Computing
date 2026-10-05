package newsapi_test

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-mongo-docker/newsapi"
)

func TestLoadEnvFileReadsValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("NEWSAPI_API_KEY=test-key\nTEST_CONFIG_VALUE=test-db\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	if err := os.Unsetenv("NEWSAPI_API_KEY"); err != nil {
		t.Fatalf("unset NEWSAPI_API_KEY: %v", err)
	}
	if err := os.Unsetenv("TEST_CONFIG_VALUE"); err != nil {
		t.Fatalf("unset TEST_CONFIG_VALUE: %v", err)
	}

	if err := newsapi.LoadEnvFile(path); err != nil {
		t.Fatalf("LoadEnvFile returned error: %v", err)
	}
	if got := os.Getenv("NEWSAPI_API_KEY"); got != "test-key" {
		t.Fatalf("NEWSAPI_API_KEY = %q, want %q", got, "test-key")
	}
	if got := os.Getenv("TEST_CONFIG_VALUE"); got != "test-db" {
		t.Fatalf("TEST_CONFIG_VALUE = %q, want %q", got, "test-db")
	}
}

func TestFetchArticlesRequiresAPIKey(t *testing.T) {
	_, err := newsapi.FetchArticles(newsapi.Request{Topic: "cloud computing", Days: 1, Limit: 1})
	if err == nil {
		t.Fatal("expected missing API key error")
	}
	if !strings.Contains(err.Error(), "NEWSAPI_API_KEY is not set") {
		t.Fatalf("expected API key error, got %v", err)
	}
}

func TestFetchArticlesParsesResponseAndOptionalFields(t *testing.T) {
	body := `{"status":"ok","totalResults":2,"articles":[{"title":"complete","author":"Writer","description":"Summary","url":"https://example.com/complete","source":{"name":"Example News"},"publishedAt":"2026-10-01T10:00:00Z"},{"title":"sparse"}]}`
	client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    r,
		}, nil
	})}

	articles, err := newsapi.FetchArticles(newsapi.Request{
		APIKey: "test-key", Topic: "cloud computing", Days: 1, Limit: 2, HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("FetchArticles returned error: %v", err)
	}
	if len(articles) != 2 {
		t.Fatalf("got %d articles, want 2", len(articles))
	}
	if articles[0].Title != "complete" || articles[0].Author != "Writer" || articles[0].Source.Name != "Example News" || articles[0].PublishedAt == nil {
		t.Fatalf("first article was not decoded: %#v", articles[0])
	}
	if articles[1].Title != "sparse" || articles[1].PublishedAt != nil {
		t.Fatalf("optional fields were not handled: %#v", articles[1])
	}
}

func TestFetchArticlesReportsAPIAndDecodeErrors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantError  string
	}{
		{name: "API error response", statusCode: http.StatusUnauthorized, body: `{"status":"error","message":"Invalid API key"}`, wantError: "Invalid API key"},
		{name: "HTTP error without API message", statusCode: http.StatusInternalServerError, body: "not json", wantError: "NewsAPI request failed: 500 Internal Server Error"},
		{name: "malformed JSON", statusCode: http.StatusOK, body: "{", wantError: "decode response failed"},
		{name: "unexpected API status", statusCode: http.StatusOK, body: `{"status":"error","articles":[]}`, wantError: "unexpected API status: error"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: test.statusCode,
					Status:     fmt.Sprintf("%d %s", test.statusCode, http.StatusText(test.statusCode)),
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(test.body)),
					Request:    r,
				}, nil
			})}
			_, err := newsapi.FetchArticles(newsapi.Request{
				APIKey: "test-key", Topic: "cloud computing", Days: 1, Limit: 1, HTTPClient: client,
			})
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("error = %v, want it to contain %q", err, test.wantError)
			}
		})
	}
}

func TestSaveArticlesWritesJSONFiles(t *testing.T) {
	dir := t.TempDir()
	items := []newsapi.Article{{
		Title: "Example article",
		URL:   "https://example.com/article",
	}}

	if err := newsapi.SaveArticles(dir, items); err != nil {
		t.Fatalf("SaveArticles returned error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "article_01.json")); err != nil {
		t.Fatalf("missing article file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "articles.json")); err != nil {
		t.Fatalf("missing aggregate file: %v", err)
	}
}
