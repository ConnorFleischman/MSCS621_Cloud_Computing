# Project 1 Report Draft: Bullet-Style Evidence

> Verified against the implementation in the current repository. This report covers the assignment requirements requested in the project PDF, the actual code paths in the Go project, and the SQLite migration that replaced the earlier MongoDB-oriented design for Docker size reduction.

## Title and Submission Information
- Project: NewsAPI News Search CLI
- Course: MSCS 621 Cloud Computing, Fall 2026
- Authors: **TODO: confirm names and roles**
- Submission date: **TODO**
- Implementation language: Go 1.25 module with Dockerized build and execution.
- Database: embedded SQLite persisted at `/data/news.db` inside the container volume.
- Release container: multi-stage Docker build ending in `scratch` runtime.
- Measured release image: `project1-metrics:latest`, Linux/amd64.
- Image index digest: `sha256:6c54e259082b818df909da82c76bf909e99e12f51873c1b0c327d7b809401c03`.
- Source revision: `bedaed13fe21b115e9fbcb042a6a16e2517a37e6` plus documentation and benchmark harness changes; final submission tag selection remains with the authors.
- Verified implementation files: `main.go`, `newsapi/cache.go`, `newsapi/sqlite.go`, `dockerfile`, `docker-compose.yml`, `README.md`, and `docs/docker_image_optimization.md`.

## Abstract
- This project implements a Go-based CLI that accepts a topic, a number of lookback days, and a requested limit on returned articles.
- The application calls NewsAPI, caches query coverage and article results, and reuses stored results when the request is still fresh and sufficiently covered.
- The design uses Go goroutines and channel-based result/error fan-out to serve multiple requests concurrently while preserving a per-query cache lock for overlapping misses.
- The database of record is SQLite, not MongoDB. The project originally used a MongoDB-oriented design, but the final implementation was migrated to SQLite to reduce Docker image size and runtime footprint.
- Verified metrics show a single warm cache hit p50 of 0.926 ms versus 62.647 ms for a mock API fetch with a fixed 50 ms delay. Distinct cold searches reached 118.190 searches/s at concurrency 20 with zero errors across 1,600 measured requests.

## 1. Requirements Coverage from the Assignment PDF
- Search topic and keyword support:
  - The CLI supports `-topic` and the legacy `-q` alias in `main.go`.
  - A country filter is also supported through `-country`.
- Search date range and article count limit:
  - `-days` defines the lookback window and `-articles` caps the number returned.
  - The cache logic checks whether the dates and count are already covered before calling NewsAPI.
- Persistent database caching:
  - The project stores article and coverage records in SQLite when `DATABASE_PATH` is set.
  - If a request has the same normalized topic, country, and date coverage and enough cached items, the code returns cached results instead of reaching NewsAPI.
  - If the user expands the time window or requests a larger limit, the application refreshes the cache and adds additional results.
- Concurrency and multiple users:
  - `ProcessRequestsWithSource` fans out requests into goroutines and emits `RequestResult` and `error` values over channels.
  - This supports simultaneous requests without serializing all work.
- Docker image and deployment requirements:
  - The project ships a Docker image built from `dockerfile`.
  - The final runtime is a minimal `scratch` image with the executable and CA certificates only.
- Reporting and tests:
  - The project includes verification steps, fixture tests, benchmark harnesses, and documentation files under `docs/` and `metrics/`.
- AI and disclosure requirements:
  - The prompt logs are stored in `Prompts/connor_prompts.md` and `Prompts/das_prompts.md`.
  - The current report explicitly records the AI-assisted workflow and itemizes reproducibility limitations and missing exact model metadata.

## 2. Architecture

