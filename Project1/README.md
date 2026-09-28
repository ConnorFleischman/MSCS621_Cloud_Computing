# NewsAPI Project

This project is a Go-based news search app that fetches stories from NewsAPI, caches results, and stores query coverage in MongoDB when configured.

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
- Stored tests: `tests/`
- MongoDB container: `docker-compose.yml`

## Concurrency design
The service is designed to process independent news searches in parallel using goroutines and channels.

- Each request is wrapped in its own goroutine via `newsapi.ProcessRequests(...)`.
- The function sends successful results over a `results` channel and failures over an `errors` channel.
- Each goroutine calls the same fetch path (`FetchCachedArticles`) independently, so multiple users can run searches at the same time without blocking each other.
- The file cache still uses a narrow mutex guard (`cacheMu`) to prevent concurrent writes to the same on-disk cache file.
- MongoDB and NewsAPI access are isolated per request: each request creates its own timeout-bound context and client connection, which lets Mongo's connection pool and the HTTP client handle concurrency safely.

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

## Prerequisites
- Go 1.25.0+ (the module's minimum version)
- Docker Desktop or Docker Engine
- A NewsAPI key from https://newsapi.org/

## 1) Configure environment variables
Create a `.env` file in the project root using the example file:

```bash
cp .env.example .env
```

Then update `.env` with your values:

```env
NEWSAPI_API_KEY=your_newsapi_key_here
MONGO_URI=mongodb://admin:secret@localhost:27017/
MONGO_DATABASE=news
MONGO_TEST_URI=mongodb://admin:secret@localhost:27017/
```

Important host note:
- When running the Go app directly on your machine, use `localhost`.
- When the app runs inside the Docker Compose network, use `mongodb` instead.
- Do not use `host.docker.internal` for a local Windows host run; it is not resolvable outside Docker Desktop.

Example values:

```env
# Local machine execution
MONGO_URI=mongodb://admin:secret@localhost:27017/

# Docker Compose execution inside the app container
MONGO_URI=mongodb://admin:secret@mongodb:27017/
```

Notes:
- `NEWSAPI_API_KEY` is required for live API requests.
- `MONGO_URI` is used by the app when Mongo cache is enabled.
- `MONGO_TEST_URI` is not used by the current tests.

## 2) Start MongoDB with Docker
From `Project1` (run `cd Project1` first if your terminal is at the repository root):

```bash
docker compose -p project1 up -d mongodb
```

This starts MongoDB on port `27017` with the configured admin credentials.

To confirm it is healthy:

```bash
docker compose -p project1 ps
```

You can also inspect logs:

```bash
docker compose -p project1 logs -f mongodb
```

## 3) Run the app locally or with Docker
Run all commands below from `Project1`. Before using the Docker alternatives, build the app image (repeat after code changes):

```bash
docker buildx build --load --build-context golang:1.22-alpine=docker-image://golang:1.25-alpine -f dockerfile -t project1-go-app .
```

Compose loads `Project1/.env`, sets `MONGO_URI` to the `mongodb` service, and starts MongoDB automatically when running the app. Each `run --rm` command removes the app container after it exits.

The Dockerfile selects Go 1.22 with automatic toolchain upgrades disabled (`GOTOOLCHAIN=local`), while the module requires Go 1.25. The build command uses a [Docker build-context override](https://docs.docker.com/reference/cli/docker/buildx/build/#additional-build-contexts---build-context) to select Go 1.25 without changing the Dockerfile. The tag `project1-go-app` matches the image expected by `docker compose -p project1`. Rebuild using that command after code changes; ordinary `docker compose build` or `up --build` would select Go 1.22 again.

The Dockerfile's `COPY . .` includes `.env` if present; there is currently no `.dockerignore` to exclude it.

With Mongo configured and your `NEWSAPI_API_KEY` in `.env`, run:

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

docker compose -p project1 run --rm --no-deps -e MONGO_URI= -v "./.newsapi-cache:/app/.newsapi-cache" go-app -topic "health" -cache /app/.newsapi-cache
```

The file-cache command clears `MONGO_URI` because the app selects MongoDB whenever that variable is nonempty. For a local `go run` using file caching, also set `MONGO_URI` to an empty value in your environment or `.env`.

Available options:
- `-q` : legacy alias for topic
- `-topic` : topic or keyword to search
- `-country` : country code, default `us`
- `-days` : number of days back to search, minimum `1`
- `-articles` : number of articles to return, minimum `1`
- `-output` : optional folder where article JSON files are saved
- `-cache` : folder used for the local file cache

## 4) Run the app in Docker
After building the image in section 3, start the containerized app:

```bash
docker compose -p project1 up --no-build
```

This runs the previously built image using the container environment. The app uses the MongoDB service inside Docker and exits after its default search; MongoDB remains running.

## 5) Run stored tests
The Docker alternatives use `--entrypoint go` to run the Go tool included in the image instead of the app. Rebuild the image after changing source code or tests.

The current cache tests expect file caching. Docker test commands clear `MONGO_URI` and skip starting MongoDB; for local tests, ensure `MONGO_URI` is unset or empty in the shell.

Run all Go tests:

```bash
go test ./...
```

Docker alternative:

```bash
docker run --entrypoint go project1:latest test ./...
```

Run the tests in verbose mode to see live output:

```bash
go test ./... -v
```

Docker alternative:

```bash
docker run --entrypoint go project1:latest test ./... -v
```

Run only the cached API tests:

```bash
go test ./tests -run 'Test(FetchCachedArticles|ProcessCachedRequests)' -v
```

Docker alternative:

```bash
docker run --entrypoint go project1:latest test ./tests -run 'Test(FetchCachedArticles|ProcessCachedRequests)' -v
```

There is currently no `TestMongoCache` integration test in `tests/`.

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
```bash
docker compose -p project1 down

docker compose -p project1 logs -f
docker compose -p project1 ps

go test ./...

# Docker alternative to go test
docker run --entrypoint go project1:latest test ./...
```

## Submission notes
For project submission, build the app for the target architecture you need, and keep the Docker setup consistent with the MongoDB service defined in `docker-compose.yml`.
