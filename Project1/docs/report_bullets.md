# Project 1 Report Draft: Bullet-Style Evidence

> Draft based on the current repository. Complete items marked **TODO** before submission. Performance measurements and test evidence were collected on October 3, 2026. Exact AI model metadata still requires verification.

## Title and Submission Information
- Project: NewsAPI News Search CLI
- Course: MSCS 621 Cloud Computing, Fall 2026
- Authors: **TODO: confirm names and roles**
- Submission date: **TODO**
- Implementation language: Go 1.25.0 module; Docker build uses Go 1.25 Alpine.
- Database: embedded SQLite, persisted at `/data/news.db` in Docker.
- Release container: multi-stage build with a `scratch` runtime.
- Measured release image: `project1-metrics:latest`, Linux/amd64.
- Image index digest: `sha256:6c54e259082b818df909da82c76bf909e99e12f51873c1b0c327d7b809401c03`.
- Source revision: `bedaed13fe21b115e9fbcb042a6a16e2517a37e6` plus the working-tree metrics harness and documentation changes; final submission tag selection remains with the authors.

## Abstract
- Go CLI searches NewsAPI for a topic over a requested number of days, limits returned articles, and caches article data and search coverage.
- SQLite is the configured persistent cache backend; the existing file-cache path remains available.
- Batch searches use goroutines and result/error channels to process independent searches concurrently.
- The release image contains the executable and CA certificates rather than a Go toolchain or database server.
- Measured single cache-hit p50: 0.926 ms; mock API/cache-miss p50: 62.647 ms with a fixed 50 ms upstream delay. Distinct cold queries reached 118.190 searches/s at concurrency 20; all 1,600 measured requests succeeded.

## 1. Requirements and Scope
- Supports topic, country, lookback days (including today), and article limit through CLI flags.
- Provides single-search and JSON batch invocation.
- Uses NewsAPI on cache misses or when stored date/count coverage is insufficient or stale.
- Persists results and coverage in SQLite when `DATABASE_PATH` is set; an empty path selects file-cache behavior.
- **Database:** SQLite is embedded in the executable through `mattn/go-sqlite3`; it stores articles and query coverage in a local database file without a separate database container.
- SQLite is a single-host database using a local persistent volume; multi-host storage is outside the implemented scope.

## 2. Architecture

### 2.1 Components and Request Flow
- `main.go` parses CLI flags, loads configuration, builds one request or reads a JSON batch, invokes the request fan-out, formats output, and optionally saves JSON article documents.
- `models/` contains shared API data structures; `newsapi/` owns API access, request normalization, cache decisions, and persistence.
- A request is normalized by topic/country and date range, then cache coverage and article count/freshness are checked.
- A sufficient fresh cache entry returns without an API fetch. Otherwise, the fetch path requests pages, merges/deduplicates articles, and stores articles plus coverage.
- SQLite is initialized/versioned in `newsapi/sqlite.go`; `PRAGMA user_version=1` identifies the supported schema. Unknown versions are rejected.
- The `articles` table stores topic, country, a deduplication key, publication time, and article JSON; its primary key prevents duplicates within each topic/country.
- The `coverage` table records searched date intervals, fetch times, exhausted-result status, and requested limits. Indexed reads select matching coverage and newest articles.
- Article upserts and coverage updates commit in one transaction, so a failed write cannot publish coverage without its corresponding articles.
- Docker Compose mounts a named volume at `/data`, so the SQLite database persists across temporary containers.

### 2.2 Architectural Decisions
- **Go:** one compiled application, standard goroutine/channel concurrency, and a compact multi-stage Docker build.
- **SQLite:** the database engine is compiled into the application. SQL tables and indexes support topic/country/date lookups; atomic transactions keep articles and coverage consistent. The runtime needs only the executable, certificates, and a persistent database file.
- **NewsAPI:** supplies third-party news search; the API key is injected at runtime through environment configuration and is not part of the image.
- **Docker:** multi-stage build keeps compiler, C build tools, and UPX in the builder; the final `scratch` stage contains only runtime necessities and the executable.
- **SQLite durability/concurrency settings:** WAL records changes separately from the main database so readers can proceed while a writer commits; writes remain serialized. `FULL` synchronous mode requests durable synchronization. Each operation opens a connection pool capped at one connection, with a 50 ms native busy timeout and 10 ms retry waits bounded by the configured database deadline. No database transaction spans an HTTP request.
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
- The measured runtime platform is Linux/amd64, built and executed through Docker Desktop on Windows.

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
  - `docker run --rm -e DATABASE_PATH= project1-go-build go test ./... -count=1`
