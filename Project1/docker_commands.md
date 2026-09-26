<!-- Docker commands for building and running the news CLI with MongoDB. -->
# Docker usage

Run these commands from `Project1`. Put your NewsAPI key in `tests/.env`:

```dotenv
NEWSAPI_API_KEY=your_api_key_here
```

Compose loads that file at runtime and sets `MONGO_URI` to the `mongodb` service. The image does not include `.env` files. `MONGO_DATABASE` defaults to `news`.

Build the CLI and search (MongoDB starts automatically and must become healthy first):

```powershell
docker compose build go-app
docker compose run --rm go-app -topic technology -days 2 -articles 5
```

Repeat the second command for another search. MongoDB persists cached articles in its named volume. The CLI exits after each search; this is expected.

To start both services and run the CLI with its default search:

```powershell
docker compose up --build
```

Inspect CLI options, service status, or database logs:

```powershell
docker compose run --rm --no-deps go-app -help
docker compose ps
docker compose logs mongodb
```

Stop the services while retaining cached articles:

```powershell
docker compose down
```

For an x64 submission image:

```powershell
docker buildx build --platform linux/amd64 --load -t project1-news .
```
