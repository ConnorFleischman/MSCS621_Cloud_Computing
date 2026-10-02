# NewsAPI Project

This project is a Go-based news search app that fetches stories from NewsAPI, caches results, and stores articles and query coverage in embedded SQLite when configured. Docker runs a single application container; no database server or local SQLite installation is needed.

## Project 1 Guidelines
- 3 weeks for project
- Write in Go code, follow AI rules in syllabus
- May be asked to explain snippets of code needed for this project as part of the exam
- Using newsapi.org
    - Create account to use API
    - Collection of news articles to try to access
- Must use Go Channels and Goroutines
- Needs to support multiple invocations
    - Assume there are multiple users that will reach the system at the same time
- Enable searching for a news topic (topic and keyword, find relevant articles)
- User specifies the topic, number of days from today, and maximum number of articles they are interested in
    - Enter 7 days, look from data from all last week. Only want 10 of them, can limit that. Pulling less than limit, just display those articles
- Can use CLI (text-based) or GUI
- Going to use database, can choose Database.
    - When user search, results go to database. If repeated data shows up, go to database first instead of API
    - Examine if database has that data, then go to the API
- Allowed to work in pair
- After midterm week, project presentation for each time, uploaded before class

## Architecture
- Go app entrypoint: `main.go`
- Shared models: `models/`
- Core app logic: `newsapi/`
- Unit tests are co-located with their packages (`main_test.go`, `newsapi/*_test.go`); the full-flow SQLite integration test is in `tests/` and runs automatically with a temporary database.
- SQLite storage: `newsapi/sqlite.go`; Docker persists `/data/news.db` in the `sqlite-data` named volume.

## Concurrency design
The service is designed to process independent news searches in parallel using goroutines and channels.

- Each request is wrapped in its own goroutine via `newsapi.ProcessRequests(...)`.
- The function sends successful results over a `results` channel and failures over an `errors` channel.
- Each goroutine calls the same fetch path (`FetchCachedArticles`) independently, so multiple users can run searches at the same time without blocking each other.
- Matching queries share a narrow in-process lock; independent NewsAPI calls still overlap.
- SQLite uses WAL, bounded lock waits, unique keys, and short transactions to support separate invocations sharing the same database. Only one database writer commits at a time; no database transaction spans a network request.
- Article updates and query coverage commit together. The file-cache option remains available.

This pattern supports multiple simultaneous users while preserving predictable cache semantics and avoiding accidental data races between independent searches.

## Invocation method
Use the request fan-out helper when running more than one independent search:

```go
requests := []newsapi.Request{
    {Topic: "cloud computing", Days: 7, Limit: 5},
    {Topic: "machine learning", Days: 7, Limit: 3},
    {Topic: "cybersecurity", Days: 3, Limit: 2},
}

resultsCh, errCh := newsapi.ProcessCachedRequests(".newsapi-cache", requests)

for result := range resultsCh {
    fmt.Printf("%s -> %d articles\n", result.Request.Topic, len(result.Articles))
}

for err := range errCh {
    log.Printf("search failed: %v", err)
}
```

For a single CLI query, you can still invoke the app as before:

```bash
go run . -topic "cloud computing" -days 7 -articles 5
```

The same fetch path is used in both cases; the concurrency helper simply exposes the per-request goroutine/channel model for multi-user workloads.

To run the searches in `searches.example.json` concurrently, run from `Project1`:

```powershell
go run . -batch searches.example.json
```

Docker equivalent, after configuring `.env` as described below:

```powershell
docker compose -p project1 build go-app
docker compose -p project1 run --rm -v "./searches.example.json:/app/searches.example.json:ro" go-app -batch /app/searches.example.json
```

The build includes the latest batch-search code. The read-only mount supplies the JSON file because the runtime image contains only the executable and its runtime dependencies. Compose mounts the persistent SQLite volume automatically.

## Prerequisites
- Docker Desktop or Docker Engine for the Docker commands below. Go, a C compiler, and SQLite are downloaded/built inside Docker; no local installation of them is required.
- A NewsAPI key from https://newsapi.org/.
- For native source builds only: Go 1.25.0+ and a CGO-compatible C compiler (GCC/MinGW on Windows). Use Docker to avoid this setup.

## 1) Configure environment variables
From `Project1`, create `.env` if it does not already exist:

