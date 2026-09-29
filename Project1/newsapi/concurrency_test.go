package newsapi_test

import (
	"errors"
	"sort"
	"testing"

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
	if len(errs) != 1 || errs[0] != "beta failed" {
		t.Fatalf("expected a single beta error, got %#v", errs)
	}
}
