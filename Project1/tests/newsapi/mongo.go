// mongo.go stores query coverage and deduplicated articles in an indexed MongoDB cache.
package newsapi

import (
	"context"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// fetchMongoArticles connects the CLI to MongoDB, using the same coverage checks as the file cache.
func fetchMongoArticles(uri string, req Request) ([]Article, error) {
	// Connect to MongoDB with a timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Connect to the MongoDB server
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	defer client.Disconnect(context.Background())

	// Determine the database name from the .env or default to "news"
	name := os.Getenv("MONGO_DATABASE")
	if name == "" {
		name = "news"
	}

	// Make sure indexes are created for the database
	db := client.Database(name)
	if err = indexMongoCache(ctx, db); err != nil {
		return nil, err
	}

	return fetchCachedArticles("", req, time.Now().UTC(), FetchArticles, db)
}

// indexMongoCache enforces article identity and indexes topic/date searches and coverage lookups.
func indexMongoCache(ctx context.Context, db *mongo.Database) error {
	// Create indexes for articles collection
	_, err := db.Collection("articles").Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "topic", Value: 1}, {Key: "country", Value: 1}, {Key: "key", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "topic", Value: 1}, {Key: "publishedAt", Value: -1}}},
	})
	if err != nil {
		return err
	}

	// Create indexes for coverage collection
	_, err = db.Collection("coverage").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "topic", Value: 1}, {Key: "country", Value: 1}, {Key: "from", Value: 1}, {Key: "to", Value: 1}}, Options: options.Index().SetUnique(true),
	})
	return err
}

// loadMongoCache reads matching articles and a completed interval covering the requested UTC dates.
func loadMongoCache(db *mongo.Database, req Request, from, to time.Time) (articleCache, error) {
	// Query the coverage collection for a matching interval
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Initialize an empty articleCache
	var cache articleCache
	filter := bson.M{"topic": req.Topic, "country": req.Country, "from": bson.M{"$lte": from}, "to": bson.M{"$gte": to}}
	err := db.Collection("coverage").FindOne(ctx, filter).Decode(&cache)
	if err != nil && err != mongo.ErrNoDocuments {
		return cache, err
	}

	// Read only the requested number of matching articles, newest first.
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
	// Insert/update articles and coverage in the MongoDB database
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Insert/update each article in the articles collection
	for _, article := range cache.Articles {
		key := articleKey(article)
		filter := bson.M{"topic": req.Topic, "country": req.Country, "key": key}

		doc := struct {
			Article             `bson:",inline"`
			Topic, Country, Key string
		}{article, req.Topic, req.Country, key}

		if _, err := db.Collection("articles").UpdateOne(ctx, filter, bson.M{"$set": doc}, options.Update().SetUpsert(true)); err != nil {
			return err
		}
	}

	// Insert/update the coverage document in the coverage collection
	filter := bson.M{"topic": req.Topic, "country": req.Country, "from": cache.From, "to": cache.To}
	_, err := db.Collection("coverage").UpdateOne(ctx, filter, bson.M{"$set": filter}, options.Update().SetUpsert(true))

	return err
}
