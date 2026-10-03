# Project 1 Report Draft: Bullet-Style Evidence

> Draft based on the current repository. Complete items marked **TODO** before submission. Performance results and exact AI model metadata are not available in the repository evidence reviewed so far.

## Title and Submission Information
- Project: NewsAPI News Search CLI
- Course: MSCS 621 Cloud Computing, Fall 2026
- Authors: **TODO: confirm names and roles**
- Submission date: **TODO**
- Implementation language: Go 1.25.0 module; Docker build uses Go 1.25 Alpine.
- Database: embedded SQLite, persisted at `/data/news.db` in Docker.
- Release container: multi-stage build with a `scratch` runtime.
- Docker image tag/digest submitted: **TODO: capture final image tag and digest**

## Abstract
- Go CLI searches NewsAPI for a topic over a requested number of days, limits returned articles, and caches article data and search coverage.
- SQLite is the configured persistent cache backend; the existing file-cache path remains available.
- Batch searches use goroutines and result/error channels to process independent searches concurrently.
- The release image contains the executable and CA certificates rather than a Go toolchain or database server.
- Application latency/throughput results: **TODO: run and report the performance procedure in Section 5.**

## 1. Requirements and Scope
- Supports topic, country, lookback days (including today), and article limit through CLI flags.
- Provides single-search and JSON batch invocation.
- Uses NewsAPI on cache misses or when stored date/count coverage is insufficient or stale.
- Persists results and coverage in SQLite when `DATABASE_PATH` is set; an empty path selects file-cache behavior.
- **Database requirement clarification:** the assignment allows a database of choice, while the requested report criteria mention MongoDB. The submitted implementation is SQLite, not MongoDB. Explain the MongoDB-to-SQLite migration and confirm this interpretation with the instructor if the rubric requires MongoDB specifically.
- Does not claim that the local SQLite cache provides MongoDB's remote database-server deployment model.

## 2. Architecture

### 2.1 Components and Request Flow
- `main.go` parses CLI flags, loads configuration, builds one request or reads a JSON batch, invokes the request fan-out, formats output, and optionally saves JSON article documents.
- `models/` contains shared API data structures; `newsapi/` owns API access, request normalization, cache decisions, and persistence.
- A request is normalized by topic/country and date range, then cache coverage and article count/freshness are checked.
- A sufficient fresh cache entry returns without an API fetch. Otherwise, the fetch path requests pages, merges/deduplicates articles, and stores articles plus coverage.
- SQLite is initialized/versioned in `newsapi/sqlite.go`; article and coverage writes are committed in one transaction.
- Docker Compose mounts a named volume at `/data`, so the SQLite database persists across temporary containers.

### 2.2 Architectural Decisions
- **Go:** one compiled application, standard goroutine/channel concurrency, and a compact multi-stage Docker build.
- **SQLite instead of MongoDB:** embedded database avoids a separate database service and MongoDB runtime image; the migration reduced runtime footprint. It is a single-host, local-volume design and not a network database service.
- **NewsAPI:** supplies third-party news search; the API key is injected at runtime through environment configuration and is not part of the image.
- **Docker:** multi-stage build keeps compiler, C build tools, and UPX in the builder; the final `scratch` stage contains only runtime necessities and the executable.
- **SQLite durability/concurrency settings:** WAL, bounded busy timeout, `FULL` synchronous mode, transactions, unique keys, and short database transactions are used. No database transaction is held during an HTTP request.
- **Tradeoff:** the selected `mattn/go-sqlite3` driver uses CGO, so native builds need a compatible C compiler; Docker supplies it in the build stage.

### 2.3 Cache Semantics
- Cache lifetime is 15 minutes.
- Cache identity includes normalized topic, country, and date interval; matching articles are selected newest-first and limited to the requested count.
- A hit requires coverage of the requested interval, fresh coverage, and enough results or an exhausted result set whose recorded requested limit is sufficient.
- A stale or insufficient entry triggers API retrieval. Fetched results are merged with cached results and deduplicated by URL, or by a content-derived key when URL is absent.
- Article records and matching coverage metadata are persisted atomically in SQLite. Restarted invocations can reuse stored data.
- The file cache remains available when SQLite is not selected.

### 2.4 Concurrency and Reliability
- `ProcessRequestsWithSource` starts one goroutine per request and reports successful results and failures on separate buffered channels.
- A narrow per-query in-process lock coalesces overlapping identical cache misses; unrelated searches can fetch concurrently.
- SQLite locks/transactions and uniqueness constraints protect database correctness across separate invocations. WAL supports concurrent readers, but SQLite still serializes writers.
- The fan-out currently starts a goroutine for each batch entry; performance results must state tested batch sizes and should not imply unlimited concurrency.
- API and database operations have configurable timeouts. SQLite busy/locked errors are retried within the configured operation deadline.

## 3. Build, Configuration, and Usage