- Race detector:
  - `docker run --rm -e DATABASE_PATH= project1-go-build go test -race ./... -count=1`
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
- October 3, 2026: `go test -json ./... -count=1` and `go test -race -json ./... -count=1` each passed 57 named tests, with zero failed or skipped tests. Three packages contain tests; the models and metrics command packages have no tests.
- Complete command output: [test_results.jsonl](../metrics/test_results.jsonl) and [race_results.jsonl](../metrics/race_results.jsonl). Environment and package durations: [runtime.json](../metrics/metrics-2026-10-03/runtime.json).
- Fixture tests verify one API fetch on a miss, no extra fetch on a sufficient hit, invalid batch rejection without fetching, overlapping concurrent requests, and stable CLI output.
- The test suite includes API/cache behavior, concurrency, timeout behavior, SQLite persistence, transaction rollback/recovery, schema validation, and a full SQLite-backed application flow.
- The project documentation reports successful race-detector runs and container smoke tests including cache reuse across temporary containers and simultaneous containers sharing SQLite storage.
- Tests used Go 1.25.14, GCC 15.2.0, Docker Engine 29.8.0, and Docker Desktop 4.91.0, Linux/amd64. Revision and image identity are recorded above and in the raw metadata. The race run reported no races.
- The release image was rebuilt on October 3 and passed six separate cache-hit invocations with networking disabled. Every invocation returned five articles from the same SQLite fixture after the preceding container was removed. Source tests and the race run were separate from the final request benchmark run.
- The packed executable inside the development image was also timed separately; this isolates process startup from Docker launch/removal time. Final submission packaging and author metadata remain to be completed.

## 5. Performance Method and Results

### 5.1 Measured Method
- Measured October 3, 2026 on Windows 10 build 19045, Intel64 Family 6 Model 158 Stepping 10. Docker Desktop 4.91.0 provided a Linux/amd64 VM with 12 logical CPUs and 7.68 GiB memory. Docker Engine: 29.8.0; Go: 1.25.14; GCC: 15.2.0; UPX: 5.2.0.
- Harness: [`metrics/harness/main.go`](../metrics/harness/main.go), using the public `FetchCachedArticlesWithSource` path and the normal goroutine/channel fan-out. [`run_metrics.sh`](../metrics/run_metrics.sh) builds it with the release C optimization flags, static linking, and `netgo`, `osusergo`, and `sqlite_omit_load_extension` tags. The harness executable is unpacked; its startup occurs before request timing.
- A local HTTP mock returned five dated articles in a 680-byte JSON response after a fixed 50 ms delay. A request-local transport redirected NewsAPI HTTP calls to the mock. No live NewsAPI requests or real API keys were used.
- SQLite schema initialization and an initial HTTP fetch occurred before timing. Cache hits reused one warmed topic; cold requests used fresh topics with no cached coverage. The initialized temporary database was reused across scenarios and stored inside the Linux container filesystem.
- Each of 16 scenario/concurrency combinations had one excluded warm-up repetition and five measured repetitions of 20 requests: 100 latency samples per row and 1,600 measured requests overall. Each repetition ran successive waves at concurrency 1, 2, 5, 10, or 20. Build and test jobs had finished before this final benchmark run.
- Operation latency includes normalization, database open/close, per-query lock waits, cache checks, optional mock HTTP, and SQLite writes. It excludes CLI/process startup, schema initialization, warm-up, and CSV writing.
- Percentiles use nearest rank over the 100 operation samples per row. Throughput is completed searches divided by the sum of measured fan-out wave durations; it describes these short bursts, not a long sustained load test.
- Identical cold waves share a topic, while distinct cold waves use one fresh topic per request. The harness checks five articles per result, zero errors, and expected API-call counts; a mismatch exits unsuccessfully.
- Raw results: [summary CSV](../metrics/metrics-2026-10-03/benchmark_results.csv), [all 1,600 operation samples](../metrics/metrics-2026-10-03/operations.csv), [method metadata](../metrics/metrics-2026-10-03/method.json), and [runtime/image metadata](../metrics/metrics-2026-10-03/runtime.json). Exact reproduction commands: [metrics README](../metrics/metrics-2026-10-03/README.md).

