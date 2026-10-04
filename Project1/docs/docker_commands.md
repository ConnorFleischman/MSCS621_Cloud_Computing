# Docker usage

Run commands from `Project1` (`cd Project1` from the repository root). Docker builds Go and SQLite together; you do not need Go, a C compiler, or SQLite installed locally.

Create `.env` from `.env.example` if needed, then set `NEWSAPI_API_KEY`. Preserve your existing key if updating an existing file. Replace old Mongo settings with `DATABASE_PATH=./data/news.db` for native runs. Compose uses `/data/news.db` and a persistent named volume automatically.

Build and search:

```powershell
docker compose -p project1 build go-app
docker compose -p project1 run --rm go-app -topic technology -days 2 -articles 5
```

Repeat the second command for another search. SQLite stores articles and query coverage in `project1_sqlite-data`; the CLI exits after each search. There is no separate database container.

Save article JSON files on your computer:

```powershell
docker compose -p project1 run --rm -v "./output:/app/output" go-app -topic "cloud computing" -days 7 -articles 5 -output /app/output
```

Run concurrent searches from a batch file:

```powershell
docker compose -p project1 run --rm -v "./searches.example.json:/app/searches.example.json:ro" go-app -batch /app/searches.example.json
```

Use the existing JSON file cache instead of SQLite:

```powershell
docker compose -p project1 run --rm -e DATABASE_PATH= -e MONGO_URI= -v "./.newsapi-cache:/app/.newsapi-cache" go-app -topic health -days 7 -articles 5 -cache /app/.newsapi-cache
```

A `-cache` path alone does not select file caching while `DATABASE_PATH` is set.

Run the default search:

```powershell
docker compose -p project1 up --no-build
```

The release image contains no Go compiler or shell. Build the development stage for tests, repeating the build after source changes:

```powershell
docker build -f dockerfile --target build -t project1-go-build .
docker run --rm -e DATABASE_PATH= -e MONGO_URI= project1-go-build go test ./... -count=1
docker run --rm -e DATABASE_PATH= -e MONGO_URI= project1-go-build go test -race ./... -count=1
docker run --rm -e DATABASE_PATH= -e MONGO_URI= project1-go-build go test ./tests -run TestSQLiteBackedApplicationFlow -v -count=1
```

Tests use mock API responses and temporary SQLite databases, including the integration tests. No database service or real API key is needed.

Inspect status and stop services while retaining data:

```powershell
docker compose -p project1 ps
docker compose -p project1 logs go-app
docker compose -p project1 down
```

`run --rm` logs appear in that command's terminal; removed containers do not retain logs for `compose logs`. Avoid `down -v` unless deleting SQLite data is intentional.

The old MongoDB volume is not converted or removed. The new cache starts empty and refills from NewsAPI. If an old `mongodb` container is still running, `docker stop mongodb` stops it without deleting its data.

See [the README](Project1/README.md) for configuration and [submission commands](Project1/README.md#submission-notes).
