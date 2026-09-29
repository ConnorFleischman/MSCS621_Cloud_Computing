package main_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-mongo-docker/newsapi"
)

func TestLoadEnvFileReadsValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("NEWSAPI_API_KEY=test-key\nMONGO_DATABASE=test-db\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	if err := os.Unsetenv("NEWSAPI_API_KEY"); err != nil {
		t.Fatalf("unset NEWSAPI_API_KEY: %v", err)
	}
	if err := os.Unsetenv("MONGO_DATABASE"); err != nil {
		t.Fatalf("unset MONGO_DATABASE: %v", err)
	}

	if err := newsapi.LoadEnvFile(path); err != nil {
		t.Fatalf("LoadEnvFile returned error: %v", err)
	}
	if got := os.Getenv("NEWSAPI_API_KEY"); got != "test-key" {
		t.Fatalf("NEWSAPI_API_KEY = %q, want %q", got, "test-key")
	}
	if got := os.Getenv("MONGO_DATABASE"); got != "test-db" {
		t.Fatalf("MONGO_DATABASE = %q, want %q", got, "test-db")
	}
}

func TestFetchArticlesRequiresAPIKey(t *testing.T) {
	_, err := newsapi.FetchArticles(newsapi.Request{Topic: "cloud computing", Days: 1, Limit: 1})
	if err == nil {
		t.Fatal("expected missing API key error")
	}
	if !strings.Contains(err.Error(), "NEWSAPI_API_KEY is not set") {
		t.Fatalf("expected API key error, got %v", err)
	}
}

func TestSaveArticlesWritesJSONFiles(t *testing.T) {
	dir := t.TempDir()
	items := []newsapi.Article{{
		Title: "Example article",
		URL:   "https://example.com/article",
	}}

	if err := newsapi.SaveArticles(dir, items); err != nil {
		t.Fatalf("SaveArticles returned error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "article_01.json")); err != nil {
		t.Fatalf("missing article file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "articles.json")); err != nil {
		t.Fatalf("missing aggregate file: %v", err)
	}
}
