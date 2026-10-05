package main

import (
	"bytes"
	"os"
	"path/filepath"
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

func TestPrintArticlesGoldenOutput(t *testing.T) {
	published := time.Date(2026, time.October, 1, 10, 0, 0, 0, time.UTC)
	items := []newsapi.Article{{
		Title:       "Fixture cache article",
		Description: "Fixture summary",
		URL:         "https://example.com/fixture",
		PublishedAt: &published,
	}}
	items[0].Source.Name = "Fixture News"

	for _, test := range []struct {
		name    string
		fromAPI bool
	}{
		{name: "cache-hit.txt"},
		{name: "cache-miss.txt", fromAPI: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			printArticles(&output, items, 1, test.fromAPI)
			want, err := os.ReadFile(filepath.Join("testdata", "expected", test.name))
			if err != nil {
				t.Fatalf("read expected output: %v", err)
			}
			gotText := strings.ReplaceAll(output.String(), "\r\n", "\n")
			wantText := strings.ReplaceAll(string(want), "\r\n", "\n")
			if strings.TrimSuffix(gotText, "\n") != strings.TrimSuffix(wantText, "\n") {
				t.Fatalf("output differs from %s:\n got: %q\nwant: %q", test.name, output.String(), string(want))
			}
		})
	}
}
