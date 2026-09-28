<!-- Docker commands for building and running the news CLI with MongoDB. -->
# Docker usage

Run these commands from `Project1`, which contains `go.mod`, `main.go`, `dockerfile`, and `docker-compose.yml`. If your terminal is at the repository root, first run `cd Project1`. Put your NewsAPI key in the existing `.env`:

```dotenv
NEWSAPI_API_KEY=your_api_key_here
```

Compose loads that file at runtime and sets `MONGO_URI` to the `mongodb` service. `MONGO_DATABASE` defaults to `news`. The current Dockerfile copies the project directory, including `.env` if present; `.gitignore` does not exclude files from Docker builds.

The module requires Go 1.25.0 or newer, but the Dockerfile selects Go 1.22 with `GOTOOLCHAIN=local`, which disables automatic upgrades. The build command below uses a [Docker build-context override](https://docs.docker.com/reference/cli/docker/buildx/build/#additional-build-contexts---build-context) to select Go 1.25 without editing the Dockerfile. It loads the result as `project1-go-app`, the image name used by Compose with `-p project1`. Use this build command again after code changes; ordinary `docker compose build` or `up --build` would use Go 1.22 again.

Build the CLI and search (MongoDB starts automatically and must become healthy first):

```powershell
docker buildx build --load --build-context golang:1.22-alpine=docker-image://golang:1.25-alpine -f dockerfile -t project1-go-app .
docker compose -p project1 run --rm go-app -topic technology -days 2 -articles 5
```

Repeat the second command for another search. MongoDB persists cached articles in its named volume. The CLI exits after each search; this is expected.

Save article JSON files in `Project1/output`:

```powershell
docker compose -p project1 run --rm -v "./output:/app/output" go-app -topic "cloud computing" -days 7 -articles 5 -output /app/output
```

Use the local file cache instead of MongoDB, keeping it in `Project1/.newsapi-cache` between runs:

```powershell
docker compose -p project1 run --rm --no-deps -e MONGO_URI= -v "./.newsapi-cache:/app/.newsapi-cache" go-app -topic health -days 7 -articles 5 -cache /app/.newsapi-cache
```

The empty `MONGO_URI` overrides Compose's database setting. A `-cache` path alone does not select file caching while `MONGO_URI` is set.

After building the image above, start both services and run the CLI with its default search:

```powershell
docker compose -p project1 up --no-build
```

Run the existing tests from `tests/` using the Go tool included in the image (rebuild after source or test changes):

```powershell
docker compose -p project1 run --rm --no-deps -e MONGO_URI= --entrypoint go go-app test ./... -v
docker compose -p project1 run --rm --no-deps -e MONGO_URI= --entrypoint go go-app test ./tests -run 'Test(FetchCachedArticles|ProcessCachedRequests)' -v
```

These tests exercise file caching, so they must run with `MONGO_URI` empty. There is currently no `TestMongoCache` integration test.

Inspect service status or database logs:

```powershell
docker compose -p project1 ps
docker compose -p project1 logs mongodb
```

Stop the services while retaining cached articles:

```powershell
docker compose -p project1 down
```

For an x64 submission image:

```powershell
docker buildx build --platform linux/amd64 --load --build-context golang:1.22-alpine=docker-image://golang:1.25-alpine -f dockerfile -t project1-news .
```

This tags a separate submission image; the Compose commands above use `project1-go-app`.
