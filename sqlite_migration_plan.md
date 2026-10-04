# MongoDB to SQLite migration plan

Status: implemented with a deliberately small storage-layer change. The existing optional database hook now takes `*sql.DB`; the cache algorithm and file backend remain in place, avoiding a new storage abstraction or connection-pool lifecycle across callers. Each request closes its own database handle, matching the previous Mongo connection ownership.

The C-based static SQLite build with UPX was selected after comparing pure-Go, dynamic, static, and compressed candidates. See [measured results and verification](docker_image_optimization.md) and [current commands](README.md). Existing MongoDB data is preserved but not imported; historical cache import remains an optional separate task. The numbered sections below retain the original design/acceptance plan for reference.

## Objective and target

Preserve the existing news search, persistence, caching, CLI output, and concurrent invocation behavior while minimizing the complete runnable system's image footprint. Keep Docker, replace the MongoDB service with SQLite embedded in the Go executable, and persist the database outside the image.

Target: one application container, one executable, CA certificates for HTTPS, and a Docker named volume mounted at `/data`. Store the database at `/data/news.db`. No database server, SQLite CLI, Go compiler, C compiler, source, test fixtures, or package manager belongs in the release image.

Select the smallest measured build that passes all acceptance tests. An absolute theoretical minimum cannot be promised; document the tested candidates and results rather than claiming an unmeasured minimum. Default comparison platform is `linux/amd64`; measure other required platforms separately.

## 1. Establish the baseline and behavior contract

- Record the current application and resolved MongoDB image digests, platform, compressed layer sizes, and unpacked image sizes. Report the combined runtime footprint with shared layers counted once. Record database volume usage and builder cache separately.
- Run existing unit/fixture tests and the MongoDB integration test against a disposable database. Capture representative CLI output and API request counts before changing storage.
- Preserve topic/country normalization, UTC date windows including today, newest-first results, result limits, optional article fields, JSON exports, batch input, partial batch failures, and API/cache source reporting.
- Preserve the 15-minute cache lifetime, date coverage lookup, refresh after expiry, larger-limit/date-range fetching, short/empty-result caching, pagination, and repeated-page error handling.
- Preserve duplicate prevention by URL or content, country/topic isolation, overlapping searches, and concurrent independent API requests.
- Preserve the existing explicit file-cache workflow and its `-cache` directory option. SQLite replaces the configured database backend; file caching remains available.

## 2. Introduce a small storage boundary

Refactor `newsapi/cache.go` so the common fetch/coverage logic uses a small load/save storage interface instead of the optional `*mongo.Database` parameter. Keep pagination and coverage decisions in the common logic. Provide file and SQLite implementations; remove the Mongo implementation after parity is demonstrated.

Use Go's `database/sql` for SQLite access. Own and close connection pools explicitly; reuse a pool within a CLI batch rather than opening a connection for each SQL statement. Keep the existing public fetch helpers usable with temporary database paths in tests. Resolve database paths consistently for in-process cache locking.

Replace BSON hashing with a deterministic representation of article content. Normalize timestamps to UTC and an explicitly chosen precision compatible with the existing Mongo millisecond round-trip. Test missing dates, equivalent time zones, sub-millisecond timestamps, empty URLs, and persistence round-trips before removing BSON. Do not change API JSON field names.

## 3. Implement the SQLite schema and persistence

Create schema version 1 automatically on first use, inside a transaction. Track schema version with `PRAGMA user_version`, and reject unknown newer versions clearly.

| Table | Fields and constraints |
| --- | --- |
| `articles` | Topic, country, article key, nullable publication timestamp, and complete article JSON; primary key `(topic, country, article_key)` |
| `coverage` | Topic, country, start/end timestamps, fetched timestamp, exhausted flag, requested limit; primary key `(topic, country, from_time, to_time)` |

Use UTC integer timestamps with consistent precision and half-open date intervals. Add an article index beginning with topic/country and publication time, plus a coverage index appropriate to the containing-interval lookup. Preserve the existing selection of the most recently fetched matching coverage record. Store full article JSON without requiring SQLite JSON extensions.

