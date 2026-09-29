package newsapi_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go-mongo-docker/newsapi"
)

func TestNormalizeRequestTimeoutsAppliesDefaults(t *testing.T) {
	req := newsapi.Request{}
	got := newsapi.NormalizeRequestTimeouts(req)
	if got.Timeout <= 0 {
		t.Fatal("expected API timeout default to be > 0")
	}
	if got.MongoTimeout <= 0 {
		t.Fatal("expected Mongo timeout default to be > 0")
	}
}

func TestFormatTimeoutErrorIsUserFriendly(t *testing.T) {
	err := newsapi.FormatTimeoutError("NewsAPI", 250*time.Millisecond, context.DeadlineExceeded)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "NewsAPI") {
		t.Fatalf("expected NewsAPI in timeout message, got %q", msg)
	}
	if !strings.Contains(msg, "250ms") {
		t.Fatalf("expected timeout duration in message, got %q", msg)
	}
	if !strings.Contains(msg, "timed out") {
		t.Fatalf("expected timeout wording in message, got %q", msg)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("expected wrapped deadline exceeded error")
	}
}
