package newsapi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sqliteFixture() (Request, articleCache) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	return Request{Topic: "test", Country: "us", Limit: 10}, articleCache{
		From: now.Add(-24 * time.Hour), To: now.Add(time.Hour), FetchedAt: now,
		Exhausted: true, RequestedLimit: 10,
		Articles: []Article{{Title: "without URL", PublishedAt: &now}, {Title: "without date", URL: "https://example.test/nodate"}},
	}
}

func TestSQLitePersistenceIsolationAndRollback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "news.db")
	db, err := openSQLite(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	req, cache := sqliteFixture()
	if err := saveSQLiteCache(db, req, cache); err != nil {
		t.Fatal(err)
	}
	if err := saveSQLiteCache(db, req, cache); err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = openSQLite(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, err := loadSQLiteCache(db, req, cache.From, cache.To)
	if err != nil || len(got.Articles) != 1 || !got.Exhausted || got.RequestedLimit != 10 {
		t.Fatalf("round trip: %+v, %v", got, err)
	}
	if articleKey(got.Articles[0]) != articleKey(cache.Articles[0]) {
		t.Fatal("content identity changed after persistence")
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM articles").Scan(&count); err != nil || count != 2 {
		t.Fatalf("deduplication/nullable date: %d, %v", count, err)
	}
	other := req
	other.Country = "gb"
	got, err = loadSQLiteCache(db, other, cache.From, cache.To)
	if err != nil || len(got.Articles) != 0 || !got.From.IsZero() {
		t.Fatalf("country leaked: %+v, %v", got, err)
	}
	// Force coverage to fail after article inserts. Neither may commit.
	_, err = db.Exec(`CREATE TRIGGER reject_coverage BEFORE INSERT ON coverage BEGIN SELECT RAISE(ABORT, 'test failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	if err := saveSQLiteCache(db, other, cache); err == nil {
		t.Fatal("expected transaction failure")
	}
	if err := db.QueryRow("SELECT count(*) FROM articles WHERE country='gb'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial transaction: %d, %v", count, err)
	}
	if _, err := db.Exec("DROP TRIGGER reject_coverage"); err != nil {
		t.Fatal(err)
	}
	if err := saveSQLiteCache(db, other, cache); err != nil {
		t.Fatalf("retry after rollback: %v", err)
	}
}

func TestSQLiteBusyDeadlineAndSchemaVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "news.db")
	db, err := openSQLite(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	other, err := openSQLite(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, err := db.Exec("BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	req, cache := sqliteFixture()
	req.DBTimeout = 100 * time.Millisecond
	start := time.Now()
	err = saveSQLiteCache(other, req, cache)
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second {
		t.Fatalf("lock timeout: %v after %s", err, time.Since(start))
	}
	if _, err := db.Exec("ROLLBACK"); err != nil {
		t.Fatal(err)
	}
	if err := saveSQLiteCache(other, req, cache); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA user_version=99"); err != nil {
		t.Fatal(err)
	}
	if unexpected, err := openSQLite(path, time.Second); err == nil {
		unexpected.Close()
		t.Fatal("accepted unknown schema")
	}
}

func TestSQLiteContentKeyTimestampNormalization(t *testing.T) {
	_, cache := sqliteFixture()
	first := cache.Articles[0]
	stamp := first.PublishedAt.In(time.FixedZone("offset", 3600)).Add(999 * time.Microsecond)
	second := first
	second.PublishedAt = &stamp
	if articleKey(first) != articleKey(second) {
		t.Fatal("timezone or sub-millisecond precision changed identity")
	}
}

func TestSQLiteProcesses(t *testing.T) {
	if path := os.Getenv("SQLITE_TEST_CHILD_PATH"); path != "" {
		db, err := openSQLite(path, 10*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		if marker := os.Getenv("SQLITE_TEST_CRASH_MARKER"); marker != "" {
			if _, err := db.Exec("BEGIN IMMEDIATE; DELETE FROM articles;"); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(marker, []byte("ready"), 0o600); err != nil {
				t.Fatal(err)
			}
			time.Sleep(30 * time.Second)
			return
		}
		req, cache := sqliteFixture()
		cache.Articles = []Article{{URL: "shared", Title: "shared"}, {URL: os.Getenv("SQLITE_TEST_CHILD_ID"), Title: "unique"}}
		if err := saveSQLiteCache(db, req, cache); err != nil {
			t.Fatal(err)
		}
		return
	}
	path := filepath.Join(t.TempDir(), "news.db")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	commands := make([]*exec.Cmd, 6)
	for i := range commands {
		cmd := exec.Command(exe, "-test.run=^TestSQLiteProcesses$", "-test.timeout=20s")
		cmd.Env = append(os.Environ(), "SQLITE_TEST_CHILD_PATH="+path, fmt.Sprintf("SQLITE_TEST_CHILD_ID=%d", i))
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		commands[i] = cmd
	}
	for _, cmd := range commands {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("concurrent writer: %v", err)
		}
	}
	db, err := openSQLite(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow("SELECT count(*) FROM articles").Scan(&count); err != nil || count != 7 {
		t.Fatalf("concurrent records: %d, %v", count, err)
	}
	marker := path + ".ready"
	cmd := exec.Command(exe, "-test.run=^TestSQLiteProcesses$")
	cmd.Env = append(os.Environ(), "SQLITE_TEST_CHILD_PATH="+path, "SQLITE_TEST_CRASH_MARKER="+marker)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("writer did not start transaction")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	if err := db.QueryRow("SELECT count(*) FROM articles").Scan(&count); err != nil || count != 7 {
		t.Fatalf("committed data lost after crash: %d, %v", count, err)
	}
	var integrity string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("integrity: %s, %v", integrity, err)
	}
}

func TestLegacyMongoConfiguration(t *testing.T) {
	t.Setenv("DATABASE_PATH", "")
	t.Setenv("MONGO_URI", "mongodb://legacy")
	_, _, err := FetchCachedArticlesWithSource(t.TempDir(), Request{})
	if err == nil || !strings.Contains(err.Error(), "DATABASE_PATH") {
		t.Fatalf("missing migration guidance: %v", err)
	}
}