Use parameterized SQL and UPSERT. Commit article updates and corresponding coverage metadata in one transaction: failures must not leave coverage claiming that missing articles were saved. Read related coverage and article data in a consistent snapshot. Finish reads and close cursors before starting writes.

Enable WAL, a bounded busy timeout on every connection, and explicit durable synchronization settings. Retain thread safety and file locking. Keep transactions short; never hold a database transaction across NewsAPI HTTP calls. Bound lock waits/retries by the database operation timeout and return understandable errors.

Keep per-query in-process locks for concurrent miss coalescing. SQLite transactions and unique constraints protect separate processes sharing the database; do not assume Go mutexes protect separate containers. Cross-process duplicate API calls may still occur, as with the current implementation; correctness must not depend on avoiding them.

WAL allows concurrent readers and a writer, but only one writer at a time. Use a Docker named volume on one Docker host, not a network filesystem. Mount the whole directory so database, WAL, and shared-memory files stay together. This preserves this project's local multi-invocation workflow, not MongoDB's remote database server capability. [SQLite WAL documentation](https://sqlite.org/wal.html)

## 4. Update configuration and remove MongoDB coupling

- Add `DATABASE_PATH`; a nonempty value selects SQLite, and an unset/empty value retains file-cache behavior. Compose supplies `/data/news.db` by default. Local instructions provide a local path explicitly.
- Introduce `DB_TIMEOUT`, `-db-timeout`, and a database-neutral request timeout field. Keep the old Mongo timeout flag/environment names as deprecated aliases for transition; new names take precedence. Define and test precedence explicitly.
- Remove Mongo connection/credential settings from examples and Compose. If a legacy `MONGO_URI` is set without SQLite configuration, report migration guidance rather than silently switching to an empty cache.
- Remove Mongo-specific types from `models/news.go` and imports from `cache.go`, integration tests, and other packages. Preserve externally visible JSON where used; replace or remove unused Mongo-only records after checking references.
- Run `go mod tidy` and verify the production dependency graph contains no Mongo driver or BSON package. Do not retain both SQLite drivers in the selected release build.
- Leave the Go module name alone unless a separate rename is useful; its current name does not increase the shipped binary meaningfully.

## 5. Compare builds and minimize the release image

Build the same functionality and run the same tests for each candidate. Pin compatible Go, driver, and builder versions and record them.

| Candidate | Build approach | Final runtime |
| --- | --- | --- |
| A | `modernc.org/sqlite`, `CGO_ENABLED=0`, stripped Go executable | `scratch` plus executable and CA bundle |
| B | `mattn/go-sqlite3`, CGO enabled, Alpine/musl builder, explicitly static external linking | `scratch` plus executable and CA bundle |
| C | C-based driver with dynamic linking | Minimal compatible runtime and required libraries, only if its total is smaller |

The pure-Go driver avoids a C compiler. The C-based driver requires CGO and a compiler, which can be installed only inside the Docker builder. Neither requires a SQLite installation on the user's computer when building/running through Docker. [Pure-Go driver](https://pkg.go.dev/modernc.org/sqlite), [C-based driver](https://github.com/mattn/go-sqlite3)

Use `-trimpath` and `-ldflags="-s -w"` for release Go builds. For the C candidate, benchmark size optimization such as `-Os` and supported options to omit unused extension loading/features. Keep transactions, WAL, synchronization, locking, and all functionality exercised by the application. Do not disable durability or thread safety for size. Validate candidate flags against the selected driver and SQLite version. [SQLite compile options](https://sqlite.org/compile.html)

Verify claimed static executables using ELF inspection in the builder and by running in `scratch`. Exercise DNS and HTTPS, not just CLI startup. Provide writable `/data`, cache/output paths, and a writable temporary directory if SQLite requires one; test volume ownership with the chosen runtime UID.

Add `.dockerignore` entries for `.env`, database/WAL files, generated output, caches, and local binaries. Keep required test fixtures available to the test stage. This reduces context and accidental inclusion; distinguish those benefits from final runtime size savings.

After choosing the smallest ordinary build, optionally benchmark executable compression if supported for that artifact. Accept it only if compressed image/archive size also improves and startup, HTTPS, concurrency, and database tests pass. Record CPU/memory/startup costs; a smaller executable alone does not prove a smaller distributed image.

Produce a comparison table of binary bytes, unpacked runtime bytes, compressed image bytes, compressed submission archive bytes, and baseline percentage reductions. Choose using compressed submission size as the primary metric if submitting an archive; also report unpacked and registry sizes. If rankings differ, document the tradeoff rather than calling one universally smallest. Keep all benchmark tooling out of the release image.

## 6. Simplify Compose and preserve persistent data

Remove the MongoDB service, database port, credentials, health check, and dependency. Retain the `go-app` service, `.env` injection for NewsAPI, existing CLI argument handling, and output/batch file mounts. Add a dedicated `sqlite-data:/data` named volume. Concurrent `docker compose run --rm go-app ...` invocations must share that volume.

Do not delete the existing Mongo volume or run `docker compose down -v` during migration. The new volume starts empty unless data is imported. Functionality parity and historical data preservation are separate concerns.

If existing cache data must carry over, use a one-off migration tool in a separate development target: export articles/coverage, convert BSON dates and recompute content keys, then import transactionally into SQLite. Verify counts, unique keys, date ranges, and sample queries. Keep Mongo dependencies out of the release executable. Stop application writes during the final export/cutover. Preserve Mongo data for rollback; later SQLite writes are not automatically synchronized back.

## 7. Verify parity and image behavior

- Port `tests/mongo_flow_test.go` to an always-runnable SQLite test with a temporary on-disk database and mock NewsAPI. Preserve its assertions for repeat hits, expanded limits/date ranges, independent parallel searches, mixed limits, overlapping windows, short results, and expiry.
- Run existing CLI golden-output, batch, API, pagination, timeout, and file-cache tests. Run shared cache cases against SQLite where applicable.
- Add storage-focused tests for rollback, duplicate upserts, optional fields, timestamp/hash round-trips, initialization races, schema version checks, country isolation, lock timeout/cancellation, and error recovery.
- Use separate processes, not only goroutines, to test simultaneous writes and reads of one database. Confirm no lost records, duplicate identities, partial coverage, or database corruption.
- Test restart persistence and reuse across fresh `--rm` containers; kill a writer during a transaction and verify recovery/integrity without losing committed data.
- Run `go test ./...`, the race detector in a compatible development build, and the integration/concurrency suite against the exact chosen release build where feasible. The race-enabled executable is not shipped.
- Smoke-test DNS/HTTPS with certificate verification, single searches, batch searches, JSON exports, mounted file inputs, file-cache mode, and invalid CLI arguments in the final minimal image.
- Inspect final layers for unexpected files and confirm that the compiler, source, credentials, database contents, and Mongo packages are absent.

## 8. Documentation and completion criteria

Update `README.md`, `.env.example`, Docker command examples, and `docker_image_optimization.md`. Remove stale Go build-context overrides and Mongo setup steps. Explain the database path, volume persistence, backup using a SQLite-consistent method or stopped app, concurrency limitations, and Docker-only prerequisites. Native Windows source builds with the C driver need a compatible compiler; Docker builds do not.

The migration is complete when behavior tests pass, persistence works across independent invocations, only one runtime image is needed, the chosen image wins the documented size comparison, and a clean Docker build/run succeeds using the published instructions. Publish actual measurements, commands, platform, and limitations.

Implementation order: baseline -> storage boundary and SQLite parity -> configuration and Compose -> build comparison and optimization -> final-container validation -> documentation and cutover. Do not let image optimization obscure correctness failures.

Execution note: Docker Desktop was subsequently located outside the normal sandbox PATH and used for builds, tests, and size measurements. No Go, C compiler, or SQLite installation was required on the host. The prior Mongo integration test was not run; its assertions were migrated to SQLite and passed. Optional historical-data import was not implemented to keep the change focused.