### 2.1 Go, NewsAPI, Docker, and persistence architecture
- `main.go` is the CLI entrypoint and validates the flags before invoking the request pipeline.
- `models/news.go` defines the response and article structures used by the Go app.
- `newsapi/newsapi.go` contains the NewsAPI request logic and request normalization.
- `newsapi/cache.go` contains the cache policy, request fan-out, deduplication, and result merging logic.
- `newsapi/sqlite.go` initializes SQLite, creates the schema, and persists article/coverage data in a transaction-safe way.
- `dockerfile` builds the executable in a Go builder stage, compresses it with UPX, and copies only the runtime binary and CA certificates into the minimal final image.
- `docker-compose.yml` mounts a named volume at `/data` so database files persist between container runs.

### 2.2 Why SQLite replaces MongoDB in the final implementation
- The assignment permitted choosing the database technology.
- The original project direction included a MongoDB-based architecture, but the final implementation replaces that path with SQLite because SQLite is embedded in the executable and eliminates the need for a separate MongoDB service image.
- This design directly addresses the Docker image size requirement in the assignment: the final image is a single minimal binary + CA bundle, rather than a Go app plus an additional MongoDB server image.
- The project documentation in `docs/docker_image_optimization.md` records the measured image-size comparison and shows that the SQLite image is significantly smaller than the previous Go + MongoDB footprint.
- The migration is expressly documented in `docs/docker_image_optimization.md` and `README.md`, and the runtime behavior still retains the cache, concurrency, and CLI semantics of the previous design.

### 2.3 Cache behavior and data model
- The cache key includes normalized topic, country, and date range.
- The refresh policy uses a 15-minute cache lifetime.
- The code checks coverage by comparing the requested range against stored coverage and by ensuring the cached item count is sufficient.
- If the date window or result count is insufficient, the project requests more data from NewsAPI and merges the new results without duplication.
- The implementation deduplicates articles by URL or a content-derived fallback key to prevent duplicate records.
- The `articles` table holds topic, country, article identity, publication timestamp, and complete article JSON.
- The `coverage` table stores the covered date range, fetch time, exhausted flag, and requested limit.
- This behavior is verified in the cache and SQLite tests and in the README fixture documentation.

### 2.4 Concurrency model and reliability
- The concurrency model creates one goroutine per request and returns success and failure values through separate channels.
- Independent requests overlap while identical or near-identical cache misses are coalesced behind a per-query lock to avoid duplicate fetches for the same missing entry.
- SQLite uses WAL mode, a bounded busy timeout, durable synchronous writes, and small transactions to preserve correctness across concurrent request threads and multiple container invocations.
- The DB writes are short and do not span an HTTP request.
- The project explicitly validates racing and concurrency behaviors through the SQLite tests and the `go test -race` run.

## 3. Build, Run, Test, Input, and Output Instructions
- Native reference build prerequisites:
  - Go 1.25.0+
  - CGO-compatible C compiler for local builds
  - Docker recommended for a repeatable build
- Docker build command:
  - `docker compose -p project1 build go-app`
- Single query command:
  - `docker compose -p project1 run --rm go-app -topic "cloud computing" -days 7 -articles 5`
- Batch command:
  - `docker compose -p project1 run --rm -v "./searches.example.json:/app/searches.example.json:ro" go-app -batch /app/searches.example.json`
- Local test command using the Docker build stage:
  - `docker build -f dockerfile --target build -t project1-go-build .`
  - `docker run --rm -e DATABASE_PATH= project1-go-build go test ./... -count=1`
- Race check command:
  - `docker run --rm -e DATABASE_PATH= project1-go-build go test -race ./... -count=1`
- Fixture tests used in the project:
  - `go test ./newsapi -run '^TestCacheMissFixture$' -v -count=1`
  - `go test ./newsapi -run '^TestCacheHitFixture$' -v -count=1`
  - `go test . -run '^TestInvalidBatchFixture$' -v -count=1`
  - `go test ./newsapi -run '^TestConcurrentRequestsFixture$' -v -count=1`
  - `go test . -run '^TestPrintArticlesGoldenOutput$' -v -count=1`
