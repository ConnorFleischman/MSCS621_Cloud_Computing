# Smaller Docker image: multi-stage Alpine runtime

The Dockerfile now builds the application in a Go image and copies only the
compiled executable into a separate Alpine runtime image. The final image no
longer contains the Go compiler, downloaded Go modules, build cache, source, or
tests. Alpine retains a shell for troubleshooting.

## Changes

- Named the existing `golang:1.25-alpine` stage `build`.
- Set `CGO_ENABLED=0` during compilation so the executable does not require C
  libraries from the builder at runtime.
- Added an `alpine:3.24` runtime stage with CA certificates for NewsAPI HTTPS
  requests and other TLS connections.
- Copied `/app/app` from the builder, preserving `WORKDIR /app` and
  `ENTRYPOINT ["./app"]` so CLI flags and cache/output mounts work as before.

No application code or Compose configuration changed. Build flags that strip
debug information were not added. The existing `COPY . .` still copies the build
context into the builder, but only the executable reaches the final image.

## Normal build and run commands

Run these commands from `Project1`:

```powershell
docker build -f dockerfile -t project1-go-app .
docker compose -p project1 up -d mongodb
docker compose -p project1 run --rm go-app -topic "cloud computing" -days 7 -articles 5
```

Rebuild once to replace the old image, then rebuild after source changes as usual.
Compose can also build the image with `docker compose -p project1 build go-app`.
The README's older Go 1.22 build-context override is unnecessary: the builder
already uses Go 1.25.

Normal Compose run, volume-mount, logs, and shutdown commands are unchanged.
Keep `Project1/.env` on the host: Compose's `env_file` supplies its values at
runtime. The final image no longer includes `.env`. For a direct `docker run`,
pass `--env-file .env` and configure the appropriate MongoDB network/hostname.

## Test commands

The final image no longer has `go` or test source, so commands using
`--entrypoint go go-app test ...` must use a development container instead.
The README's existing PowerShell test command still works:

```powershell
docker run --rm --mount "type=bind,source=$PWD,target=/app" -w /app -e MONGO_URI= golang:1.25-alpine go test ./... -count=1
```

Alternatively, build the named builder stage to test the source included in that
build. Use a separate tag to preserve the small runtime image:

```powershell
docker build -f dockerfile --target build -t project1-go-build .
docker run --rm -e MONGO_URI= project1-go-build go test ./... -count=1
```

The MongoDB integration test still requires `MONGO_TEST_URI` and access to a
disposable MongoDB server, as described in the README. Local `go run` and
`go test` commands are unchanged.

## Size and local storage

Verification build measured with `docker image ls`:

| Image | Disk usage | Content size |
| --- | --- | --- |
| Existing `project1-go-app:latest` | 768 MB | 204 MB |
| New `project1-go-app:multistage-check` | 34.9 MB | 11.2 MB |

This is approximately a 95% reduction in both reported measures. The verification
build used a separate tag; rebuild with the normal command above to update the
image used by Compose. Sizes can vary with platform, dependencies, and base-image
updates.

The image built successfully. A temporary container confirmed certificates are
present, `/app` is writable, and `.env` and the Go compiler are absent. The CLI
started and rejected `-days 0` with its expected validation message and exit code
1. Live NewsAPI and MongoDB requests were not exercised by this startup check.

Use `docker image ls` to compare the rebuilt runtime image with the old image.
The reduction affects the Go application image; the separate MongoDB image is
unchanged. Docker may retain builder layers as build cache, so a smaller runtime
image does not automatically reclaim all local Docker storage.

## On using docker compose and mongo image (AI Response)
**The most reasonable reading is that the size requirement applies to your Go application image. The PDF does not explicitly require shrinking MongoDB—but it does not define whether supporting images count toward grading.**

On page 1 of [the assignment PDF](C:/Users/daspa/OneDrive/Documents/MaristGraduateSemesters/FALL26/CloudComputing/MSCS621_Cloud_Computing/mscs621fa26Proj1.pdf), it says:

> “A Docker image, docker file, and source code need to be submitted separately.”

It then says the Dockerfile and source must produce the submitted image, followed by:

> “The size of the docker image should be minimal.”

That sequence points to **the image you build from your Go source and Dockerfile**. Your optimized application image—34.9 MB in our verification—is the natural submission artifact. MongoDB is an external dependency using a prebuilt image.

**Using Compose is consistent with the written requirements.** The assignment requires a database of your choice and instructions for compiling and running the project. It neither requires nor prohibits Compose. In your project:

- The **Dockerfile** builds the Go application image.
- **Compose** starts MongoDB and the application, configures their connection, and manages database storage.
- Compose does **not** combine MongoDB and Go into one image.

I would keep the current architecture and include `docker-compose.yml` with the submission and run instructions. Explain that the app image is 34.9 MB and MongoDB is a separate runtime dependency; don’t present 34.9 MB as the entire system’s footprint. Providing equivalent `docker run` instructions would also make the submission accessible using the commands taught in class.

Because image size explicitly affects grading, the one point worth confirming with the instructor is: **“Does the size comparison cover the submitted application image, or all images required to run the application, including the database?”** Until clarified, I would not spend effort modifying the MongoDB image.