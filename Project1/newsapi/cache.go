package newsapi

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const cacheLifetime = 15 * time.Minute

// Only searches sharing a cache entry wait for each other. Remove unused locks
// so a long-running process does not retain every query it has ever seen.
var cacheLocks = struct {
	sync.Mutex
	entries map[string]*cacheLock
}{entries: make(map[string]*cacheLock)}

type cacheLock struct {
	sync.Mutex
	users int
}

func lockCache(key string) func() {
	cacheLocks.Lock()
	entry := cacheLocks.entries[key]
	if entry == nil {
		entry = &cacheLock{}
		cacheLocks.entries[key] = entry
	}
	entry.users++
	cacheLocks.Unlock()
	entry.Lock()
	return func() {
		entry.Unlock()
		cacheLocks.Lock()
		entry.users--
		if entry.users == 0 {
			delete(cacheLocks.entries, key)
		}
		cacheLocks.Unlock()
	}
}

type articleCache struct {
	From           time.Time
	To             time.Time
	Articles       []Article
	FetchedAt      time.Time
	Exhausted      bool
	RequestedLimit int
}

// RequestResult carries one request's outcome after a goroutine finishes.
type RequestResult struct {
	Request  Request
	Articles []Article
	Err      error
	FromAPI  bool
	Index    int
}

// ProcessRequests fans out independent requests to goroutines and returns results/errors over channels.
func ProcessRequests(requests []Request, fetch func(Request) ([]Article, error)) (<-chan RequestResult, <-chan error) {
	return ProcessRequestsWithSource(requests, func(req Request) ([]Article, bool, error) {
		items, err := fetch(req)
		return items, false, err
	})
}

// ProcessRequestsWithSource preserves request identity and cache provenance.
func ProcessRequestsWithSource(requests []Request, fetch func(Request) ([]Article, bool, error)) (<-chan RequestResult, <-chan error) {
	requests = append([]Request(nil), requests...)
	results := make(chan RequestResult, len(requests))
	errs := make(chan error, len(requests))

	go func() {
		var wg sync.WaitGroup
		wg.Add(len(requests))

		for index, request := range requests {
			go func(index int, req Request) {
				defer wg.Done()

				items, fromAPI, err := fetch(req)
				if err != nil {
					errs <- fmt.Errorf("request %d (%q): %w", index+1, req.Topic, err)
					return
				}

				results <- RequestResult{Request: req, Articles: items, FromAPI: fromAPI, Index: index}
			}(index, request)
		}

		wg.Wait()
		close(results)
		close(errs)
	}()

	return results, errs
}

// ProcessCachedRequests is the directory-aware helper for many independent searches using the shared cache logic.
func ProcessCachedRequests(dir string, requests []Request) (<-chan RequestResult, <-chan error) {
	return ProcessRequestsWithSource(requests, func(req Request) ([]Article, bool, error) {
		return FetchCachedArticlesWithSource(dir, req)
	})
}

// FetchCachedArticles reads the cache before fetching missing date coverage or article counts.
func FetchCachedArticles(dir string, req Request) ([]Article, error) {
	articles, _, err := FetchCachedArticlesWithSource(dir, req)
	return articles, err
}

// FetchCachedArticlesWithSource reports whether filling the result required a NewsAPI request.
func FetchCachedArticlesWithSource(dir string, req Request) ([]Article, bool, error) {
	if path := os.Getenv("DATABASE_PATH"); path != "" {
		return fetchSQLiteArticles(path, req)
	}
	if os.Getenv("MONGO_URI") != "" {
		return nil, false, fmt.Errorf("MongoDB is no longer supported: set DATABASE_PATH for SQLite and remove MONGO_URI")
	}
	return fetchCachedArticles(dir, req, time.Now().UTC(), FetchArticles)
}

