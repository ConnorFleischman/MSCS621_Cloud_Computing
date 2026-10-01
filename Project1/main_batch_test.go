package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go-mongo-docker/newsapi"
)

func TestBatchCLIOverlapsSearchesAndKeepsOutputsSeparate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "searches.json")
	if err := os.WriteFile(path, []byte(`[{"topic":"alpha","days":1,"articles":1},{"topic":"beta","days":2,"articles":2}]`), 0600); err != nil {
		t.Fatal(err)
	}
	old := fetchArticles
	defer func() { fetchArticles = old }()
	entered := make(chan struct{}, 2)
	fetchArticles = func(_ string, req newsapi.Request) ([]newsapi.Article, bool, error) {
		entered <- struct{}{}
		deadline := time.After(3 * time.Second)
		for len(entered) < 2 {
			select {
			case <-deadline:
				return nil, false, fmt.Errorf("batch searches did not overlap")
			default:
				time.Sleep(time.Millisecond)
			}
		}
		return []newsapi.Article{{Title: req.Topic}}, true, nil
	}
	out := filepath.Join(dir, "output")
	if err := runApp([]string{"-batch", path, "-output", out}); err != nil {
		t.Fatal(err)
	}
	for i, topic := range []string{"alpha", "beta"} {
		data, err := os.ReadFile(filepath.Join(out, fmt.Sprintf("search_%03d", i+1), "articles.json"))
		if err != nil || !strings.Contains(string(data), topic) {
			t.Fatalf("output %s: %s, %v", topic, data, err)
		}
	}
}

func TestBatchRejectsInvalidInputBeforeFetching(t *testing.T) {
	for _, input := range []string{`[]`, `null`, `[{"topic":"x","days":0,"articles":1}]`, `[{"topic":"x","days":1,"articles":1,"typo":2}]`, `[] []`} {
		path := filepath.Join(t.TempDir(), "input.json")
		if err := os.WriteFile(path, []byte(input), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := readBatch(path, newsapi.Request{}); err == nil {
			t.Errorf("accepted %s", input)
		}
	}
}

func TestBatchReportsFailureAndPreservesSuccessfulOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "input.json")
	if err := os.WriteFile(path, []byte(`[{"topic":"ok","days":1,"articles":1},{"topic":"bad","days":1,"articles":1}]`), 0600); err != nil {
		t.Fatal(err)
	}
	old := fetchArticles
	defer func() { fetchArticles = old }()
	fetchArticles = func(_ string, req newsapi.Request) ([]newsapi.Article, bool, error) {
		if req.Topic == "bad" {
			return nil, false, fmt.Errorf("unavailable")
		}
		return []newsapi.Article{{Title: "ok"}}, false, nil
	}
	err := runApp([]string{"-batch", path, "-output", dir})
	if err == nil || !strings.Contains(err.Error(), `request 2 ("bad")`) {
		t.Fatalf("missing request identity: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "search_001", "articles.json")); err != nil {
		t.Fatal(err)
	}
}