- Important inputs and outputs:
  - `searches.example.json` contains the sample batch input.
  - `newsapi/testdata/cache-case.json` contains the cache fixture.
  - `newsapi/testdata/concurrent-case.json` contains the concurrent-request fixture.
  - `testdata/invalid-batch-case.json` contains invalid-batch input.
  - `testdata/expected/cache-hit.txt` and `testdata/expected/cache-miss.txt` are the golden outputs.
- Output behavior:
  - The CLI prints article titles, sources, author, date, description, URL, and whether the source was cache or NewsAPI.
  - When fewer articles are available than requested, the app states the count difference.

## 4. Tests and Findings
- Verified test evidence:
  - The repository includes the pass-oriented command sequence and the serialized test output under `metrics/test_results.jsonl` and `metrics/race_results.jsonl`.
  - `README.md` and the metrics collection notes confirm the commands and environment used for the verified runs.
- Findings from the tests:
  - Cache miss triggers one upstream NewsAPI request and stores the result.
  - Cache hit reuses the stored data and reduces or eliminates external requests.
  - Invalid batch input fails validation and avoids a fetch.
  - Concurrent identical searches reuse the hit path and reduce redundant API calls.
  - CLI output matches the golden files.
  - SQLite schema and transaction logic survive repeated, overlapping, and failed-write scenarios.
  - Race execution confirms there are no data races under the project’s concurrency model.
- Environment for the measured run:
  - Go 1.25.14, GCC 15.2.0, Docker Engine 29.8.0, Docker Desktop 4.91.0, Linux/amd64, Windows host.
- Verified benchmark outcomes:
  - Cache-hit p50 latency: 0.926 ms.
  - Cache-miss p50 latency with 50 ms mock delay: 62.647 ms.
  - Distinct cold searches at concurrency 20: 118.190 searches/s.
  - Identical cold searches at concurrency 20: 224.394 searches/s with only five API calls for 100 searches.
  - Zero errors were recorded across the 1,600 measured requests.

## 5. Performance Measurements and Benchmark Results
- Benchmark harness:
  - `metrics/harness/main.go` calls the real fetch path used by the app and exercises the same goroutine/channel design.
  - `metrics/run_metrics.sh` drives the benchmark run and captures raw data.
- Measurement method:
  - Local mock HTTP server with a fixed 50 ms delay.
  - Reused temporary SQLite database for warmed and cold scenarios.
  - One warm-up run followed by five measured repetitions per scenario.
  - 100 samples per row; 1,600 measured operations total.
  - Metrics capture p50, p95, p99, max latency, throughput, errors, and API-call count.
- Verified benchmark table:
  - Cache hit, concurrency 1: 100 completed, 0 errors, 0 API calls, 1044.035 searches/s, p50 0.926 ms.
  - Cache hit, concurrency 20: 100 completed, 0 errors, 0 API calls, 1044.043 searches/s, p50 12.259 ms.
  - API fetch/cache miss, concurrency 1: 100 completed, 0 errors, 100 API calls, 15.451 searches/s, p50 62.647 ms.
  - Unique cold searches, concurrency 20: 100 completed, 0 errors, 100 API calls, 118.190 searches/s, p50 94.894 ms.
  - Identical cold searches, concurrency 20: 100 completed, 0 errors, 5 API calls, 224.394 searches/s, p50 67.167 ms.
- Image-size and startup measurement summary:
  - Executable size: 2,470,712 bytes after build and UPX compression.
  - Runtime image size: 5,277,856 bytes local Docker image-store size.
  - Docker image export archive: 2,595,328 bytes.
  - The measured SQLite image is substantially smaller than the prior Go + MongoDB combination; this is the concrete reason the project moved to SQLite.
- Important caveat:
  - Latency and throughput results are for the deterministic local mock environment and the project’s single-host SQLite deployment. They are not live NewsAPI production numbers.

## 6. Related Work and Academic Disclosure
- IEEE or ACM citation style is used in the project documentation; they are not combined across styles.
- Relevant cited work and sources include:
  - Go language specification and memory model
  - NewsAPI documentation
  - Docker multi-stage build documentation
  - SQLite WAL and transaction documentation
  - Tail-at-scale systems work as an example of latency/throughput discussion in concurrent distributed systems
