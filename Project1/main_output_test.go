package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"go-mongo-docker/newsapi"
)

func TestPrintArticlesIncludesAvailableFieldsAndShortage(t *testing.T) {
	published := time.Date(2026, time.September, 29, 12, 30, 0, 0, time.UTC)
	items := []newsapi.Article{
		{Title: "Example", Author: "A. Writer", PublishedAt: &published, Description: "Summary", URL: "https://example.com"},
		{},
	}
	items[0].Source.Name = "Example News"

	var output bytes.Buffer
	printArticles(&output, items, 3, false)
	text := output.String()
	for _, expected := range []string{
		"Fetched 2 of 3 requested article(s)",
		"Results: cache",
		"Only 2 of 3 requested article(s) were available.",
		"1. Example",
		"Source: Example News",
		"Author: A. Writer",
		"Published: 2026-09-29 12:30 UTC",
		"Description: Summary",
		"URL: https://example.com",
		"2. (untitled)",
	} {
		if !strings.Contains(text, expected) {
			t.Errorf("output missing %q:\n%s", expected, text)
		}
	}
}
