// mongo_test.go checks MongoDB persistence, indexes, topic normalization, and cache reuse with fake news.
package newsapi

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// TestMongoCache verifies storage against an isolated database when MONGO_TEST_URI is supplied.
func TestMongoCache(t *testing.T) {
	// Go runs this test from tests/newsapi, one directory below tests/.env.
	if err := LoadEnvFile("../.env"); err != nil {
		t.Fatal(err)
	}
	uri := os.Getenv("MONGO_TEST_URI")
	if uri == "" {
		t.Skip("set MONGO_TEST_URI in tests/.env or the environment to run MongoDB integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect(ctx)

	db := client.Database(fmt.Sprintf("news_test_%d", time.Now().UnixNano()))
	defer db.Drop(ctx)

	if err = indexMongoCache(ctx, db); err != nil {
		t.Fatal(err)
	}

	now, calls := time.Now().UTC(), 0
	req := Request{Topic: "  Cloud   COMPUTING ", Days: 2, Limit: 2}
	fetch := func(Request) ([]Article, error) {
		calls++
		a := Article{URL: "https://example.com/a", Title: "A", PublishedAt: &now}
		return []Article{a, a, {Title: "No URL", PublishedAt: &now}}, nil
	}

	for _, days := range []int{2, 7, 7} {
		req.Days = days
		items, err := fetchCachedArticles("", req, now, fetch, db)
		if err != nil || len(items) != 2 {
			t.Fatalf("days=%d: articles=%d, err=%v", days, len(items), err)
		}
		req.Topic = "cloud computing"
	}

	if calls != 2 {
		t.Fatalf("got %d fetches, want 2", calls)
	}

	filter := bson.M{"topic": "cloud computing", "country": "us"}
	if n, err := db.Collection("articles").CountDocuments(ctx, filter); err != nil || n != 2 {
		t.Fatalf("unique articles=%d, err=%v", n, err)
	}

	var stored bson.M
	if err = db.Collection("articles").FindOne(ctx, bson.M{"url": "https://example.com/a"}).Decode(&stored); err != nil {
		t.Fatal(err)
	}
	if stored["title"] != "A" || stored["publishedAt"] == nil {
		t.Fatal("article data missing")
	}
	delete(stored, "_id")
	if _, err = db.Collection("articles").InsertOne(ctx, stored); !mongo.IsDuplicateKeyError(err) {
		t.Fatalf("unique index did not reject duplicate: %v", err)
	}
	
	from := now.UTC().Truncate(24*time.Hour).AddDate(0, 0, -6)
	to := now.UTC().Truncate(24*time.Hour).AddDate(0, 0, 1)
	filter["from"], filter["to"] = from, to
	if n, err := db.Collection("coverage").CountDocuments(ctx, filter); err != nil || n != 1 {
		t.Fatalf("coverage=%d, err=%v", n, err)
	}
	cursor, err := db.Collection("articles").Find(ctx, bson.M{"topic": "cloud computing", "publishedAt": bson.M{"$gte": from}}, options.Find().SetHint(bson.D{{Key: "topic", Value: 1}, {Key: "publishedAt", Value: -1}}))
	if err != nil {
		t.Fatalf("topic/date index unavailable: %v", err)
	}
	cursor.Close(ctx)
	empty := Request{Topic: "empty", Country: "us"}
	if err = saveMongoCache(db, empty, articleCache{From: from, To: to}); err != nil {
		t.Fatal(err)
	}
	cache, err := loadMongoCache(db, empty, from, to)
	if err != nil || !cache.From.Equal(from) || !cache.To.Equal(to) || len(cache.Articles) != 0 {
		t.Fatalf("empty coverage was not preserved: %+v, %v", cache, err)
	}
}
