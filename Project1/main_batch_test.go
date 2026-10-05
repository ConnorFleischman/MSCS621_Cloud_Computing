package main

import (
	"encoding/json"
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
	for _, input := range []string{`[]`, `null`, `[{"topic":"x","days":0,"articles":1}]`, `[{"topic":"x","days":1,"articles":0}]`, `[{"days":1,"articles":1}]`, `[{"topic":"x","days":1,"articles":1,"typo":2}]`, `[] []`} {
		path := filepath.Join(t.TempDir(), "input.json")
		if err := os.WriteFile(path, []byte(input), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := readBatch(path, newsapi.Request{}); err == nil {
			t.Errorf("accepted %s", input)
		}
	}
}

func TestInvalidBatchFixture(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "invalid-batch-case.json"))
	if err != nil {
		t.Fatalf("read invalid batch fixture: %v", err)
	}
	var fixture struct {
		Input         json.RawMessage `json:"input"`
		ExpectedError string          `json:"expected_error"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode invalid batch fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "invalid-batch.json")
	if err := os.WriteFile(path, fixture.Input, 0600); err != nil {
		t.Fatalf("write invalid batch input: %v", err)
	}
	_, err = readBatch(path, newsapi.Request{})
	if err == nil {
		t.Fatal("invalid batch fixture was accepted")
	}
	if got, want := err.Error(), fixture.ExpectedError; got != want {
		t.Fatalf("validation error = %q, want %q", got, want)
	}
}

func TestCLIRejectsInvalidInputBeforeFetching(t *testing.T) {
	old := fetchArticles
	defer func() { fetchArticles = old }()
	fetched := false
	fetchArticles = func(string, newsapi.Request) ([]newsapi.Article, bool, error) {
		fetched = true
		return nil, false, nil
	}

	for _, args := range [][]string{
		{"-topic", "cloud computing", "-days", "0", "-articles", "1"},
		{"-topic", "cloud computing", "-days", "1", "-articles", "0"},
		{"-topic", "cloud computing", "-days", "invalid", "-articles", "1"},
	} {
		fetched = false
		if err := runApp(args); err == nil {
			t.Errorf("runApp(%v) accepted invalid input", args)
		}
		if fetched {
			t.Errorf("runApp(%v) fetched articles before rejecting input", args)
		}
	}
}

func TestCLIValidInputReachesFetcher(t *testing.T) {
	old := fetchArticles
	defer func() { fetchArticles = old }()
	var got newsapi.Request
	fetchArticles = func(_ string, req newsapi.Request) ([]newsapi.Article, bool, error) {
		got = req
		return nil, false, nil
	}

	if err := runApp([]string{"-topic", "cloud computing", "-days", "2", "-articles", "3"}); err != nil {
		t.Fatalf("runApp returned error for valid input: %v", err)
	}
	if got.Topic != "cloud computing" || got.Days != 2 || got.Limit != 3 {
		t.Fatalf("fetch request = topic %q, days %d, limit %d", got.Topic, got.Days, got.Limit)
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
