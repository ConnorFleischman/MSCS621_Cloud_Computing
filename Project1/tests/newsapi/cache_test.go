// cache_test.go verifies persistent cache coverage, count expansion, pagination, and failure handling offline.
package newsapi

import (
	"fmt"
	"testing"
	"time"
)

// TestCacheExpansion checks that only insufficient coverage or counts cause additional fetches.
func TestCacheExpansion(t *testing.T) {
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	older := now.AddDate(0, 0, -4)
	a, b := Article{URL: "a", PublishedAt: &now}, Article{URL: "b", PublishedAt: &older}
	dir, calls := t.TempDir(), 0
	fetch := func(req Request) ([]Article, error) { calls++; return []Article{a, a, b}, nil }
	
	for _, tc := range []struct{ days, limit, want, calls int }{
		{1, 1, 1, 1}, // Initial fetch.
		{1, 1, 1, 1}, // Persistent cache hit.
		{7, 1, 1, 2}, // Wider coverage despite sufficient count.
		{7, 2, 2, 2}, // Merged results are already sufficient.
		{1, 2, 1, 3}, // Older articles cannot satisfy a narrower request.
		{7, 3, 2, 4}, // Return available unique results when API is exhausted.
	} {
		items, err := fetchCachedArticles(dir, Request{Topic: "go", Days: tc.days, Limit: tc.limit}, now, fetch)
		if err != nil || len(items) != tc.want || calls != tc.calls {
			t.Fatalf("%+v: got %d articles, %d calls, %v", tc, len(items), calls, err)
		}
	}
	if _, err := fetchCachedArticles(dir, Request{Topic: "different"}, now, fetch); err != nil || calls != 5 {
		t.Fatalf("topic isolation: calls=%d, err=%v", calls, err)
	}
	if _, err := fetchCachedArticles(dir, Request{Topic: "go"}, now.AddDate(0, 0, 1), fetch); err != nil || calls != 6 {
		t.Fatalf("new day: calls=%d, err=%v", calls, err)
	}
}

// TestCachePagination ensures a duplicate cached first page does not hide additional articles.
func TestCachePagination(t *testing.T) {
	now, dir, calls := time.Now().UTC(), t.TempDir(), 0
	fetch := func(req Request) ([]Article, error) {
		calls++
		var items []Article
		for i := (req.Page - 1) * 100; i < min(req.Page*100, 101); i++ {
			items = append(items, Article{URL: fmt.Sprint(i), PublishedAt: &now})
		}
		return items, nil
	}
	for _, limit := range []int{100, 101, 101} {
		items, err := fetchCachedArticles(dir, Request{Topic: "go", Limit: limit}, now, fetch)
		if err != nil || len(items) != limit {
			t.Fatalf("limit %d: got %d articles, %v", limit, len(items), err)
		}
	}
	if calls != 3 {
		t.Fatalf("got %d calls, want 3", calls)
	}
}

// TestCacheFetchFailure verifies that failed expansion leaves existing cache coverage intact.
func TestCacheFetchFailure(t *testing.T) {
	now, dir := time.Now().UTC(), t.TempDir()
	req := Request{Topic: "go", Days: 1, Limit: 1}
	fetch := func(Request) ([]Article, error) { return []Article{{URL: "a", PublishedAt: &now}}, nil }
	if _, err := fetchCachedArticles(dir, req, now, fetch); err != nil {
		t.Fatal(err)
	}
	fail := func(Request) ([]Article, error) { return nil, fmt.Errorf("offline") }
	req.Days = 7
	if _, err := fetchCachedArticles(dir, req, now, fail); err == nil {
		t.Fatal("expected failed expansion")
	}
	req.Days = 1
	if items, err := fetchCachedArticles(dir, req, now, fail); err != nil || len(items) != 1 {
		t.Fatalf("cache was damaged: %v", err)
	}
}
