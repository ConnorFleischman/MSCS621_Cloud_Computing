package newsapi

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func cacheTestArticles(now time.Time, count int) []Article {
	items := make([]Article, count)
	for i := range items {
		items[i] = Article{Title: fmt.Sprint(i), URL: fmt.Sprintf("https://test/%d", i), PublishedAt: &now}
	}
	return items
}

func TestIndependentCacheMissesOverlap(t *testing.T) {
	dir, now := t.TempDir(), time.Now().UTC()
	entered := make(chan string, 2)
	release := make(chan struct{})
	fetch := func(req Request) ([]Article, error) {
		entered <- req.Topic
		<-release
		return cacheTestArticles(now, 1), nil
	}
	var wg sync.WaitGroup
	for _, topic := range []string{"alpha", "beta"} {
		wg.Add(1)
		go func(topic string) {
			defer wg.Done()
			_, _, err := fetchCachedArticles(dir, Request{Topic: topic, Days: 1, Limit: 1}, now, fetch)
			if err != nil {
				t.Error(err)
			}
		}(topic)
	}
	defer wg.Wait()
	defer close(release)
	for range 2 {
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			t.Fatal("unrelated cache misses did not overlap")
		}
	}
}

func TestConcurrentLimitsRemainIndependent(t *testing.T) {
	for _, limits := range [][]int{{5, 20}, {20, 5}} {
		t.Run(fmt.Sprint(limits), func(t *testing.T) {
			dir, now := t.TempDir(), time.Now().UTC()
			entered, release := make(chan struct{}), make(chan struct{})
			var calls atomic.Int32
			fetch := func(Request) ([]Article, error) {
				if calls.Add(1) == 1 {
					close(entered)
					<-release
				}
				return cacheTestArticles(now, 30), nil
			}
			var wg sync.WaitGroup
			run := func(limit int) {
				defer wg.Done()
				items, _, err := fetchCachedArticles(dir, Request{Topic: "same", Days: 1, Limit: limit}, now, fetch)
				if err != nil || len(items) != limit {
					t.Errorf("limit %d: got %d, err=%v", limit, len(items), err)
				}
			}
			wg.Add(1)
			go run(limits[0])
			<-entered
			wg.Add(1)
			go run(limits[1])
			close(release)
			wg.Wait()
			if calls.Load() != 1 {
				t.Errorf("prefetched results should satisfy both limits; calls=%d", calls.Load())
			}
		})
	}
}

func TestShortAndEmptySearchesAreCachedAndRefreshed(t *testing.T) {
	for _, count := range []int{0, 3} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			dir := t.TempDir()
			now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
			calls := 0
			fetch := func(Request) ([]Article, error) { calls++; return cacheTestArticles(now, count), nil }
			req := Request{Topic: "short", Days: 1, Limit: 20}
			for i := range 2 {
				items, fromAPI, err := fetchCachedArticles(dir, req, now, fetch)
				if err != nil || len(items) != count || fromAPI != (i == 0) {
					t.Fatalf("repeat %d: count=%d api=%t err=%v", i, len(items), fromAPI, err)
				}
			}
			if calls != 1 {
				t.Fatalf("repeat fetched again: %d", calls)
			}
			req.Limit = 30
			if _, api, err := fetchCachedArticles(dir, req, now, fetch); err != nil || !api {
				t.Fatalf("expanded limit: api=%t err=%v", api, err)
			}
			if _, api, err := fetchCachedArticles(dir, req, now.Add(cacheLifetime), fetch); err != nil || !api {
				t.Fatalf("expired cache: api=%t err=%v", api, err)
			}
		})
	}
}

func TestFailedFetchCanBeRetried(t *testing.T) {
	dir, now := t.TempDir(), time.Now().UTC()
	req := Request{Topic: "retry", Days: 1, Limit: 1}
	_, _, err := fetchCachedArticles(dir, req, now, func(Request) ([]Article, error) { return nil, fmt.Errorf("unavailable") })
	if err == nil {
		t.Fatal("expected error")
	}
	items, _, err := fetchCachedArticles(dir, req, now, func(Request) ([]Article, error) { return cacheTestArticles(now, 1), nil })
	if err != nil || len(items) != 1 {
		t.Fatalf("retry: %v", err)
	}
}

func TestExpandedLimitFetchesAdditionalPages(t *testing.T) {
	dir, now := t.TempDir(), time.Now().UTC()
	all := cacheTestArticles(now, 250)
	var pages []int
	fetch := func(req Request) ([]Article, error) {
		pages = append(pages, req.Page)
		start := (req.Page - 1) * req.Limit
		return all[start:min(start+req.Limit, len(all))], nil
	}
	for _, limit := range []int{150, 250} {
		items, _, err := fetchCachedArticles(dir, Request{Topic: "pages", Days: 1, Limit: limit}, now, fetch)
		if err != nil || len(items) != limit {
			t.Fatalf("limit %d: count=%d err=%v", limit, len(items), err)
		}
	}
	if fmt.Sprint(pages) != "[1 2 1 2 3]" {
		t.Fatalf("unexpected pages: %v", pages)
	}
}

func TestRepeatedUpstreamPageStopsWithError(t *testing.T) {
	dir, now := t.TempDir(), time.Now().UTC()
	calls := 0
	_, _, err := fetchCachedArticles(dir, Request{Topic: "repeated", Days: 1, Limit: 200}, now, func(Request) ([]Article, error) {
		calls++
		return cacheTestArticles(now, 100), nil
	})
	if err == nil || calls != 2 {
		t.Fatalf("expected pagination error after two repeated pages: calls=%d err=%v", calls, err)
	}
}
