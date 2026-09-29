// news.go holds the data structures for news articles and API responses, including MongoDB integration.
package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type NewsAPIResponse struct {
	Status       string    `json:"status" bson:"status"`
	TotalResults int       `json:"totalResults" bson:"totalResults"`
	Articles     []Article `json:"articles" bson:"articles"`
	Code         string    `json:"code,omitempty" bson:"code,omitempty"`
	Message      string    `json:"message,omitempty" bson:"message,omitempty"`
}

type Source struct {
	ID   string `json:"id,omitempty" bson:"id,omitempty"`
	Name string `json:"name,omitempty" bson:"name,omitempty"`
}

// Missing or null text decodes to empty strings; absent dates remain nil.
type Article struct {
	Title       string     `json:"title" bson:"title"`
	Author      string     `json:"author,omitempty" bson:"author,omitempty"`
	Description string     `json:"description,omitempty" bson:"description,omitempty"`
	URL         string     `json:"url" bson:"url"`
	Source      Source     `json:"source" bson:"source"`
	PublishedAt *time.Time `json:"publishedAt,omitempty" bson:"publishedAt,omitempty"`
}

// DateRange holds inclusive search bounds; nil means an unbounded endpoint.
type DateRange struct {
	From *time.Time `json:"from,omitempty" bson:"from,omitempty"`
	To   *time.Time `json:"to,omitempty" bson:"to,omitempty"`
}

type ArticleRecord struct {
	ID        bson.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	Article   `bson:",inline"`
	Topic     string    `json:"topic" bson:"topic"`
	DateRange DateRange `json:"dateRange" bson:"dateRange"`
}
