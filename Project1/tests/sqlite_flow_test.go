package integration_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"go-mongo-docker/newsapi"
)

// redirectTransport is an http.RoundTripper that rewrites the request URL to a different target host.
type redirectTransport struct {
	target string
	base   http.RoundTripper
}

// RoundTrip rewrites the request URL to the target host and delegates to the base RoundTripper.
func (transport redirectTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	redirected := request.Clone(request.Context())
	redirected.URL = new(url.URL)
	*redirected.URL = *request.URL
	target, _ := url.Parse(transport.target)
	redirected.URL.Scheme, redirected.URL.Host = target.Scheme, target.Host
	redirected.Host = target.Host
	return transport.base.RoundTrip(redirected)
}

func TestSQLiteBackedApplicationFlow(t *testing.T) {
	serverCalls := atomic.Int32{}
	parallelCalls := atomic.Int32{}
	parallelReady := make(chan struct{})
	overlapCalls := atomic.Int32{}
	overlapReady := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("apiKey") != "integration-test-key" {
			http.Error(w, "unexpected API key", http.StatusUnauthorized)
			return
		}
		if strings.HasPrefix(r.URL.Query().Get("q"), "parallel-") {
			if parallelCalls.Add(1) == 2 {
				close(parallelReady)
			}
			select {
			case <-parallelReady:
			case <-time.After(3 * time.Second):
				http.Error(w, "independent SQLite searches did not overlap", http.StatusGatewayTimeout)
				return
			}
		}
		if r.URL.Query().Get("q") == "overlap" {
			if overlapCalls.Add(1) == 2 {
				close(overlapReady)
			}
			select {
			case <-overlapReady:
			case <-time.After(3 * time.Second):
				http.Error(w, "overlap timed out", 504)
				return
			}
		}
		call := serverCalls.Add(1)
		var articles []newsapi.Article
		switch call {
		case 1:
			articles = makeArticles(1)
		case 2:
			articles = makeArticles(3)
		case 3:
			articles = append(makeArticles(3), makeArticle("flow-older", 6*24*time.Hour))
		default:
			articles = []newsapi.Article{makeArticle(r.URL.Query().Get("q"), time.Hour)}
		}
		if r.URL.Query().Get("q") == "mixed-limits" {
			articles = makeArticles(30)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "articles": articles})
	}))
	defer server.Close()

	// Use a mock HTTP transport; SQLite accesses a local temporary file.
	apiClient := &http.Client{Transport: redirectTransport{target: server.URL, base: http.DefaultTransport}}

	path := filepath.Join(t.TempDir(), "news.db")
	t.Setenv("DATABASE_PATH", path)
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Run a series of requests to test the full application flow, including caching behavior and concurrent requests.
	request := newsapi.Request{HTTPClient: apiClient, APIKey: "integration-test-key", Topic: "flow", Days: 3, Limit: 1}
	items, fromAPI, err := newsapi.FetchCachedArticlesWithSource("", request)
	if err != nil || !fromAPI || len(items) != 1 {
		t.Fatalf("first search: got %d articles, fromAPI=%t, err=%v", len(items), fromAPI, err)
	}
	assertArticleCount(t, db, "flow", 1)

	// Repeat the same request to verify that the result is served from the SQLite cache and does not trigger an API call.
	items, fromAPI, err = newsapi.FetchCachedArticlesWithSource("", request)
	if err != nil || fromAPI || len(items) != 1 || serverCalls.Load() != 1 {
		t.Fatalf("repeat search should use SQLite: got %d articles, fromAPI=%t, API calls=%d, err=%v", len(items), fromAPI, serverCalls.Load(), err)
	}

	// Expand the article limit to verify that additional articles are fetched from the API and stored in the SQLite cache.
	request.Limit = 3
	items, fromAPI, err = newsapi.FetchCachedArticlesWithSource("", request)
	if err != nil || !fromAPI || len(items) != 3 {
		t.Fatalf("expanded article limit: got %d articles, fromAPI=%t, err=%v", len(items), fromAPI, err)
	}
	assertArticleCount(t, db, "flow", 3)

	// Expand the date range to verify that older articles are fetched from the API and stored in the SQLite cache.
	request.Days, request.Limit = 7, 4
	items, fromAPI, err = newsapi.FetchCachedArticlesWithSource("", request)
	if err != nil || !fromAPI || len(items) != 4 {
		t.Fatalf("expanded date range: got %d articles, fromAPI=%t, err=%v", len(items), fromAPI, err)
	}
	if items[3].Title != "flow-older" {
		t.Fatalf("expanded date range did not return the older article: got %q", items[3].Title)
	}
	assertArticleCount(t, db, "flow", 4)

	// Test concurrent requests to verify that the application can handle multiple requests in parallel and that the SQLite cache is used appropriately.
	requests := []newsapi.Request{
		{HTTPClient: apiClient, APIKey: "integration-test-key", Topic: "parallel-a", Days: 1, Limit: 1},
		{HTTPClient: apiClient, APIKey: "integration-test-key", Topic: "parallel-b", Days: 1, Limit: 1},
	}
	results, errs := newsapi.ProcessCachedRequests("", requests)
	resultCount := 0
	for result := range results {
		if result.Err != nil || len(result.Articles) != 1 {
			t.Errorf("concurrent search %q returned %d articles: %v", result.Request.Topic, len(result.Articles), result.Err)
		}
		assertArticleCount(t, db, result.Request.Topic, 1)
		resultCount++
	}
	for err := range errs {
		t.Errorf("concurrent search failed: %v", err)
	}
	if resultCount != len(requests) || serverCalls.Load() != 5 {
		t.Fatalf("concurrent searches: got %d results and %d API calls, want 2 and 5", resultCount, serverCalls.Load())
	}

	// Short results must persist exhaustion metadata and serve repeat queries.
	short := newsapi.Request{HTTPClient: apiClient, APIKey: "integration-test-key", Topic: "short", Days: 1, Limit: 20}
	for i := range 2 {
		items, api, err := newsapi.FetchCachedArticlesWithSource("", short)
		if err != nil || len(items) != 1 || api != (i == 0) {
			t.Fatalf("short search %d: count=%d api=%t err=%v", i, len(items), api, err)
		}
	}
	if serverCalls.Load() != 6 {
		t.Fatalf("repeat short query reached API: %d calls", serverCalls.Load())
	}
	short.Limit = 21
	if _, api, err := newsapi.FetchCachedArticlesWithSource("", short); err != nil || !api {
		t.Fatalf("expanded short query: api=%t err=%v", api, err)
	}

	before := serverCalls.Load()
	mixed, failures := newsapi.ProcessCachedRequests("", []newsapi.Request{
		{HTTPClient: apiClient, APIKey: "integration-test-key", Topic: "mixed-limits", Days: 3, Limit: 5},
		{HTTPClient: apiClient, APIKey: "integration-test-key", Topic: "mixed-limits", Days: 3, Limit: 20},
	})
	count := 0
	for result := range mixed {
		count++
		if len(result.Articles) != result.Request.Limit {
			t.Errorf("limit %d got %d", result.Request.Limit, len(result.Articles))
		}
	}
	for err := range failures {
		t.Error(err)
	}
	if count != 2 || serverCalls.Load()-before != 1 {
		t.Fatalf("mixed limits: results=%d API calls=%d", count, serverCalls.Load()-before)
	}

	overlapping, failures := newsapi.ProcessCachedRequests("", []newsapi.Request{
		{HTTPClient: apiClient, APIKey: "integration-test-key", Topic: "overlap", Days: 1, Limit: 1},
		{HTTPClient: apiClient, APIKey: "integration-test-key", Topic: "overlap", Days: 2, Limit: 1},
	})
	count = 0
	for result := range overlapping {
		count++
		if len(result.Articles) != 1 {
			t.Errorf("overlapping search returned %d articles", len(result.Articles))
		}
	}
	for err := range failures {
		t.Error(err)
	}
	if count != 2 {
		t.Fatalf("overlapping search successes=%d", count)
	}
	assertArticleCount(t, db, "overlap", 1)

	// Expired exhaustion metadata must allow fresh news to be discovered.
	_, err = db.Exec("UPDATE coverage SET fetched_at=? WHERE topic=?", time.Now().Add(-time.Hour).UnixMilli(), "short")
	if err != nil {
		t.Fatal(err)
	}
	if _, api, err := newsapi.FetchCachedArticlesWithSource("", short); err != nil || !api {
		t.Fatalf("expired SQLite cache: api=%t err=%v", api, err)
	}
}

// makeArticles generates a slice of articles with the specified count, each with a unique title and publication time offset.
func makeArticles(count int) []newsapi.Article {
	articles := make([]newsapi.Article, count)
	for i := range articles {
		articles[i] = makeArticle(fmt.Sprintf("flow-%d", i+1), time.Duration(i+1)*time.Hour)
	}
	return articles
}

// makeArticle creates a single article with the given title and age (time since publication).
func makeArticle(title string, age time.Duration) newsapi.Article {
	published := time.Now().UTC().Add(-age)
	return newsapi.Article{
		Title:       title,
		URL:         "https://example.test/" + title,
		PublishedAt: &published,
	}
}

// assertArticleCount checks that the number of articles stored in the SQLite collection for a given topic matches the expected count.
func assertArticleCount(t *testing.T, db *sql.DB, topic string, want int64) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var got int64
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM articles WHERE topic=?", topic).Scan(&got)
	if err != nil || got != want {
		t.Fatalf("persisted article count for %q = %d, want %d (err=%v)", topic, got, want, err)
	}
}