```powershell
Copy-Item .env.example .env
```

If `.env` already exists, edit it rather than overwriting your API key:

```dotenv
NEWSAPI_API_KEY=your_api_key_here
DATABASE_PATH=./data/news.db
DB_TIMEOUT=10s
```

Remove the old `MONGO_URI`, `MONGO_DATABASE`, and `MONGO_TEST_URI` entries. Compose sets `DATABASE_PATH=/data/news.db` inside the container and clears a legacy `MONGO_URI` automatically. For native runs, a nonempty `MONGO_URI` without `DATABASE_PATH` produces migration guidance.

`DATABASE_PATH` selects SQLite. Leaving it empty selects the existing JSON file cache. `-cache` controls the file-cache directory only. The application creates the SQLite database/schema on first use. NewsAPI credentials are supplied at runtime and are excluded from the build context.

## 2) Persistent storage and migration
Compose mounts `sqlite-data` at `/data`, keeping the database, WAL, and shared-memory files together. Repeated `run --rm` invocations use the same data. Use this on a single Docker host; do not put the WAL database on a network filesystem.

The SQLite database starts empty. Existing MongoDB data is not automatically imported or deleted; cached searches refill through NewsAPI. Keep the old Mongo volume if historical data must be recovered. Do not run `docker compose down -v` unless you intend to delete the new SQLite cache.

If an old MongoDB container is still running, stop it explicitly with `docker stop mongodb`. This preserves its volume. For a database backup, stop all application invocations and copy the entire data directory, or use a SQLite-aware backup tool; do not copy only the main file while writers are active.

## 3) Run the app locally or with Docker
Run all commands below from `Project1`. Before using the Docker alternatives, build the app image (repeat after code changes):

```bash
docker compose -p project1 build go-app
```

Compose loads `Project1/.env`, mounts SQLite storage, and runs the built image. Each `run --rm` removes only the temporary container. The final image contains a statically linked, UPX-compressed Go/SQLite executable and CA certificates, with no compiler or shell. See [image optimization notes](docker_image_optimization.md) for measured sizes and the `COMPRESS=0` build option.

With `DATABASE_PATH` configured and your `NEWSAPI_API_KEY` in `.env`, run:

```bash
go run . -topic "cloud computing" -days 7 -articles 5
```

Docker alternative:

```bash
docker compose -p project1 run --rm go-app -topic "cloud computing" -days 7 -articles 5
```

Common flags:

```bash
go run . -q "artificial intelligence" -days 3 -articles 10

go run . -topic "technology" -country us -days 7 -articles 5 -output ./output

go run . -topic "health" -cache .newsapi-cache
```

Docker alternatives for the same searches (the volume mounts keep output and file cache on your machine):

```bash
docker compose -p project1 run --rm go-app -q "artificial intelligence" -days 3 -articles 10

docker compose -p project1 run --rm -v "./output:/app/output" go-app -topic "technology" -country us -days 7 -articles 5 -output ./output

docker compose -p project1 run --rm --no-deps -e DATABASE_PATH= -e MONGO_URI= -v "./.newsapi-cache:/app/.newsapi-cache" go-app -topic "health" -cache /app/.newsapi-cache
```

The file-cache command clears `DATABASE_PATH` and any legacy `MONGO_URI`. For native file caching, clear these values in your environment or `.env` as well.

Available options:
- `-q` : legacy alias for topic
- `-topic` : topic or keyword to search
- `-country` : country code, default `us`
- `-days` : number of days back to search, minimum `1`
- `-articles` : number of articles to return, minimum `1`
- `-output` : optional folder where article JSON files are saved
- `-cache` : folder used for the local file cache
- `-db-timeout` : database operation timeout, default `10s`
- `-api-timeout` : NewsAPI timeout, default `15s`

`DB_TIMEOUT` overrides `-db-timeout`. Deprecated `MONGO_TIMEOUT` and `-mongo-timeout` are accepted only when the new timeout setting is absent; `MONGO_TIMEOUT` overrides the legacy flag. `NEWSAPI_TIMEOUT` overrides `-api-timeout`.

## 4) Run the app in Docker
After building the image in section 3, start the containerized app:

```bash
docker compose -p project1 up --no-build
```

This runs the previously built image using the container environment. The app uses SQLite and exits after its default search; no database service remains running.

