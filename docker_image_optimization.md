# Smaller image: embedded SQLite and a static executable

The application now embeds SQLite instead of requiring a MongoDB server image. The cache algorithm, CLI, JSON exports, goroutines/channels, and optional file cache remain. SQLite saves articles and coverage in a transaction and persists them in the Compose `sqlite-data` volume.

## Selected build

The Dockerfile compiles `github.com/mattn/go-sqlite3` with Go and a C compiler inside an Alpine builder, then copies only the executable and CA certificates to `scratch`. The host only needs Docker. Build dependencies and UPX stay in the builder.

Release flags use `-trimpath`, `-s -w`, static linking, `netgo`, `osusergo`, `sqlite_omit_load_extension`, C `-Oz`, and function/data section collection. UPX `--best --lzma` compresses the executable; the build verifies it with `upx -t`. Thread safety, WAL, transactions, locking, and durable synchronization remain enabled.

To disable executable compression for debugging or a runtime that disallows packed executables:

```powershell
docker build --build-arg COMPRESS=0 -f dockerfile -t project1-go-app .
```

The final image has no shell, package manager, compiler, source, `.env`, SQLite CLI, or database contents. Mount data at `/data`; `/tmp` is writable. The existing root execution model is retained to avoid changing bind-mount permissions as part of this migration.

## Measurements

Measured on Linux/amd64 through Docker Desktop, October 1, 2026 (local time). Builder: Go 1.25.14, GCC 15.2.0, UPX 5.2.0; C driver v1.14.52. The tested `golang:1.25-alpine` base resolved to `sha256:1ae0735f00daffa3aaf1363a5184c0d2dc55c78e3db4ec70241cdac97bf84b59`. Image tags and package repositories can change; rebuild measurements may differ.

| Candidate | Executable bytes | Gzip-compressed executable bytes |
| --- | ---: | ---: |
| Pure Go (`modernc.org/sqlite` v1.59.0), stripped | 10,277,048 | 4,464,386 |
| Pure Go, UPX compressed | 3,387,260 | 3,387,572 |
| C SQLite, dynamically linked, `-Os` | 7,354,008 | 3,227,530 |
| C SQLite, static, `-Os` | 7,386,344 | 3,246,384 |
| C SQLite, static, `-Oz` and unused sections removed | 7,341,224 | 3,230,552 |
| **Selected: previous row plus UPX** | **2,470,712** | **2,471,273** |

The dynamic candidate also requires the 670,312-byte musl loader/runtime; its small executable reduction does not justify including that runtime. The pure-Go candidate passed the same ordinary Go tests in a temporary comparison checkout, but produced a larger artifact. Only the selected driver is retained in the project.

| Runtime setup | Docker content size | Docker local disk usage |
| --- | ---: | ---: |
| Previous Go application | 11.3 MB | 35 MB |
| Previous MongoDB image | 339 MB | 1.3 GB |
| SQLite, static `-Os`, without UPX | 3.42 MB | 11 MB |
| **Selected SQLite image (entire runtime system)** | **2.58 MB** | **5.28 MB** |

Docker's containerd image store reports content and disk usage separately; disk usage includes compressed and unpacked storage. Do not present either as the sum of plain executable sizes. Using inspected content sizes, the selected image is approximately **99.3% smaller than the old Go plus MongoDB images combined**. Builder caches and persistent database data are excluded from this runtime comparison.

The selected image's layer files total 2,650,071 bytes. A `docker image save` archive measured 2,595,328 bytes; gzipping that archive measured 2,581,164 bytes. The uncompressed `-Os` candidate's save archive measured 3,430,912 bytes (3,412,137 bytes after gzip). These exports use this Docker installation's compressed layer format; other engines/export formats can differ.

Compression adds startup work: three invalid-argument startup measurements, excluding Docker startup, were 0.02-0.03 seconds and roughly 5 MB peak RSS unpacked, versus 0.14-0.15 seconds and roughly 7 MB packed. These are small local samples, not application throughput benchmarks. The compressed build is the default because minimum image size is the priority. This is the smallest validated candidate tested, not a claim of an absolute theoretical minimum.

## Build, run, and test

From `Project1`, configure `.env` as described in the [README](README.md), then:

```powershell
docker compose -p project1 build go-app
docker compose -p project1 run --rm go-app -topic "cloud computing" -days 7 -articles 5
```

SQLite is embedded; there is no database service to start. Repeated runs share `project1_sqlite-data`. Existing MongoDB volumes are preserved but not imported. The new cache starts empty and refills through NewsAPI.

The runtime has no Go compiler. Build the development stage to run tests:

```powershell
docker build -f dockerfile --target build -t project1-go-build .
docker run --rm -e DATABASE_PATH= -e MONGO_URI= project1-go-build go test ./... -count=1
docker run --rm -e DATABASE_PATH= -e MONGO_URI= project1-go-build go test -race ./... -count=1
```

Rebuild the development image after changing source or tests. The integration test runs automatically with mock HTTP responses and a temporary SQLite file; it no longer needs `MONGO_TEST_URI`.

## Verification

- Existing tests passed before migration; the old optional MongoDB integration test was skipped without its test URI.
- All migrated Go tests and the race detector passed. Checks include existing cache/CLI behavior, transactional rollback, timeout handling, unknown schema versions, nullable dates, identity round-trips, country isolation, six concurrent writer processes, and recovery after killing a writer mid-transaction.
- The actual compressed `scratch` image passed HTTPS mock requests, DNS, cache miss/hit across removed containers, six simultaneous containers sharing one database, batch input and JSON exports, file-cache mode, and invalid-argument handling.
- A request to real NewsAPI with a deliberately invalid key returned the expected HTTP 401, verifying the shipped public CA bundle and DNS without using a real API key. No live successful NewsAPI query was needed for validation.
- Compose configuration validates. The Go dependency graph contains no Mongo driver or BSON package.

The previous application image is retained as `project1-go-app:mongo-backup`; `project1-go-app:latest` now contains SQLite. Old images and builder caches may remain on your computer for rollback/rebuilds. The reduced image size does not automatically reclaim that storage; no existing MongoDB data was deleted.
