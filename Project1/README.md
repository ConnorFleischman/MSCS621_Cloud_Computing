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
- Stored tests: `tests/newsapi/`
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
- Go 1.22+
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
- `MONGO_TEST_URI` is used for the Mongo integration test when you want to run the DB-backed checks locally.

## 2) Start MongoDB with Docker
From the project root:

```bash
docker compose up -d mongodb
```

This starts MongoDB on port `27017` with the configured admin credentials.

To confirm it is healthy:

```bash
docker compose ps
```

You can also inspect logs:

```bash
docker compose logs -f mongodb
```

## 3) Run the app locally
With Mongo configured and your `NEWSAPI_API_KEY` in `.env`, run:

```bash
go run . -topic "cloud computing" -days 7 -articles 5
```

Common flags:

```bash
go run . -q "artificial intelligence" -days 3 -articles 10

go run . -topic "technology" -country us -days 7 -articles 5 -output ./output

go run . -topic "health" -cache .newsapi-cache
```

Available options:
- `-q` : legacy alias for topic
- `-topic` : topic or keyword to search
- `-country` : country code, default `us`
- `-days` : number of days back to search, minimum `1`
- `-articles` : number of articles to return, minimum `1`
- `-output` : optional folder where article JSON files are saved
- `-cache` : folder used for the local file cache

## 4) Run the app in Docker
Build and start the containerized app:

```bash
docker compose up --build
```

This will build the Go app and run it using the container environment. The app is configured to use the MongoDB service inside Docker by default.

## 5) Run stored tests
Run all Go tests:

```bash
go test ./...
```

Run the tests in verbose mode to see live output:

```bash
go test ./... -v
```

Run only the cached API tests:

```bash
go test ./tests/newsapi -v
```

If you want to run the MongoDB integration test, set `MONGO_TEST_URI` in your environment or `.env` before executing:

```bash
go test ./tests/newsapi -run TestMongoCache -v
```

## 6) Custom API queries
You can run live custom searches directly through the app entrypoint:

```bash
go run . -topic "cybersecurity" -days 14 -articles 3
```

To save the results locally as JSON:

```bash
go run . -topic "machine learning" -days 30 -articles 10 -output ./output
```

The app checks the configured cache first and only fetches from NewsAPI when needed.

## 7) Useful commands
```bash
docker compose down

docker compose logs -f
docker compose ps

go test ./...
```

## Submission notes
For project submission, build the app for the target architecture you need, and keep the Docker setup consistent with the MongoDB service defined in `docker-compose.yml`.
