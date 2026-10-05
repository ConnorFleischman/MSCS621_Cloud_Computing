// Command metrics measures the public SQLite-backed search path against a local mock API.
package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync/atomic"
	"time"

	"go-mongo-docker/newsapi"
)

type redirect struct{ target *url.URL }

func (r redirect) RoundTrip(req *http.Request) (*http.Response, error) {
	copy := req.Clone(req.Context())
	u := *req.URL
	u.Scheme, u.Host = r.target.Scheme, r.target.Host
	copy.URL, copy.Host = &u, r.target.Host
	return http.DefaultTransport.RoundTrip(copy)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	out := flag.String("output", "metrics/results", "artifact directory")
	flag.Parse()
	if err := os.MkdirAll(*out, 0755); err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "sqlite-metrics-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	var calls atomic.Int64
	now := time.Now().UTC()
	articles := make([]newsapi.Article, 5)
	for i := range articles {
		published := now.Add(-time.Duration(i+1) * time.Minute)
		articles[i] = newsapi.Article{Title: fmt.Sprintf("Mock article %d", i), URL: fmt.Sprintf("https://example.invalid/article/%d", i), PublishedAt: &published}
	}
	payload, err := json.Marshal(map[string]any{"status": "ok", "totalResults": 5, "articles": articles})
	if err != nil {
		return err
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	client := &http.Client{Transport: redirect{u}}
	base := newsapi.Request{APIKey: "metrics-placeholder", Country: "us", Days: 1, Limit: 5, HTTPClient: client, Timeout: 5 * time.Second, DBTimeout: 10 * time.Second}
	// Seed a persistent fixture for the actual release-image cache-hit smoke test.
	fixture := filepath.Join(*out, "runtime-fixture.db")
	if _, err := os.Stat(fixture); err == nil {
		return fmt.Errorf("fixture already exists: use a new output directory")
	}
	if err := os.Setenv("DATABASE_PATH", fixture); err != nil {
		return err
	}
	base.Topic = "runtime-smoke"
	if items, _, err := newsapi.FetchCachedArticlesWithSource("", base); err != nil || len(items) != 5 {
		return fmt.Errorf("seed runtime fixture: items=%d error=%v", len(items), err)
	}
	if err := os.Setenv("DATABASE_PATH", filepath.Join(dir, "news.db")); err != nil {
		return err
	}
	base.Topic = "warm"
	if _, _, err := newsapi.FetchCachedArticlesWithSource("", base); err != nil {
		return err
	}
	// The preceding fetch initializes the schema, HTTP connection, and warm query before timing.
	rawFile, err := os.Create(filepath.Join(*out, "operations.csv"))
	if err != nil {
		return err
	}
	defer rawFile.Close()
	raw := csv.NewWriter(rawFile)
	_ = raw.Write([]string{"scenario", "concurrency", "repetition", "wave", "index", "latency_ms", "from_api", "articles", "error"})
	summaryFile, err := os.Create(filepath.Join(*out, "benchmark_results.csv"))
	if err != nil {
		return err
	}
	defer summaryFile.Close()
	summary := csv.NewWriter(summaryFile)
	_ = summary.Write([]string{"scenario", "concurrency", "completed", "errors", "api_calls", "elapsed_seconds", "throughput_per_second", "p50_ms", "p95_ms", "p99_ms", "max_ms"})
	type scenario struct {
		name        string
		concurrency int
	}
	cases := []scenario{}
	for _, n := range []int{1, 2, 5, 10, 20} {
		cases = append(cases, scenario{"cache_hit", n})
	}
	cases = append(cases, scenario{"cache_miss", 1})
	for _, name := range []string{"unique_cold", "identical_cold"} {
		for _, n := range []int{1, 2, 5, 10, 20} {
			cases = append(cases, scenario{name, n})
		}
	}
	for _, sc := range cases {
		durations := []float64{}
		completed, failures := 0, 0
		var elapsed time.Duration
		var apiCalls int64
		for repetition := 0; repetition <= 5; repetition++ {
			// Repetition zero warms each scenario and is excluded from all reported metrics.
			for wave := 0; wave < 20/sc.concurrency; wave++ {
				requests := make([]newsapi.Request, sc.concurrency)
				for i := range requests {
					requests[i] = base
					if sc.name != "cache_hit" {
						requests[i].Topic = fmt.Sprintf("%s-%d-%d-%d", sc.name, sc.concurrency, repetition, wave)
						if sc.name != "identical_cold" {
							requests[i].Topic += fmt.Sprintf("-%d", i)
						}
					}
				}
				type operation struct {
					ms    float64
					api   bool
					count int
					err   error
				}
				ops := make(chan operation, sc.concurrency)
				before := calls.Load()
				start := time.Now()
				results, errs := newsapi.ProcessRequestsWithSource(requests, func(req newsapi.Request) ([]newsapi.Article, bool, error) {
					started := time.Now()
					items, api, err := newsapi.FetchCachedArticlesWithSource("", req)
					ops <- operation{float64(time.Since(started)) / float64(time.Millisecond), api, len(items), err}
					return items, api, err
				})
				for range results {
				}
				for range errs {
				}
				wall := time.Since(start)
				if repetition > 0 {
					elapsed += wall
					apiCalls += calls.Load() - before
				}
				for i := 0; i < sc.concurrency; i++ {
					op := <-ops
					if repetition == 0 {
						if op.err != nil || op.count != 5 {
							return fmt.Errorf("warmup %s: count=%d error=%v", sc.name, op.count, op.err)
						}
						continue
					}
					durations = append(durations, op.ms)
					errText := ""
					if op.err != nil {
						errText = op.err.Error()
						failures++
					} else if op.count != 5 {
						errText = "unexpected article count"
						failures++
					} else {
						completed++
					}
					_ = raw.Write([]string{sc.name, strconv.Itoa(sc.concurrency), strconv.Itoa(repetition), strconv.Itoa(wave), strconv.Itoa(i), fmt.Sprintf("%.6f", op.ms), strconv.FormatBool(op.api), strconv.Itoa(op.count), errText})
				}
			}
		}
		sort.Float64s(durations)
		percentile := func(p int) float64 { return durations[(len(durations)*p+99)/100-1] }
		row := []string{sc.name, strconv.Itoa(sc.concurrency), strconv.Itoa(completed), strconv.Itoa(failures), strconv.FormatInt(apiCalls, 10), fmt.Sprintf("%.6f", elapsed.Seconds()), fmt.Sprintf("%.3f", float64(completed)/elapsed.Seconds()), fmt.Sprintf("%.3f", percentile(50)), fmt.Sprintf("%.3f", percentile(95)), fmt.Sprintf("%.3f", percentile(99)), fmt.Sprintf("%.3f", durations[len(durations)-1])}
		_ = summary.Write(row)
		fmt.Println(row)
		if failures > 0 {
			raw.Flush()
			summary.Flush()
			return fmt.Errorf("%s had %d failures", sc.name, failures)
		}
		wantCalls := int64(100)
		if sc.name == "cache_hit" {
			wantCalls = 0
		} else if sc.name == "identical_cold" {
			wantCalls = int64(100 / sc.concurrency)
		}
		if apiCalls != wantCalls {
			return fmt.Errorf("%s: API calls %d, expected %d", sc.name, apiCalls, wantCalls)
		}
	}
	raw.Flush()
	summary.Flush()
	if err := raw.Error(); err != nil {
		return err
	}
	if err := summary.Error(); err != nil {
		return err
	}
	metadata := map[string]any{"mock_delay_ms": 50, "response_bytes": len(payload), "articles_per_request": 5, "requests_per_repetition": 20, "measured_repetitions": 5, "warmup_repetitions": 1, "percentile_method": "nearest rank over 100 operation samples", "request_timing": "public FetchCachedArticlesWithSource including database open/close; schema initialization excluded", "throughput_timing": "sum of measured fan-out wave durations; CSV writing and warmup excluded", "timestamp_utc": time.Now().UTC().Format(time.RFC3339)}
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(*out, "method.json"), append(data, '\n'), 0644)
}
