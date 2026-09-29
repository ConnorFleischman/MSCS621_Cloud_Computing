package main_test

import (
	"testing"

	"go-mongo-docker/newsapi"
)

func TestProcessCachedRequestsAcceptsBatchRequests(t *testing.T) {
	requests := []newsapi.Request{
		{Topic: "cloud computing", Days: 7, Limit: 3},
		{Topic: "machine learning", Days: 5, Limit: 2},
	}

	resultsCh, errCh := newsapi.ProcessCachedRequests(t.TempDir(), requests)
	for result := range resultsCh {
		if len(result.Articles) != 0 && result.Request.Topic == "" {
			t.Fatalf("empty request topic in result")
		}
	}
	for err := range errCh {
		if err == nil {
			t.Fatal("unexpected nil error from batch processing")
		}
	}
}
