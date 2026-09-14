package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	// Use environment variable for the connection string, or default to localhost
	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		uri = "mongodb://admin:secret@localhost:27017/"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Connect to MongoDB
	clientOptions := options.Client().ApplyURI(uri)
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		log.Fatal("Connection failed: ", err)
	}
	defer client.Disconnect(ctx)

	// Verify the connection
	if err = client.Ping(ctx, nil); err != nil {
		log.Fatal("Ping failed: ", err)
	}
	fmt.Println("Successfully connected to MongoDB!")

	// Insert a test document
	collection := client.Database("testdb").Collection("users")
	doc := bson.M{"name": "Gopher", "role": "Developer", "created_at": time.Now()}
	
	res, err := collection.InsertOne(ctx, doc)
	if err != nil {
		log.Fatal(err)
	}
	
	fmt.Printf("Inserted document with ID: %v\n", res.InsertedID)
}