package newsapi

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// fetchMongoArticles connects the CLI to MongoDB, using the same coverage checks as the file cache.
func fetchMongoArticles(uri string, req Request) ([]Article, bool, error) {
	req = NormalizeRequestTimeouts(req)
	ctx, cancel := context.WithTimeout(context.Background(), req.MongoTimeout)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, false, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), req.MongoTimeout)
		defer cancel()
		_ = client.Disconnect(cleanup)
	}()

	name := os.Getenv("MONGO_DATABASE")
	if name == "" {
		name = "news"
	}

	db := client.Database(name)
	if err = indexMongoCache(ctx, db); err != nil {
		return nil, false, err
	}

	return fetchCachedArticles(fmt.Sprintf("mongo:%x:%s", sha256.Sum256([]byte(uri)), name), req, time.Now().UTC(), FetchArticles, db)
}

// indexMongoCache enforces article identity and indexes topic/date searches and coverage lookups.
func indexMongoCache(ctx context.Context, db *mongo.Database) error {
	_, err := db.Collection("articles").Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "topic", Value: 1}, {Key: "country", Value: 1}, {Key: "key", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "topic", Value: 1}, {Key: "publishedAt", Value: -1}}},
	})
	if err != nil {
		return err
	}

	_, err = db.Collection("coverage").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "topic", Value: 1}, {Key: "country", Value: 1}, {Key: "from", Value: 1}, {Key: "to", Value: 1}}, Options: options.Index().SetUnique(true),
	})
	return err
}

// loadMongoCache reads matching articles and a completed interval covering the requested UTC dates.
func loadMongoCache(db *mongo.Database, req Request, from, to time.Time) (articleCache, error) {
	req = NormalizeRequestTimeouts(req)
	ctx, cancel := context.WithTimeout(context.Background(), req.MongoTimeout)
	defer cancel()

	var cache articleCache
	filter := bson.M{"topic": req.Topic, "country": req.Country, "from": bson.M{"$lte": from}, "to": bson.M{"$gte": to}}
	err := db.Collection("coverage").FindOne(ctx, filter, options.FindOne().SetSort(bson.D{{Key: "fetchedat", Value: -1}})).Decode(&cache)
	if err != nil && err != mongo.ErrNoDocuments {
		return cache, err
	}

	filter = bson.M{"topic": req.Topic, "country": req.Country, "publishedAt": bson.M{"$gte": from, "$lt": to}}
	cursor, err := db.Collection("articles").Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "publishedAt", Value: -1}}).SetLimit(int64(max(1, req.Limit))))
	if err != nil {
		return cache, err
	}
	defer cursor.Close(ctx)

	err = cursor.All(ctx, &cache.Articles)
	return cache, err
}

// saveMongoCache upserts articles before recording coverage; absent URLs use a content hash for identity.
func saveMongoCache(db *mongo.Database, req Request, cache articleCache) error {
	req = NormalizeRequestTimeouts(req)
	ctx, cancel := context.WithTimeout(context.Background(), req.MongoTimeout)
	defer cancel()

	for _, article := range cache.Articles {
		key := articleKey(article)
		filter := bson.M{"topic": req.Topic, "country": req.Country, "key": key}

		doc := struct {
			Article             `bson:",inline"`
			Topic, Country, Key string
		}{article, req.Topic, req.Country, key}

		if err := upsertCacheDocument(ctx, db.Collection("articles"), filter, bson.M{"$set": doc}); err != nil {
			return err
		}
	}

	filter := bson.M{"topic": req.Topic, "country": req.Country, "from": cache.From, "to": cache.To}
	return upsertCacheDocument(ctx, db.Collection("coverage"), filter, bson.M{"$set": bson.M{"topic": req.Topic, "country": req.Country, "from": cache.From, "to": cache.To, "fetchedat": cache.FetchedAt, "exhausted": cache.Exhausted, "requestedlimit": cache.RequestedLimit}})
}

// Different date windows (or processes) can discover the same article at once.
// If another writer wins the unique-key insert, update that document instead.
func upsertCacheDocument(ctx context.Context, collection *mongo.Collection, filter, update bson.M) error {
	_, err := collection.UpdateOne(ctx, filter, update, options.UpdateOne().SetUpsert(true))
	if mongo.IsDuplicateKeyError(err) {
		_, err = collection.UpdateOne(ctx, filter, update)
	}
	return err
}
