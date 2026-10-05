package main

import (
	"testing"
	"time"

	"go-mongo-docker/newsapi"
)

func TestDatabaseTimeoutConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name            string
		args            []string
		current, legacy string
		want            time.Duration
	}{
		{"default", nil, "", "", 10 * time.Second},
		{"legacy flag", []string{"-mongo-timeout", "2s"}, "", "", 2 * time.Second},
		{"new flag wins", []string{"-db-timeout", "3s", "-mongo-timeout", "2s"}, "", "4s", 3 * time.Second},
		{"new environment wins", []string{"-db-timeout", "3s"}, "5s", "4s", 5 * time.Second},
		{"legacy environment", []string{"-mongo-timeout", "2s"}, "", "4s", 4 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DB_TIMEOUT", tc.current)
			t.Setenv("MONGO_TIMEOUT", tc.legacy)
			old := fetchArticles
			defer func() { fetchArticles = old }()
			fetchArticles = func(_ string, req newsapi.Request) ([]newsapi.Article, bool, error) {
				if req.DBTimeout != tc.want {
					t.Errorf("timeout=%s, want %s", req.DBTimeout, tc.want)
				}
				return nil, false, nil
			}
			if err := runApp(tc.args); err != nil {
				t.Fatal(err)
			}
		})
	}
}