## 5) Run tests
Run these commands from `Project1`. The release image has no Go compiler, so build the development stage for tests (repeat after code changes):

```powershell
docker build -f dockerfile --target build -t project1-go-build .
docker run --rm -e DATABASE_PATH= -e MONGO_URI= project1-go-build go test ./... -count=1
docker run --rm -e DATABASE_PATH= -e MONGO_URI= project1-go-build go test -race ./... -count=1
```

Tests use mock NewsAPI responses and temporary databases. They require neither your API key nor a database server. The SQLite tests also check concurrent processes, interrupted transactions, lock timeouts, duplicate prevention, and restart persistence.

Run just the cache tests or the complete SQLite flow:

```powershell
docker run --rm -e DATABASE_PATH= -e MONGO_URI= project1-go-build go test ./newsapi -run 'Test(FetchCachedArticles|ProcessCachedRequests|SQLite)' -v -count=1
docker run --rm -e DATABASE_PATH= -e MONGO_URI= project1-go-build go test ./tests -run TestSQLiteBackedApplicationFlow -v -count=1
```

For native tests with Go and a C compiler installed, set `DATABASE_PATH` and `MONGO_URI` empty, then run `go test ./...`.

### Reproducible fixture cases
The fixture-backed tests use checked-in request/response inputs and expected outputs. They use a fixed cache clock and mocked fetch functions, so they do not require a NewsAPI key, a database server, or network access. Run these commands from `Project1` with `DATABASE_PATH` and `MONGO_URI` unset or empty:

```powershell
go test ./newsapi -run '^TestCacheMissFixture$' -v -count=1
go test ./newsapi -run '^TestCacheHitFixture$' -v -count=1
go test . -run '^TestInvalidBatchFixture$' -v -count=1
go test ./newsapi -run '^TestConcurrentRequestsFixture$' -v -count=1
go test . -run '^TestPrintArticlesGoldenOutput$' -v -count=1
go test ./... -count=1
```

Fixture inputs and expected results are kept together in `newsapi/testdata/cache-case.json`, `newsapi/testdata/concurrent-case.json`, and `testdata/invalid-batch-case.json`. The two CLI output goldens are under `testdata/expected/`. Expected behavior:
- Cache miss: one API fetch; output source is `News API` and the returned title is `Fixture cache article`.
- Cache hit: two identical searches produce sources `News API` then `cache`, with only one API fetch total.
- Invalid batch: the exact validation message is part of `testdata/invalid-batch-case.json`; no fetch is performed.
- Concurrent requests: both fixture requests overlap; normalized results are compared with the expected result in `newsapi/testdata/concurrent-case.json`.
- CLI output: cache-hit and cache-miss text must exactly match their golden files under `testdata/expected/`.

## 6) Custom API queries
You can run live custom searches directly through the app entrypoint:

```bash
go run . -topic "cybersecurity" -days 14 -articles 3
```

Docker alternative:

```bash
docker compose -p project1 run --rm go-app -topic "cybersecurity" -days 14 -articles 3
```

To save the results locally as JSON:

```bash
go run . -topic "machine learning" -days 30 -articles 10 -output ./output
```

Docker alternative (saves JSON in your local `Project1/output` folder):

```bash
docker compose -p project1 run --rm -v "./output:/app/output" go-app -topic "machine learning" -days 30 -articles 10 -output ./output
```

The app checks the configured cache first and only fetches from NewsAPI when needed.

## 7) Useful commands
```powershell
docker compose -p project1 down

docker compose -p project1 logs go-app
docker compose -p project1 ps

go test ./...

# Docker alternative to go test
docker run --rm -e DATABASE_PATH= -e MONGO_URI= project1-go-build go test ./... -count=1
```

## Submission notes
For project submission, build the app for the target architecture you need, and keep the SQLite data volume outside the submitted image. The Dockerfile compiles Go and SQLite in its builder stage and ships only the static executable and HTTPS certificates.

From `Project1`, build an x64 submission image:

```powershell
docker buildx build --platform linux/amd64 --load -f dockerfile -t project1-news .
```

This tags a separate submission image as `project1-news`; the Compose commands above use `project1-go-app`.

For measured sizes and build tradeoffs, see [image optimization notes](docker_image_optimization.md).
