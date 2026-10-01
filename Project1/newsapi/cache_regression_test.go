package newsapi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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

func readCacheFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read cache fixture %s: %v", name, err)
	}
	return data
}

type cacheFixtureOutcome struct {
	APICalls int      `json:"api_calls"`
	Sources  []string `json:"sources"`
	Titles   []string `json:"titles"`
}

type cacheFixture struct {
	Request  Request `json:"request"`
	Response struct {
		Status   string    `json:"status"`
		Articles []Article `json:"articles"`
	} `json:"newsapi_response"`
	Expected struct {
		Miss cacheFixtureOutcome `json:"miss"`
		Hit  cacheFixtureOutcome `json:"hit"`
	} `json:"expected"`
}

func loadCacheFixture(t *testing.T) cacheFixture {
	t.Helper()
	var fixture cacheFixture
	if err := json.Unmarshal(readCacheFixture(t, "cache-case.json"), &fixture); err != nil {
		t.Fatalf("decode cache fixture: %v", err)
	}
	return fixture
}

func cacheFixtureFetcher(fixture cacheFixture, calls *int) func(Request) ([]Article, error) {
	return func(Request) ([]Article, error) {
		(*calls)++
		if fixture.Response.Status != "ok" {
			return nil, fmt.Errorf("unexpected fixture status %q", fixture.Response.Status)
		}
		return fixture.Response.Articles, nil
	}
}

func assertCacheFixture(t *testing.T, got, want cacheFixtureOutcome) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cache fixture result = %+v, want %+v", got, want)
	}
}

func cacheSource(fromAPI bool) string {
	if fromAPI {
		return "News API"
	}
	return "cache"
}

func articleTitles(articles []Article) []string {
	titles := make([]string, len(articles))
	for i, article := range articles {
		titles[i] = article.Title
	}
	return titles
}

func TestCacheMissFixture(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	fixture := loadCacheFixture(t)
	var calls int
	articles, fromAPI, err := fetchCachedArticles(t.TempDir(), fixture.Request, now, cacheFixtureFetcher(fixture, &calls))
	if err != nil {
		t.Fatalf("cache miss returned error: %v", err)
	}
	got := cacheFixtureOutcome{calls, []string{cacheSource(fromAPI)}, articleTitles(articles)}
	assertCacheFixture(t, got, fixture.Expected.Miss)
}

func TestCacheHitFixture(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	fixture := loadCacheFixture(t)
	dir := t.TempDir()
	var calls int
	fetch := cacheFixtureFetcher(fixture, &calls)
	first, firstFromAPI, err := fetchCachedArticles(dir, fixture.Request, now, fetch)
	if err != nil {
		t.Fatalf("initial cache miss returned error: %v", err)
	}
	second, secondFromAPI, err := fetchCachedArticles(dir, fixture.Request, now, fetch)
	if err != nil {
		t.Fatalf("cache hit returned error: %v", err)
	}
	if !reflect.DeepEqual(articleTitles(first), articleTitles(second)) {
		t.Fatalf("cache hit articles differ: first=%v second=%v", articleTitles(first), articleTitles(second))
	}
	got := cacheFixtureOutcome{calls, []string{cacheSource(firstFromAPI), cacheSource(secondFromAPI)}, articleTitles(second)}
	assertCacheFixture(t, got, fixture.Expected.Hit)
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

func TestCacheDateWindowIncludesTodayAndPreviousDay(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	previousDay := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	today := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	beforeWindow := time.Date(2026, 9, 29, 23, 59, 0, 0, time.UTC)
	nextDayBoundary := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	articles := []Article{
		{Title: "previous day", URL: "https://test/previous", PublishedAt: &previousDay},
		{Title: "today", URL: "https://test/today", PublishedAt: &today},
		{Title: "before window", URL: "https://test/before", PublishedAt: &beforeWindow},
		{Title: "next day", URL: "https://test/next", PublishedAt: &nextDayBoundary},
	}

	for _, test := range []struct {
		days int
		want []string
	}{
		{days: 1, want: []string{"today"}},
		{days: 2, want: []string{"today", "previous day"}},
	} {
		t.Run(fmt.Sprintf("days_%d", test.days), func(t *testing.T) {
			got, _, err := fetchCachedArticles(t.TempDir(), Request{Topic: "date range", Days: test.days, Limit: 10}, now,
				func(Request) ([]Article, error) { return articles, nil })
			if err != nil {
				t.Fatalf("fetchCachedArticles returned error: %v", err)
			}
			if len(got) != len(test.want) {
				t.Fatalf("got %d articles, want %d: %#v", len(got), len(test.want), got)
			}
			for i, title := range test.want {
				if got[i].Title != title {
					t.Errorf("article %d = %q, want %q", i, got[i].Title, title)
				}
			}
		})
	}
}

func TestMergeArticlesDeduplicatesByURLAndContent(t *testing.T) {
	cached := []Article{
		{Title: "cached title", URL: "https://test/shared"},
		{Title: "same content"},
	}
	fresh := []Article{
		{Title: "fresh title", URL: "https://test/shared"},
		{Title: "same content"},
		{Title: "different content"},
	}

	got := mergeArticles(cached, fresh)
	if len(got) != 3 {
		t.Fatalf("got %d merged articles, want 3: %#v", len(got), got)
	}
	if got[0].Title != "cached title" {
		t.Fatalf("duplicate URL replaced the cached article: %#v", got[0])
	}
}

func TestFetchCachedArticlesDeduplicatesAndAppliesResultLimit(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	newest := time.Date(2026, 10, 1, 11, 0, 0, 0, time.UTC)
	second := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	third := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	articles := []Article{
		{Title: "newest", URL: "https://test/newest", PublishedAt: &newest},
		{Title: "duplicate", URL: "https://test/newest", PublishedAt: &newest},
		{Title: "second", URL: "https://test/second", PublishedAt: &second},
		{Title: "third", URL: "https://test/third", PublishedAt: &third},
	}

	got, _, err := fetchCachedArticles(t.TempDir(), Request{Topic: "limited", Days: 2, Limit: 2}, now,
		func(Request) ([]Article, error) { return articles, nil })
	if err != nil {
		t.Fatalf("fetchCachedArticles returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d articles, want limit of 2", len(got))
	}
	if got[0].Title != "newest" || got[1].Title != "second" {
		t.Fatalf("got unexpected limited results: %#v", got)
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