// fetchCachedArticles serializes only matching queries and rechecks each caller's
// limit after acquiring the lock. Network calls for unrelated searches overlap.
func fetchCachedArticles(dir string, req Request, now time.Time, fetch func(Request) ([]Article, error), db ...*sql.DB) ([]Article, bool, error) {
	req.Topic, req.Country = strings.ToLower(strings.Join(strings.Fields(req.Topic), " ")), strings.ToLower(strings.TrimSpace(req.Country))
	if req.Country == "" {
		req.Country = "us"
	}
	req.Days, req.Limit = max(1, req.Days), max(1, req.Limit)
	to := now.UTC().Truncate(24*time.Hour).AddDate(0, 0, 1)
	from := to.AddDate(0, 0, -req.Days)
	key := sha256.Sum256([]byte(req.Topic + "\x00" + req.Country + "\x00" + from.Format(time.RFC3339) + "\x00" + to.Format(time.RFC3339)))
	scope := dir
	if len(db) == 0 {
		var err error
		scope, err = filepath.Abs(dir)
		if err != nil {
			return nil, false, err
		}
	}
	unlock := lockCache(scope + "\x00" + fmt.Sprintf("%x", key))
	defer unlock()
	path := filepath.Join(dir, fmt.Sprintf("%x.json", key))
	var cache articleCache
	if len(db) > 0 {
		var err error
		cache, err = loadSQLiteCache(db[0], req, from, to)
		if err != nil {
			return nil, false, err
		}
	} else {
		data, err := os.ReadFile(path)
		if err == nil {
			err = json.Unmarshal(data, &cache)
		}
		if err != nil && !os.IsNotExist(err) {
			return nil, false, err
		}
	}
	cache.Articles = mergeArticles(nil, cache.Articles)
	items := matchingArticles(cache.Articles, from, to)
	covered := !cache.From.IsZero() && !from.Before(cache.From) && !to.After(cache.To)
	freshCache := !cache.FetchedAt.IsZero() && now.Before(cache.FetchedAt.Add(cacheLifetime))
	enough := len(items) >= req.Limit || (cache.Exhausted && req.Limit <= cache.RequestedLimit)
	if covered && freshCache && enough {
		return items[:min(len(items), req.Limit)], false, nil
	}
	cache.Exhausted = false
	var fetched []Article
	for page := 1; ; page++ {
		query := req
		query.Page, query.Limit = page, 100
		fresh, err := fetch(query)
		if err != nil {
			return nil, false, err
		}
		previousCount := len(fetched)
		fetched = mergeArticles(fetched, fresh)
		cache.Articles = mergeArticles(cache.Articles, fresh)
		items = matchingArticles(cache.Articles, from, to)
		if len(fresh) < query.Limit {
			cache.Exhausted = true
			break
		}
		if len(items) >= req.Limit {
			break
		}
		// Avoid an unbounded loop if an upstream service repeats a full page.
		if page > 1 && len(fetched) == previousCount {
			return nil, false, fmt.Errorf("NewsAPI pagination made no progress at page %d", page)
		}
	}
	cache.From, cache.To = from, to
	cache.FetchedAt, cache.RequestedLimit = now, req.Limit
	if len(db) > 0 {
		if err := saveSQLiteCache(db[0], req, cache); err != nil {
			return nil, false, err
		}
	} else {
		if err := saveFileCache(path, cache); err != nil {
			return nil, false, err
		}
	}
	return items[:min(len(items), req.Limit)], true, nil
}

// Publish complete JSON in one rename so other processes never read a partial write.
func saveFileCache(path string, cache articleCache) error {
	data, err := json.Marshal(cache)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".cache-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

// mergeArticles keeps one article per URL, using full content when a URL is absent.
func mergeArticles(cached, fresh []Article) []Article {
	seen := make(map[string]bool)
	var merged []Article
	for _, article := range append(cached, fresh...) {
		key := articleKey(article)
		if !seen[key] {
			merged = append(merged, article)
			seen[key] = true
		}
	}
	return merged
}

// articleKey uses URL identity or a content hash stable across millisecond timestamp rounding.
func articleKey(article Article) string {
	if article.URL != "" {
		return article.URL
	}
	if article.PublishedAt != nil {
		stamp := article.PublishedAt.UTC().Truncate(time.Millisecond)
		article.PublishedAt = &stamp
	}
	data, _ := json.Marshal(article)
	return fmt.Sprintf("content:%x", sha256.Sum256(data))
}

// matchingArticles selects dated articles within the requested interval, newest first.
func matchingArticles(articles []Article, from, to time.Time) []Article {
	items := []Article{}
	for _, article := range articles {
		if article.PublishedAt != nil && !article.PublishedAt.Before(from) && article.PublishedAt.Before(to) {
			items = append(items, article)
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].PublishedAt.After(*items[j].PublishedAt) })
	return items
}