### 5.2 Application Benchmark Results
| Scenario | Concurrent searches | Completed | Errors | API calls | Throughput (searches/s) | p50 (ms) | p95 (ms) | p99 (ms) | Max (ms) |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Cache hit (same warm topic) | 1 | 100 | 0 | 0 | 1044.035 | 0.926 | 1.140 | 1.578 | 1.617 |
| Cache hit (same warm topic) | 2 | 100 | 0 | 0 | 1099.560 | 1.524 | 1.925 | 2.494 | 2.674 |
| Cache hit (same warm topic) | 5 | 100 | 0 | 0 | 1222.999 | 3.000 | 4.184 | 4.410 | 4.485 |
| Cache hit (same warm topic) | 10 | 100 | 0 | 0 | 916.035 | 7.911 | 12.086 | 12.901 | 14.379 |
| Cache hit (same warm topic) | 20 | 100 | 0 | 0 | 1044.043 | 12.259 | 15.910 | 17.514 | 18.090 |
| API fetch / cache miss | 1 | 100 | 0 | 100 | 15.451 | 62.647 | 74.446 | 78.074 | 78.674 |
| Unique cold searches | 1 | 100 | 0 | 100 | 15.669 | 61.703 | 74.906 | 78.545 | 78.746 |
| Unique cold searches | 2 | 100 | 0 | 100 | 28.279 | 64.842 | 86.564 | 88.592 | 91.323 |
| Unique cold searches | 5 | 100 | 0 | 100 | 45.266 | 76.716 | 120.890 | 129.259 | 137.066 |
| Unique cold searches | 10 | 100 | 0 | 100 | 66.848 | 97.648 | 150.261 | 184.054 | 197.534 |
| Unique cold searches | 20 | 100 | 0 | 100 | 118.190 | 94.894 | 185.250 | 200.235 | 203.311 |
| Identical cold searches | 1 | 100 | 0 | 100 | 15.591 | 62.616 | 74.309 | 76.107 | 82.171 |
| Identical cold searches | 2 | 100 | 0 | 50 | 29.264 | 62.449 | 80.160 | 83.245 | 83.751 |
| Identical cold searches | 5 | 100 | 0 | 20 | 66.836 | 66.172 | 84.141 | 93.121 | 98.968 |
| Identical cold searches | 10 | 100 | 0 | 10 | 129.014 | 67.425 | 83.012 | 89.025 | 100.314 |
| Identical cold searches | 20 | 100 | 0 | 5 | 224.394 | 67.167 | 98.143 | 99.712 | 106.809 |

- Single-request cache-hit p50 was 0.926 ms versus 62.647 ms for a cache miss with a 50 ms mock delay. All 500 measured warm-cache requests made zero API calls.
- Distinct cold-query throughput rose from 15.669 searches/s at concurrency 1 to 118.190 at concurrency 20 (about 7.54 times). At concurrency 20, p95 was 185.250 ms and maximum latency was 203.311 ms; overlap improves throughput, while database contention and scheduling increase per-request latency.
- Identical cold queries at concurrency 20 completed 100 searches using five API calls, a 95% reduction relative to one API call per search. Their throughput was 224.394 searches/s. Coalescing is within one process; these results do not demonstrate API-call coalescing across containers.
- Cache-hit requests sharing one topic take the per-query lock even when warm. Concurrent callers therefore wait behind one another; the increase in p50 from 0.926 ms at concurrency 1 to 12.259 ms at concurrency 20 reflects this path and database open/close work.
- Every scenario had zero errors. Mock timings reflect this host, small payload, database state, and finite sample size; they do not predict live NewsAPI latency or production capacity.

### 5.3 Release Image, Startup, and Memory Measurements
- Measured release tag: `project1-metrics:latest`; image index digest: `sha256:6c54e259082b818df909da82c76bf909e99e12f51873c1b0c327d7b809401c03`. The runtime contains the packed executable and CA certificates; SQLite is embedded in the executable. No separate database-server image is required.