### 3.1 Configuration
- Obtain a NewsAPI key and set `NEWSAPI_API_KEY` in the local `.env` file; do not put the key in source, prompt logs, screenshots, or this report.
- `DATABASE_PATH` selects SQLite; Compose sets it to `/data/news.db`.
- `DB_TIMEOUT` / `-db-timeout` configure database operation timeout; `NEWSAPI_TIMEOUT` / `-api-timeout` configure API timeout.
- The runtime image is Linux/amd64 as measured in the image optimization notes. **TODO: state the submitted target platform.**

### 3.2 Compile, Run, and Test Commands
- Run commands from `Project1`.
- Docker release build:
  - `docker compose -p project1 build go-app`
- Single query:
  - `docker compose -p project1 run --rm go-app -topic "cloud computing" -days 7 -articles 5`
- Batch query:
  - `docker compose -p project1 run --rm -v "./searches.example.json:/app/searches.example.json:ro" go-app -batch /app/searches.example.json`
- Build the development stage and run the full suite:
  - `docker build -f dockerfile --target build -t project1-go-build .`
  - `docker run --rm -e DATABASE_PATH= -e MONGO_URI= project1-go-build go test ./... -count=1`
- Race detector:
  - `docker run --rm -e DATABASE_PATH= -e MONGO_URI= project1-go-build go test -race ./... -count=1`
- Native source builds require Go 1.25.0+ and a CGO-compatible C compiler. Docker is the documented alternative.

### 3.3 Inputs and Expected Outputs
- `searches.example.json`: sample batch containing two topics and their requested day/count limits.
- `newsapi/testdata/cache-case.json`: deterministic cache hit/miss fixture.
- `newsapi/testdata/concurrent-case.json`: concurrent request fixture.
- `testdata/invalid-batch-case.json`: invalid batch fixture.
- `testdata/expected/cache-hit.txt` and `testdata/expected/cache-miss.txt`: CLI golden outputs.
- Fixture test commands:
  - `go test ./newsapi -run '^TestCacheMissFixture$' -v -count=1`
  - `go test ./newsapi -run '^TestCacheHitFixture$' -v -count=1`
  - `go test . -run '^TestInvalidBatchFixture$' -v -count=1`
  - `go test ./newsapi -run '^TestConcurrentRequestsFixture$' -v -count=1`
  - `go test . -run '^TestPrintArticlesGoldenOutput$' -v -count=1`

## 4. Tests and Findings
- The latest recorded `go test ./... -count=1` run completed successfully.
- Fixture tests verify one API fetch on a miss, no extra fetch on a sufficient hit, invalid batch rejection without fetching, overlapping concurrent requests, and stable CLI output.
- The test suite includes API/cache behavior, concurrency, timeout behavior, SQLite persistence, transaction rollback/recovery, schema validation, and a full SQLite-backed application flow.
- The project documentation reports successful race-detector runs and container smoke tests including cache reuse across temporary containers and simultaneous containers sharing SQLite storage.
- **TODO:** record exact test date, Go/Docker versions, commit/revision, test counts, and attach complete command output or a concise test log to the submission.
- **TODO:** confirm a clean rebuild and rerun the exact submitted Docker image before final submission.

## 5. Performance Method and Results

### 5.1 Method to Run
- Do not use live NewsAPI for load testing. Use a deterministic local mock HTTP server, record its response delay and payload, and count upstream requests.
- Measure cache-hit latency after warming a temporary SQLite DB with the same normalized query; verify subsequent calls make zero mock API requests.
- Measure API-fetch/cache-miss latency with empty coverage and a fixed mock delay; initialize the DB before timing so setup is not included unintentionally.
- Exercise distinct cold topics at concurrency levels 1, 2, 5, 10, and 20 to measure independent overlap.
- Exercise identical cold topics separately to measure same-query request coalescing and compare logical searches completed against mock API requests.
- Capture per-operation durations to calculate p50/p95/p99 and max; calculate throughput as completed searches divided by elapsed time; record errors and API call counts.
- Run a warm-up and at least five measured repetitions per case. Record host, OS/architecture, Go/Docker versions, image digest, request count, mock delay, DB state, and duration.
- Report in-process request latency separately from end-to-end CLI latency, which includes process startup.
- Suggested harness output: `benchmark_results.csv` with scenario, concurrency, completed, errors, API calls, elapsed seconds, throughput, p50, p95, p99, and max.

### 5.2 Application Benchmark Results
| Scenario | Concurrent searches | Completed | Errors | API calls | Throughput (searches/s) | p50 (ms) | p95 (ms) | p99 (ms) | Max (ms) |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Cache hit | TODO | TODO | TODO | TODO | TODO | TODO | TODO | TODO | TODO |
| API fetch / cache miss | TODO | TODO | TODO | TODO | TODO | TODO | TODO | TODO | TODO |
| Unique cold searches | TODO per level | TODO | TODO | TODO | TODO | TODO | TODO | TODO | TODO |
| Identical cold searches | TODO per level | TODO | TODO | TODO | TODO | TODO | TODO | TODO | TODO |

