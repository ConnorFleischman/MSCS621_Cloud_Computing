// news.go holds the data structures for news articles and API responses.
package models

import (
	"time"
)

type NewsAPIResponse struct {
	Status       string    `json:"status"`
	TotalResults int       `json:"totalResults"`
	Articles     []Article `json:"articles"`
	Code         string    `json:"code,omitempty"`
	Message      string    `json:"message,omitempty"`
}

type Source struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// Missing or null text decodes to empty strings; absent dates remain nil.
type Article struct {
	Title       string     `json:"title"`
	Author      string     `json:"author,omitempty"`
	Description string     `json:"description,omitempty"`
	URL         string     `json:"url"`
	Source      Source     `json:"source"`
	PublishedAt *time.Time `json:"publishedAt,omitempty"`
}
