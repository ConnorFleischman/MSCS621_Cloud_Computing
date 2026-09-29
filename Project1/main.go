package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"go-mongo-docker/newsapi"
)

var fetchArticles = newsapi.FetchCachedArticles

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

	items, err := fetchArticles(*cacheDir, request)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		fmt.Println("No articles returned for this query.")
		return nil
	}

	fmt.Printf("Fetched %d article(s)\n", len(items))
	for i, item := range items {
		fmt.Printf("\n%d. %s\n", i+1, item.Title)
		if item.Source.Name != "" {
			fmt.Printf("   Source: %s\n", item.Source.Name)
		}
		if item.Description != "" {
			fmt.Printf("   %s\n", item.Description)
		}
		if item.URL != "" {
			fmt.Printf("   %s\n", item.URL)
		}
	}

	if *outputDir != "" {
		if err := newsapi.SaveArticles(*outputDir, items); err != nil {
			return fmt.Errorf("save articles failed: %w", err)
		}
		fmt.Printf("\nSaved article documents to %s\n", *outputDir)
	}

	return nil
}

func main() {
	if err := runApp(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}
