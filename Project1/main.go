package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go-mongo-docker/newsapi"
)

var fetchArticles = newsapi.FetchCachedArticlesWithSource

func runApp(args []string) error {
	if err := newsapi.LoadEnvFile(".env"); err != nil {
		log.Printf("warning: could not load .env: %v", err)
	}

	fs := flag.NewFlagSet("newsapp", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	query := fs.String("q", "", "search keyword/topic (legacy alias)")
	topic := fs.String("topic", "technology", "topic or keyword to search")
	country := fs.String("country", "us", "country code for headlines when topic is empty")
	days := fs.Int("days", 1, "number of days to look back, including today")
	articles := fs.Int("articles", 1, "number of articles to request")
	outputDir := fs.String("output", "", "folder to save article JSON documents")
	cacheDir := fs.String("cache", ".newsapi-cache", "folder for cached queries")
	batch := fs.String("batch", "", "JSON file containing searches with topic, days, articles, and optional country")
	apiTimeout := fs.Duration("api-timeout", 15*time.Second, "timeout for NewsAPI requests")
	mongoTimeout := fs.Duration("mongo-timeout", 10*time.Second, "timeout for MongoDB cache operations")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if val := strings.TrimSpace(os.Getenv("NEWSAPI_TIMEOUT")); val != "" {
		d, err := time.ParseDuration(val)
		if err != nil {
			return fmt.Errorf("invalid NEWSAPI_TIMEOUT %q: %w", val, err)
		}
		apiTimeout = &d
	}
	if val := strings.TrimSpace(os.Getenv("MONGO_TIMEOUT")); val != "" {
		d, err := time.ParseDuration(val)
		if err != nil {
			return fmt.Errorf("invalid MONGO_TIMEOUT %q: %w", val, err)
		}
		mongoTimeout = &d
	}

	if *query != "" && *topic == "technology" {
		*topic = *query
	}
	if *days < 1 {
		return fmt.Errorf("days must be at least 1")
	}
	if *articles < 1 {
		return fmt.Errorf("articles must be at least 1")
	}

	request := newsapi.Request{
		APIKey:       strings.TrimSpace(os.Getenv("NEWSAPI_API_KEY")),
		Topic:        *topic,
		Country:      *country,
		Days:         *days,
		Limit:        *articles,
		Timeout:      *apiTimeout,
		MongoTimeout: *mongoTimeout,
	}

	requests := []newsapi.Request{request}
	if *batch != "" {
		var err error
		requests, err = readBatch(*batch, request)
		if err != nil {
			return err
		}
	}
	results, failures := newsapi.ProcessRequestsWithSource(requests, func(req newsapi.Request) ([]newsapi.Article, bool, error) {
		return fetchArticles(*cacheDir, req)
	})
	var errs []error
	for result := range results {
		if *batch != "" {
			fmt.Printf("\nSearch %d: %s\n", result.Index+1, result.Request.Topic)
		}
		printArticles(os.Stdout, result.Articles, result.Request.Limit, result.FromAPI)
		if *outputDir != "" {
			dir := *outputDir
			if *batch != "" {
				dir = filepath.Join(dir, fmt.Sprintf("search_%03d", result.Index+1))
			}
			if err := newsapi.SaveArticles(dir, result.Articles); err != nil {
				errs = append(errs, fmt.Errorf("save search %d: %w", result.Index+1, err))
			} else {
				fmt.Printf("\nSaved article documents to %s\n", dir)
			}
		}
	}
	for err := range failures {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func readBatch(path string, defaults newsapi.Request) ([]newsapi.Request, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var entries []struct {
		Topic    string `json:"topic"`
		Days     int    `json:"days"`
		Articles int    `json:"articles"`
		Country  string `json:"country"`
	}
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&entries); err != nil {
		return nil, fmt.Errorf("read batch: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("batch must contain one JSON array")
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("batch must contain at least one search")
	}
	requests := make([]newsapi.Request, len(entries))
	for i, entry := range entries {
		if strings.TrimSpace(entry.Topic) == "" || entry.Days < 1 || entry.Articles < 1 {
			return nil, fmt.Errorf("batch search %d requires a topic, days >= 1, and articles >= 1", i+1)
		}
		req := defaults
		req.Topic, req.Days, req.Limit = entry.Topic, entry.Days, entry.Articles
		if entry.Country != "" {
			req.Country = entry.Country
		}
		requests[i] = req
	}
	return requests, nil
}

func printArticles(w io.Writer, items []newsapi.Article, requested int, fromAPI bool) {
	fmt.Fprintf(w, "Fetched %d of %d requested article(s)\n", len(items), requested)
	source := "cache"
	if fromAPI {
		source = "News API"
	}
	fmt.Fprintf(w, "Results: %s\n", source)
	if len(items) < requested {
		fmt.Fprintf(w, "Only %d of %d requested article(s) were available.\n", len(items), requested)
	}
	if len(items) == 0 {
		fmt.Fprintln(w, "No articles returned for this query.")
		return
	}

	for i, item := range items {
		title := item.Title
		if title == "" {
			title = "(untitled)"
		}
		fmt.Fprintf(w, "\n%d. %s\n", i+1, title)
		if item.Source.Name != "" {
			fmt.Fprintf(w, "   Source: %s\n", item.Source.Name)
		}
		if item.Author != "" {
			fmt.Fprintf(w, "   Author: %s\n", item.Author)
		}
		if item.PublishedAt != nil {
			fmt.Fprintf(w, "   Published: %s\n", item.PublishedAt.Format("2006-01-02 15:04 MST"))
		}
		if item.Description != "" {
			fmt.Fprintf(w, "   Description: %s\n", item.Description)
		}
		if item.URL != "" {
			fmt.Fprintf(w, "   URL: %s\n", item.URL)
		}
	}
}

func main() {
	if err := runApp(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}
