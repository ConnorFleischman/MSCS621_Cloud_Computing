# To test, run

```cmd
.\bin\newsapi_cli.exe -topic "technology" -days 1 -articles 1 -output .\output\demo
```

To run all tests, from repo root, run
```cmd
docker run --rm --mount "type=bind,source=$($PWD.Path),target=/app" -w /app golang:1.22 go test -v ./...
```

Run command in /tests folder, and ensure the env file with the newsorg API key is under /tests

Build the updated CLI with `go build -o tests/bin/newsapi_cli.exe ./tests/cmd/newsapi_cli` from Project1.
Queries persist in `.newsapi-cache` (override with `-cache`). Covered UTC calendar dates with enough unique, matching articles avoid API calls; wider dates or larger counts trigger paginated fetches and a URL-deduplicated merge. Results are newest first, capped at the requested count. Missing publication dates do not count toward dated requests. An API key is needed only on cache misses.

Topic searches use NewsAPI's `/everything` endpoint for historical dates; country applies only to topic-free `/top-headlines` searches, which provide live headlines rather than historical coverage. Cache updates are serialized within a process; use separate cache folders for concurrent CLI processes. If the API has fewer matching articles than requested, the available results are returned and later requests may retry.
