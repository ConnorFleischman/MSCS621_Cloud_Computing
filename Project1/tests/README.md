# To test, run

```cmd
.\bin\newsapi_cli.exe -topic "technology" -days 1 -articles 1 -output .\output\demo
```

To run unit tests in PowerShell from `Project1`, run
```powershell
docker run --rm --mount "type=bind,source=$($PWD.Path),target=/app" -w /app golang:1.22 go test -v ./...
```

Run command in /tests folder, and ensure the env file with the newsorg API key is under /tests

Build the updated CLI with `go build -o tests/bin/newsapi_cli.exe ./tests/cmd/newsapi_cli` from Project1.
Set `MONGO_URI` in the environment or CLI's `.env` to use MongoDB instead of JSON caching. `MONGO_DATABASE` defaults to `news`. Topics are lowercased with whitespace collapsed. The `articles` collection stores article fields and topic/country associations; a unique topic/country/identity index prevents duplicates (URL identity, or a content hash for missing URLs). A topic/publication-date index supports searches. The `coverage` collection stores completed UTC intervals (`from` inclusive, `to` exclusive), including empty results, after article writes succeed. Separate intervals preserve gaps. Existing JSON caches are not migrated.

For Docker, start MongoDB with `docker compose up -d mongodb` from Project1, then run the CLI with `-e MONGO_URI=mongodb://admin:secret@host.docker.internal:27017/`. To run integration tests, set `MONGO_TEST_URI=mongodb://admin:secret@host.docker.internal:27017/` in `Project1/tests/.env` or pass it with `-e` to the Go test container. The test loads `tests/.env` automatically; exported variables take priority. Use `go test -count=1 -v ./...` to force a fresh run. The test creates and drops its own uniquely named database and needs no NewsAPI key.

Without `MONGO_URI`, queries persist in `.newsapi-cache` (override with `-cache`). Covered UTC calendar dates with enough unique, matching articles avoid API calls; wider dates or larger counts trigger paginated fetches and a URL-deduplicated merge. Results are newest first, capped at the requested count. Missing publication dates do not count toward dated requests. An API key is needed only on cache misses.

Topic searches use NewsAPI's `/everything` endpoint for historical dates; country applies only to topic-free `/top-headlines` searches, which provide live headlines rather than historical coverage. Cache updates are serialized within a process; use separate cache folders for concurrent CLI processes. If the API has fewer matching articles than requested, the available results are returned and later requests may retry.