### 5.3 Existing Image Measurements (Not Application Latency Benchmarks)
- Measurements were taken on Linux/amd64 through Docker Desktop on October 1, 2026; builder versions and base digest are in `docker_image_optimization.md`.
- Selected compressed SQLite release image: 2.58 MB Docker content size and 5.28 MB local disk usage, as reported by that Docker installation.
- The optimization notes estimate this at approximately 99.3% smaller than the previous Go plus MongoDB images combined, with the documented measurement method and caveats.
- Three invalid-argument startup samples, excluding Docker startup, were 0.14-0.15 seconds and roughly 7 MB peak RSS for the packed executable, versus 0.02-0.03 seconds and roughly 5 MB for the unpacked executable.
- These are image/startup measurements, not cache-hit/API-fetch throughput results. Do not use them to fill the application benchmark table.

## 6. Related Work and Citations
- Use IEEE numbered citations consistently.
- Explain that the project integrates documented Go concurrency primitives, NewsAPI, SQLite, and Docker rather than introducing a new concurrency or cache algorithm.
- Contrast SQLite's embedded, single-host storage with MongoDB's server-based deployment if discussing the original design; do not imply equivalent deployment capabilities.
- **TODO:** verify access dates and cite each source inline in the final narrative.

## 7. AI Use and Reproducibility
- Existing prompt logs: `Prompts/connor_prompts.md` and `Prompts/das_prompts.md`.
- Those files provide a prompt history, but their entries appear summarized; verify/recover literal original prompts and include every follow-up prompt from every contributor/tool.
- The logs record Gemini 3.1 Pro and GPT-6 Astra/Luna. **TODO:** verify exact provider/model/version against original tool metadata and identify which prompts used each model.
- This report-generation pass used GitHub Copilot. **TODO:** record the exact model/version shown in VS Code metadata; it is not visible in the current assistant context.
- Documentation-generation prompt from this session, verbatim: “please create a template document containing the necessary points for the documentation. Then copy the template and generate a bullet-stile report containing the necessary information to include in the documentation. Ensure these two documents remain seperate artifacts.”
- Earlier planning prompt from this session asked for a plan covering the report, latency/concurrency measurement, citations, and AI disclosure, with the stated acceptance criteria and issue dependency order. **TODO:** copy its exact text from the conversation into the complete prompt appendix if treating this planning interaction as part of the project AI disclosure.
- **TODO:** record dates, supplied files/context, prompt outputs used, and human review/changes. Do not claim the current abbreviated logs are a complete reproducibility record.
- No API credentials belong in the report or prompt appendix.

## 8. Limitations
- SQLite/WAL is intended for one Docker host and a local persistent volume, not a shared network filesystem or remote multi-host database service.
- SQLite serializes writers; high-contention performance must be measured and disclosed.
- Batch processing starts one goroutine per request and has no documented fixed concurrency cap; report only tested load levels.
- NewsAPI latency, availability, result limits, and rate restrictions are external dependencies; deterministic mock results do not predict live-service latency.
- Historical MongoDB data is not automatically imported into the SQLite database.
- Application performance results and complete AI prompt/model provenance remain outstanding for this draft.

## References (IEEE Style)
[1] The Go Authors, “The Go Programming Language Specification.” [Online]. Available: https://go.dev/ref/spec. [Accessed: Oct. 2, 2026].

[2] The Go Authors, “The Go Memory Model.” [Online]. Available: https://go.dev/ref/mem. [Accessed: Oct. 2, 2026].

[3] NewsAPI, “News API Documentation.” [Online]. Available: https://newsapi.org/docs. [Accessed: Oct. 2, 2026].

[4] Docker, “Multi-stage builds.” [Online]. Available: https://docs.docker.com/build/building/multi-stage/. [Accessed: Oct. 2, 2026].

[5] SQLite, “Write-Ahead Logging.” [Online]. Available: https://sqlite.org/wal.html. [Accessed: Oct. 2, 2026].

[6] SQLite, “Transactions.” [Online]. Available: https://sqlite.org/lang_transaction.html. [Accessed: Oct. 2, 2026].

[7] J. Dean and L. A. Barroso, “The tail at scale,” *Communications of the ACM*, vol. 56, no. 2, pp. 74-80, Feb. 2013, doi: 10.1145/2408776.2408794.

## Submission Completion Checklist
- [ ] Confirm database choice satisfies the instructor/rubric interpretation.
- [ ] Fill all author, build, test-environment, and image identity metadata.
- [ ] Run and record cache-hit, API-fetch, and simultaneous-search measurements.
- [ ] Include raw benchmark CSV and exact harness/run instructions.
- [ ] Verify prompt logs are complete and model/version labels are exact.
- [ ] Add inline IEEE citations and check every reference.
- [ ] Ensure no secrets are present in the documents or source archive.