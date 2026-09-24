package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"go-mongo-docker/tests/newsapi"
)

func main() {
	if err := newsapi.LoadEnvFile(".env"); err != nil {
		log.Printf("warning: could not load .env: %v", err)
	}

	query := flag.String("q", "", "search keyword/topic (legacy alias)")
	topic := flag.String("topic", "technology", "topic or keyword to search")
	country := flag.String("country", "us", "country code")
	days := flag.Int("days", 1, "number of days to look back, including today")
	articles := flag.Int("articles", 1, "number of articles to request")
	outputDir := flag.String("output", "", "folder to save article JSON documents")
	flag.Parse()

	if *query != "" && *topic == "technology" {
		*topic = *query
	}
	if *days < 1 {
		log.Fatal("days must be at least 1")
	}
	if *articles < 1 {
		log.Fatal("articles must be at least 1")
	}

	apiKey := strings.TrimSpace(os.Getenv("NEWSAPI_API_KEY"))
	if apiKey == "" {
		log.Fatal("NEWSAPI_API_KEY is not set. Add it to .env or export it in your shell before running this command.")
	}

	request := newsapi.Request{
		APIKey: apiKey,
		Topic:  *topic,
		Country: *country,
		Days:   *days,
		Limit:  *articles,
	}

	items, err := newsapi.FetchArticles(request)
	if err != nil {
		log.Fatal(err)
	}
	if len(items) == 0 {
		fmt.Println("No articles returned for this query.")
		return
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
			log.Fatalf("save articles failed: %v", err)
		}
		fmt.Printf("\nSaved article documents to %s\n", *outputDir)
	}
}