| Measurement | Measured value | Scope |
| --- | ---: | --- |
| Packed executable | 2,470,712 bytes (2.471 MB) | Application binary including SQLite |
| Saved image layer blobs | 2,576,313 bytes | Layer blobs in this engine's exported archive |
| Docker image save archive | 2,595,328 bytes (2.595 MB) | Complete image export |
| Gzip of save archive | 2,581,395 bytes (2.581 MB) | Compressed export |
| Docker inspect Size | 5,277,856 bytes (5.278 MB) | Local image-store accounting on this engine |
| Cache-hit Docker invocation median | 629.882 ms | Client, container creation, application, output, and removal |
| Cache-hit Docker invocation range | 571.827-699.999 ms | Five measured runs after one warm-up |
| Packed-process wall time | 0.27-0.34 s | `/usr/bin/time -v` inside development container; excludes Docker launch |
| Packed-process peak RSS | 7,888-8,016 KiB (7.70-7.83 MiB) | Five cache-hit processes after one warm-up |

- The release-container smoke test used a fresh host bind-mounted SQLite fixture and `--network none`; six removed/recreated containers each returned five cached articles. Representative output: [release_cache_hit.txt](../metrics/metrics-2026-10-03/release_cache_hit.txt).
- Process wall time and RSS were measured for the packed release executable inside the development image because the `scratch` image has no measurement tools. Those timings use the host bind-mounted fixture, whereas request benchmarks use a temporary database inside Linux. Raw per-run timing, CPU, page-fault, and RSS output is retained in `runtime.json`.
- Executable startup includes UPX unpacking. The process and Docker figures include startup and output, so they must not replace the in-process request-latency table. Five startup samples and coarse process-timer resolution are insufficient for meaningful tail-percentile claims.
- MB means 1,000,000 bytes; MiB means 1,048,576 bytes. Export, executable, and local-store sizes are different measurements. Builder caches, persistent database data, and the Docker VM are excluded from the release-image figures.

## 6. Related Work and Citations
- Use IEEE numbered citations consistently.
- Explain that the project integrates documented Go concurrency primitives, NewsAPI, SQLite, and Docker rather than introducing a new concurrency or cache algorithm.
- Relate the implemented SQLite tables, indexes, WAL, and atomic transactions to the SQLite documentation. Explain how these established mechanisms support the application cache without claiming a new database algorithm.
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
- Database persistence depends on preserving the local SQLite file/volume. Backups, automated retention, and multi-host replication are not implemented.
- Application measurements are now included; full AI prompt/model provenance and submission metadata remain outstanding. Tests cover correctness and races, while the finite benchmark run measures only the documented local scenarios.

## References (IEEE Style)
[1] The Go Authors, “The Go Programming Language Specification.” [Online]. Available: https://go.dev/ref/spec. [Accessed: Oct. 2, 2026].

[2] The Go Authors, “The Go Memory Model.” [Online]. Available: https://go.dev/ref/mem. [Accessed: Oct. 2, 2026].

[3] NewsAPI, “News API Documentation.” [Online]. Available: https://newsapi.org/docs. [Accessed: Oct. 2, 2026].

[4] Docker, “Multi-stage builds.” [Online]. Available: https://docs.docker.com/build/building/multi-stage/. [Accessed: Oct. 2, 2026].

[5] SQLite, “Write-Ahead Logging.” [Online]. Available: https://sqlite.org/wal.html. [Accessed: Oct. 2, 2026].

[6] SQLite, “Transactions.” [Online]. Available: https://sqlite.org/lang_transaction.html. [Accessed: Oct. 2, 2026].

[7] J. Dean and L. A. Barroso, “The tail at scale,” *Communications of the ACM*, vol. 56, no. 2, pp. 74-80, Feb. 2013, doi: 10.1145/2408776.2408794.

## Submission Completion Checklist
- [x] Describe the implemented SQLite database, schema, transactions, WAL, and storage volume.
- [x] Record measured build, test-environment, platform, and image identity metadata.
- [ ] Complete author information, submission date, and final source/archive location.
- [x] Run and record cache-hit, API-fetch, and simultaneous-search measurements.
- [x] Include raw benchmark CSV and exact harness/run instructions.
- [ ] Verify prompt logs are complete and model/version labels are exact.
- [ ] Add inline IEEE citations and check every reference.
- [ ] Ensure no secrets are present in the documents or source archive.