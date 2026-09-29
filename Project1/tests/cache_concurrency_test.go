package main_test

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go-mongo-docker/newsapi"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestFetchCachedArticlesCoalescesConcurrentMisses(t *testing.T) {
	originalTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = originalTransport }()

	var calls atomic.Int32
	http.DefaultTransport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		payload := map[string]any{
			"status":       "ok",
			"totalResults": 2,
			"articles": []map[string]any{
				{
					"title":       "alpha",
					"url":         "https://example.com/a",
					"publishedAt": time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339),
				},
				{
					"title":       "beta",
					"url":         "https://example.com/b",
					"publishedAt": time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339),
				},
			},
		}
		body, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(string(body))),
			Request:    r,
		}, nil
	})

	dir := t.TempDir()
	req := newsapi.Request{APIKey: "dummy", Topic: "cloud computing", Country: "us", Days: 3, Limit: 2}

	var wg sync.WaitGroup
	results := make(chan []newsapi.Article, 8)
	start := make(chan struct{})
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			items, err := newsapi.FetchCachedArticles(dir, req)
			if err != nil {
				t.Error(err)
				return
			}
			results <- items
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	if got := calls.Load(); got != 1 {
		t.Fatalf("expected one underlying fetch for a concurrent miss, got %d", got)
	}

	var count int
	for items := range results {
		count++
		if len(items) != 2 {
			t.Fatalf("expected two articles per result, got %d", len(items))
		}
	}
	if count != 8 {
		t.Fatalf("expected 8 callers to get results, got %d", count)
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read cache dir: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one cache file to be written, got %d", len(files))
	}
}

func TestFetchCachedArticlesReportsAPIThenCache(t *testing.T) {
	originalTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = originalTransport }()

	var calls atomic.Int32
	http.DefaultTransport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		payload := map[string]any{
			"status": "ok",
			"articles": []map[string]any{{
				"title": "cached result", "url": "https://example.com/a",
				"publishedAt": time.Now().UTC().Format(time.RFC3339),
			}},
		}
		body, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body))), Request: r}, nil
	})

	dir := t.TempDir()
	req := newsapi.Request{APIKey: "dummy", Topic: "cloud computing", Days: 1, Limit: 1}
	if _, fromAPI, err := newsapi.FetchCachedArticlesWithSource(dir, req); err != nil || !fromAPI {
		t.Fatalf("first request should use NewsAPI, fromAPI=%t err=%v", fromAPI, err)
	}
	if _, fromAPI, err := newsapi.FetchCachedArticlesWithSource(dir, req); err != nil || fromAPI {
		t.Fatalf("second request should use cache, fromAPI=%t err=%v", fromAPI, err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected one NewsAPI call, got %d", got)
	}
}

func TestFetchCachedArticlesKeepsDifferentDateWindowsSeparate(t *testing.T) {
	originalTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = originalTransport }()

	var calls atomic.Int32
	http.DefaultTransport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		from := r.URL.Query().Get("from")
		payload := map[string]any{"status": "ok", "totalResults": 1, "articles": []map[string]any{{
			"title":       "result for " + from,
			"url":         "https://example.com/" + from,
			"publishedAt": time.Now().UTC().Format(time.RFC3339),
		}}}
		body, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body))), Request: r}, nil
	})

	dir := t.TempDir()
	shortReq := newsapi.Request{APIKey: "dummy", Topic: "cloud computing", Country: "us", Days: 1, Limit: 1}
	wideReq := newsapi.Request{APIKey: "dummy", Topic: "cloud computing", Country: "us", Days: 7, Limit: 1}

	shortItems, err := newsapi.FetchCachedArticles(dir, shortReq)
	if err != nil {
		t.Fatalf("short request failed: %v", err)
	}
	wideItems, err := newsapi.FetchCachedArticles(dir, wideReq)
	if err != nil {
		t.Fatalf("wide request failed: %v", err)
	}

	if got := calls.Load(); got != 2 {
		t.Fatalf("expected separate fetches for different date windows, got %d", got)
	}
	if len(shortItems) != 1 || len(wideItems) != 1 {
		t.Fatalf("expected one article per result, got short=%d wide=%d", len(shortItems), len(wideItems))
	}
	if shortItems[0].Title == wideItems[0].Title {
		t.Fatalf("expected different article payloads for distinct date windows, got identical titles %q", shortItems[0].Title)
	}
}
