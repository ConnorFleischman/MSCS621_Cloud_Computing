package newsapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/mattn/go-sqlite3"
)

// Short native waits let context deadlines interrupt contention between processes.
// Every connection gets these settings, including connections reopened by database/sql.
func openSQLite(path string, timeout time.Duration) (*sql.DB, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}
	// Windows drive paths need file:///C:/..., not a URI authority named C:.
	if u.Path[0] != '/' {
		u.Path = "/" + u.Path
	}
	q := u.Query()
	q.Set("_busy_timeout", "50")
	q.Set("_journal_mode", "WAL")
	q.Set("_synchronous", "FULL")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite3", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	err = retrySQLite(ctx, func() error {
		var version int
		if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
			return err
		}
		if version == 1 {
			return nil
		}
		if version != 0 {
			return fmt.Errorf("unsupported SQLite schema version %d", version)
		}
		_, err := db.ExecContext(ctx, `
			BEGIN IMMEDIATE;
			CREATE TABLE IF NOT EXISTS articles (
			 topic TEXT NOT NULL, country TEXT NOT NULL, article_key TEXT NOT NULL,
			 published_at INTEGER, article_json TEXT NOT NULL,
			 PRIMARY KEY(topic, country, article_key));
			CREATE INDEX IF NOT EXISTS articles_search ON articles(topic, country, published_at DESC);
			CREATE TABLE IF NOT EXISTS coverage (
			 topic TEXT NOT NULL, country TEXT NOT NULL, from_time INTEGER NOT NULL,
			 to_time INTEGER NOT NULL, fetched_at INTEGER NOT NULL,
			 exhausted INTEGER NOT NULL, requested_limit INTEGER NOT NULL,
			 PRIMARY KEY(topic, country, from_time, to_time));
			CREATE INDEX IF NOT EXISTS coverage_search ON coverage(topic, country, fetched_at DESC);
			PRAGMA user_version = 1;
			COMMIT;`)
		if err != nil {
			_, _ = db.ExecContext(context.Background(), "ROLLBACK")
		}
		return err
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize SQLite: %w", err)
	}
	return db, nil
}

func retrySQLite(ctx context.Context, operation func() error) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := operation()
		var sqliteErr sqlite3.Error
		if !errors.As(err, &sqliteErr) || (sqliteErr.Code != sqlite3.ErrBusy && sqliteErr.Code != sqlite3.ErrLocked) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func fetchSQLiteArticles(path string, req Request) ([]Article, bool, error) {
	req = NormalizeRequestTimeouts(req)
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, false, err
	}
	db, err := openSQLite(abs, req.DBTimeout)
	if err != nil {
		return nil, false, err
	}
	defer db.Close()
	return fetchCachedArticles("sqlite:"+abs, req, time.Now().UTC(), FetchArticles, db)
}

func loadSQLiteCache(db *sql.DB, req Request, from, to time.Time) (articleCache, error) {
	req = NormalizeRequestTimeouts(req)
	ctx, cancel := context.WithTimeout(context.Background(), req.DBTimeout)
	defer cancel()
	var cache articleCache
	err := retrySQLite(ctx, func() error {
		cache = articleCache{}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		var start, end, fetched int64
		err = tx.QueryRowContext(ctx, `SELECT from_time, to_time, fetched_at, exhausted, requested_limit
		 FROM coverage WHERE topic=? AND country=? AND from_time<=? AND to_time>=?
		 ORDER BY fetched_at DESC LIMIT 1`, req.Topic, req.Country, from.UnixMilli(), to.UnixMilli()).Scan(
			&start, &end, &fetched, &cache.Exhausted, &cache.RequestedLimit)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil {
			cache.From, cache.To, cache.FetchedAt = time.UnixMilli(start).UTC(), time.UnixMilli(end).UTC(), time.UnixMilli(fetched).UTC()
		}
		rows, err := tx.QueryContext(ctx, `SELECT article_json FROM articles
		 WHERE topic=? AND country=? AND published_at>=? AND published_at<?
		 ORDER BY published_at DESC, article_key LIMIT ?`, req.Topic, req.Country, from.UnixMilli(), to.UnixMilli(), max(1, req.Limit))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var data string
			var article Article
			if err := rows.Scan(&data); err != nil {
				return err
			}
			if err := json.Unmarshal([]byte(data), &article); err != nil {
				return err
			}
			cache.Articles = append(cache.Articles, article)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		return tx.Commit()
	})
	return cache, err
}

// Publish articles and their coverage together. No transaction spans an HTTP request.
func saveSQLiteCache(db *sql.DB, req Request, cache articleCache) error {
	req = NormalizeRequestTimeouts(req)
	ctx, cancel := context.WithTimeout(context.Background(), req.DBTimeout)
	defer cancel()
	return retrySQLite(ctx, func() error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		for _, article := range cache.Articles {
			var published any
			if article.PublishedAt != nil {
				published = article.PublishedAt.UnixMilli()
			}
			data, err := json.Marshal(article)
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO articles(topic,country,article_key,published_at,article_json)
			 VALUES(?,?,?,?,?) ON CONFLICT(topic,country,article_key) DO UPDATE SET
			 published_at=excluded.published_at, article_json=excluded.article_json`,
				req.Topic, req.Country, articleKey(article), published, string(data))
			if err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO coverage(topic,country,from_time,to_time,fetched_at,exhausted,requested_limit)
		 VALUES(?,?,?,?,?,?,?) ON CONFLICT(topic,country,from_time,to_time) DO UPDATE SET
		 fetched_at=excluded.fetched_at, exhausted=excluded.exhausted, requested_limit=excluded.requested_limit`,
			req.Topic, req.Country, cache.From.UnixMilli(), cache.To.UnixMilli(), cache.FetchedAt.UnixMilli(), cache.Exhausted, cache.RequestedLimit)
		if err != nil {
			return err
		}
		return tx.Commit()
	})
}
