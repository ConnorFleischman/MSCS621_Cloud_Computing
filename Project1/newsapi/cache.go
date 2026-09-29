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

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"golang.org/x/sync/singleflight"
)

var cacheMu sync.Mutex
var cacheMisses singleflight.Group

type articleCache struct {
	From     time.Time
	To       time.Time
	Articles []Article
}

// RequestResult carries one request's outcome after a goroutine finishes.
type RequestResult struct {
	Request  Request
	Articles []Article
	Err      error
}

// ProcessRequests fans out independent requests to goroutines and returns results/errors over channels.
func ProcessRequests(requests []Request, fetch func(Request) ([]Article, error)) (<-chan RequestResult, <-chan error) {
	results := make(chan RequestResult, len(requests))
	errs := make(chan error, len(requests))

	go func() {
		var wg sync.WaitGroup
		wg.Add(len(requests))

		for _, request := range requests {
			go func(req Request) {
				defer wg.Done()

				items, err := fetch(req)
				if err != nil {
					errs <- err
					return
				}

				results <- RequestResult{Request: req, Articles: items}
			}(request)
		}

		wg.Wait()
		close(results)
		close(errs)
	}()

	return results, errs
}

// ProcessCachedRequests is the directory-aware helper for many independent searches using the shared cache logic.
func ProcessCachedRequests(dir string, requests []Request) (<-chan RequestResult, <-chan error) {
	return ProcessRequests(requests, func(req Request) ([]Article, error) {
		return FetchCachedArticles(dir, req)
	})
}

// FetchCachedArticles reads the cache before fetching missing date coverage or article counts.
func FetchCachedArticles(dir string, req Request) ([]Article, error) {
	if uri := os.Getenv("MONGO_URI"); uri != "" {
		return fetchMongoArticles(uri, req)
	}
	return fetchCachedArticles(dir, req, time.Now().UTC(), FetchArticles)
}

// fetchCachedArticles serializes cache updates and allows deterministic, offline verification.
func fetchCachedArticles(dir string, req Request, now time.Time, fetch func(Request) ([]Article, error), db ...*mongo.Database) ([]Article, error) {
	req.Topic, req.Country = strings.ToLower(strings.Join(strings.Fields(req.Topic), " ")), strings.ToLower(strings.TrimSpace(req.Country))
	if req.Country == "" {
		req.Country = "us"
	}

	req.Days, req.Limit = max(1, req.Days), max(1, req.Limit)
	to := now.UTC().Truncate(24*time.Hour).AddDate(0, 0, 1)
	from := to.AddDate(0, 0, -req.Days)
	key := sha256.Sum256([]byte(req.Topic + "\x00" + req.Country + "\x00" + from.Format(time.RFC3339) + "\x00" + to.Format(time.RFC3339)))
	cacheKey := dir + "\x00" + fmt.Sprintf("%x", key)

	result, err, _ := cacheMisses.Do(cacheKey, func() (interface{}, error) {
		cacheMu.Lock()
		defer cacheMu.Unlock()

		path := filepath.Join(dir, fmt.Sprintf("%x.json", key))

		var cache articleCache
		if len(db) > 0 {
			var loadErr error
			cache, loadErr = loadMongoCache(db[0], req, from, to)
			if loadErr != nil && !os.IsNotExist(loadErr) {
				return nil, loadErr
			}
		} else {
			var data []byte
			var readErr error
			data, readErr = os.ReadFile(path)
			if readErr == nil {
				readErr = json.Unmarshal(data, &cache)
			}
			if readErr != nil && !os.IsNotExist(readErr) {
				return nil, readErr
			}
		}

		cache.Articles = mergeArticles(nil, cache.Articles)
		items := matchingArticles(cache.Articles, from, to)

		if cache.From.IsZero() || from.Before(cache.From) || to.After(cache.To) || len(items) < req.Limit {
			for page := 1; ; page++ {
				query := req
				query.Page, query.Limit = page, 100
				fresh, fetchErr := fetch(query)
				if fetchErr != nil {
					return nil, fetchErr
				}

				cache.Articles = mergeArticles(cache.Articles, fresh)
				items = matchingArticles(cache.Articles, from, to)
				if len(items) >= req.Limit || len(fresh) < query.Limit {
					break
				}
			}

			if cache.From.IsZero() || from.Before(cache.From) || from.After(cache.To) {
				cache.From = from
			}
			if to.After(cache.To) {
				cache.To = to
			}

			if len(db) > 0 {
				return items[:min(len(items), req.Limit)], saveMongoCache(db[0], req, cache)
			}
			data, marshalErr := json.Marshal(cache)
			if marshalErr != nil {
				return nil, marshalErr
			}

			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, err
			}

			if err := os.WriteFile(path, data, 0o644); err != nil {
				return nil, err
			}
		}

		return items[:min(len(items), req.Limit)], nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]Article), nil
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

// articleKey uses URL identity or a content hash stable across BSON timestamp rounding.
func articleKey(article Article) string {
	if article.URL != "" {
		return article.URL
	}
	data, _ := bson.Marshal(article)
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