- How the project differs from cited work:
  - This project is not a new database engine or a novel cache algorithm.
  - It is a compact CLI application that uses established Go concurrency primitives, a NewsAPI client, SQLite persistence, and a minimal Docker runtime to provide a single-host cached news-fetch workflow.
  - The main contribution is the selection, integration, and benchmarking of existing technologies for a cloud-computing assignment rather than a new distributed system design.

## 7. AI Use and Reproducibility
- Prompt log files already present in the project:
  - `Prompts/connor_prompts.md`
  - `Prompts/das_prompts.md`
- Verified reporting requirement:
  - The project includes the AI prompt history and explicitly states that exact model/version details must be confirmed before final submission.
- This session’s prompt text was:
  - “please create a template document containing the necessary points for the documentation. Then copy the template and generate a bullet-stile report containing the necessary information to include in the documentation. Ensure these two documents remain seperate artifacts.”
- Additional planning prompt used in this session also covered the report structure, latency measurement, citation, and AI-disclosure requirements.
- Exact AI provider/model/version status:
  - The project documentation records that the logs mention Gemini 3.1 Pro and GPT-6 Astra/Luna, but the exact model/version should be verified against the tool metadata before final submission.
  - This assistant is GitHub Copilot; the exact tool model version is pending verification in the VS Code metadata and should be captured before archival submission.
- Reproducibility requirement:
  - All prompts, prompt dates, tool/provider, code context, and resulting edits should be reported in the final documentation package.
  - The report should avoid embedding API keys or secrets in the prompt log or the final report.

## 8. Limitations and Future Work
- SQLite is a single-host persistent database and is not intended for multi-host shared storage or remote replication.
- The benchmark is a local deterministic scenario, not a production NewsAPI load test.
- The per-query lock still serializes hot identical misses within a process, so concurrency gains are bounded by the database and lock interactions.
- Historical MongoDB data is not imported automatically into the SQLite database.
- Final submission metadata such as the exact author names, date, and final archive tag still require confirmation.

## References (IEEE Style)
- [1] The Go Authors, “The Go Programming Language Specification,” 2026. [Online]. Available: https://go.dev/ref/spec. [Accessed: Oct. 2, 2026].
- [2] The Go Authors, “The Go Memory Model,” 2026. [Online]. Available: https://go.dev/ref/mem. [Accessed: Oct. 2, 2026].
- [3] NewsAPI, “News API Documentation,” 2026. [Online]. Available: https://newsapi.org/docs. [Accessed: Oct. 2, 2026].
- [4] Docker, “Multi-stage builds,” 2026. [Online]. Available: https://docs.docker.com/build/building/multi-stage/. [Accessed: Oct. 2, 2026].
- [5] SQLite, “Write-Ahead Logging,” 2026. [Online]. Available: https://sqlite.org/wal.html. [Accessed: Oct. 2, 2026].
- [6] SQLite, “Transactions,” 2026. [Online]. Available: https://sqlite.org/lang_transaction.html. [Accessed: Oct. 2, 2026].
- [7] J. Dean and L. A. Barroso, “The tail at scale,” Communications of the ACM, vol. 56, no. 2, pp. 74-80, Feb. 2013, doi: 10.1145/2408776.2408794.

## Submission Completion Checklist
- [x] Search topic, days, article-count, cache reuse, and expanded-range refresh behavior are described.
- [x] Go, API, Docker, goroutine, and channel architecture are documented.
- [x] Cache behavior and concurrency handling are documented.
- [x] Tests and findings are described with actual repository evidence.
- [x] Compile, run, test, input, and output instructions are included.
- [x] Benchmark results are recorded and linked to the harness and raw results.
- [x] MongoDB-to-SQLite migration and the Docker image-size reduction rationale are included.
- [x] AI disclosure and prompt-log requirements are included, with exact model/version verification left as a final submission step.
- [x] The content is bullet-style and aligned with the implementation in the Go project.