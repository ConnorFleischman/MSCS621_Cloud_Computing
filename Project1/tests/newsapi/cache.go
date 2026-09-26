// cache.go persists query coverage and merges additional NewsAPI results without duplicates.
package newsapi

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var cacheMu sync.Mutex

type articleCache struct {
	From, To time.Time
	Articles []Article
}

// FetchCachedArticles reads the cache before fetching missing date coverage or article counts.
func FetchCachedArticles(dir string, req Request) ([]Article, error) {
	return fetchCachedArticles(dir, req, time.Now().UTC(), FetchArticles)
}

// fetchCachedArticles serializes cache updates and allows deterministic, offline verification.
func fetchCachedArticles(dir string, req Request, now time.Time, fetch func(Request) ([]Article, error)) ([]Article, error) {
	// Lock to ensure thread-safe access to the cache
	cacheMu.Lock()
	defer cacheMu.Unlock()
	
	// Normalize request parameters
	req.Topic, req.Country = strings.TrimSpace(req.Topic), strings.ToLower(strings.TrimSpace(req.Country))
	if req.Country == "" {
		req.Country = "us"
	}

	// Calculate date range for the request
	req.Days, req.Limit = max(1, req.Days), max(1, req.Limit)
	to := now.UTC().Truncate(24*time.Hour).AddDate(0, 0, 1)
	from := to.AddDate(0, 0, -req.Days)
	key := sha256.Sum256([]byte(req.Topic + "\x00" + req.Country))
	path := filepath.Join(dir, fmt.Sprintf("%x.json", key))

	// Read existing cache from file, or initialize a new cache if it doesn't exist
	var cache articleCache
	data, err := os.ReadFile(path)
	if err == nil {
		err = json.Unmarshal(data, &cache)
	}
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	// Merge cached articles with fresh fetches, ensuring no duplicates and proper date coverage
	cache.Articles = mergeArticles(nil, cache.Articles)
	items := matchingArticles(cache.Articles, from, to)

	// Fetch additional articles if the cache does not cover the requested date range or article count
	if cache.From.IsZero() || from.Before(cache.From) || to.After(cache.To) || len(items) < req.Limit {
		for page := 1; ; page++ {
			query := req
			query.Page, query.Limit = page, 100
			fresh, err := fetch(query)
			if err != nil {
				return nil, err
			}
			
			cache.Articles = mergeArticles(cache.Articles, fresh)
			items = matchingArticles(cache.Articles, from, to)
			if len(items) >= req.Limit || len(fresh) < query.Limit {
				break
			}
		}

		// Update cache date range and write back to file
		if cache.From.IsZero() || from.Before(cache.From) || from.After(cache.To) {
			cache.From = from
		}
		if to.After(cache.To) {
			cache.To = to
		}

		// Handle edge cases where cache might be empty or have no articles
		data, err = json.Marshal(cache)
		if err != nil {
			return nil, err
		}

		if err = os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}

		if err = os.WriteFile(path, data, 0o644); err != nil {
			return nil, err
		}
	}

	return items[:min(len(items), req.Limit)], nil
}

// mergeArticles keeps one article per URL, using full content when a URL is absent.
func mergeArticles(cached, fresh []Article) []Article {
	seen := make(map[string]bool)
	var merged []Article
	for _, article := range append(cached, fresh...) {
		key := article.URL
		if key == "" {
			data, _ := json.Marshal(article)
			key = string(data)
		}
		if !seen[key] {
			merged = append(merged, article)
			seen[key] = true
		}
	}
	return merged
}

// matchingArticles selects dated articles within the requested interval, newest first.
func matchingArticles(articles []Article, from, to time.Time) []Article {
	// Filter articles based on the specified date range
	items := []Article{}
	for _, article := range articles {
		if article.PublishedAt != nil && !article.PublishedAt.Before(from) && article.PublishedAt.Before(to) {
			items = append(items, article)
		}
	}
	// Sort articles by publication date in descending order
	sort.SliceStable(items, func(i, j int) bool { return items[i].PublishedAt.After(*items[j].PublishedAt) })
	
	return items
}
