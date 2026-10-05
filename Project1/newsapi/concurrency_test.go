package newsapi_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go-mongo-docker/newsapi"
)

func TestProcessRequestsReturnsResultsAndErrors(t *testing.T) {
	requests := []newsapi.Request{
		{Topic: "alpha", Days: 1, Limit: 2},
		{Topic: "beta", Days: 1, Limit: 2},
		{Topic: "gamma", Days: 1, Limit: 2},
	}

	fetch := func(req newsapi.Request) ([]newsapi.Article, error) {
		if req.Topic == "beta" {
			return nil, errors.New("beta failed")
		}
		return []newsapi.Article{{Title: req.Topic}}, nil
	}

	resultsCh, errCh := newsapi.ProcessRequests(requests, fetch)

	results := make(map[string][]newsapi.Article)
	var errs []string
	for result := range resultsCh {
		results[result.Request.Topic] = result.Articles
	}
	for err := range errCh {
		errs = append(errs, err.Error())
	}
	sort.Strings(errs)

	if len(results) != 2 {
		t.Fatalf("expected 2 successful results, got %d", len(results))
	}
	if _, ok := results["alpha"]; !ok {
		t.Fatalf("expected alpha result to be processed")
	}
	if _, ok := results["gamma"]; !ok {
		t.Fatalf("expected gamma result to be processed")
	}
	if len(errs) != 1 || errs[0] != "request 2 (\"beta\"): beta failed" {
		t.Fatalf("expected a single beta error, got %#v", errs)
	}
}

func TestConcurrentRequestsFixture(t *testing.T) {
	input, err := os.ReadFile(filepath.Join("testdata", "concurrent-case.json"))
	if err != nil {
		t.Fatalf("read concurrent request fixture: %v", err)
	}
	type fixtureResult struct {
		Topic  string   `json:"topic"`
		Source string   `json:"source"`
		Titles []string `json:"titles"`
	}
	var fixture struct {
		Requests []struct {
			Topic    string `json:"topic"`
			Days     int    `json:"days"`
			Articles int    `json:"articles"`
		} `json:"requests"`
		Expected struct {
			MaxConcurrent int             `json:"max_concurrent"`
			Results       []fixtureResult `json:"results"`
		} `json:"expected"`
	}
	if err := json.Unmarshal(input, &fixture); err != nil {
		t.Fatalf("decode concurrent request fixture: %v", err)
	}
	requests := make([]newsapi.Request, len(fixture.Requests))
	for i, entry := range fixture.Requests {
		requests[i] = newsapi.Request{Topic: entry.Topic, Days: entry.Days, Limit: entry.Articles}
	}
	if len(requests) < 2 {
		t.Fatal("concurrency fixture must contain at least two requests")
	}

	entered := make(chan struct{}, len(requests))
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseAll := func() { releaseOnce.Do(func() { close(release) }) }
	var active, maxActive atomic.Int32
	fetch := func(req newsapi.Request) ([]newsapi.Article, bool, error) {
		current := active.Add(1)
		for previous := maxActive.Load(); current > previous && !maxActive.CompareAndSwap(previous, current); previous = maxActive.Load() {
		}
		entered <- struct{}{}
		<-release
		active.Add(-1)
		return []newsapi.Article{{Title: req.Topic + " result"}}, true, nil
	}
	resultsCh, errorsCh := newsapi.ProcessRequestsWithSource(requests, fetch)
	defer releaseAll()
	for range requests {
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			t.Fatal("requests did not overlap")
		}
	}
	releaseAll()

	results := make([]fixtureResult, 0, len(requests))
	for result := range resultsCh {
		source := "cache"
		if result.FromAPI {
			source = "News API"
		}
		results = append(results, fixtureResult{Topic: result.Request.Topic, Source: source, Titles: []string{result.Articles[0].Title}})
	}
	for err := range errorsCh {
		t.Errorf("concurrent request failed: %v", err)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Topic < results[j].Topic })
	got := struct {
		MaxConcurrent int             `json:"max_concurrent"`
		Results       []fixtureResult `json:"results"`
	}{int(maxActive.Load()), results}
	if !reflect.DeepEqual(got, fixture.Expected) {
		t.Fatalf("concurrent results = %+v, want %+v", got, fixture.Expected)
	}
}
