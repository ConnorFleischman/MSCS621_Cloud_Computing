# October 3, 2026 measurements

The report's measured results come from `benchmark_results.csv`, `operations.csv`,
`method.json`, and `runtime.json`. The final request benchmark ran after the build,
test suite, and race detector had finished. It used a local 50 ms mock API, five
articles per request, and the public SQLite-backed fetch path. No live API or real
API key was used. `operations.csv` contains 1,600 measured operations; warm-ups
are excluded. Operation indexes are completion order within each wave.

`runtime.json` retains image identifiers, export sizes, environment information,
five measured Docker cache-hit invocations, five packed-process timing/RSS logs,
and test counts. The release image is `project1-metrics:latest`. The development
image supplies Go, GCC, and `/usr/bin/time`; those tools are absent from the release
image. The generated database fixture is ignored by Git and can be regenerated.

Run these PowerShell commands from `Project1` to reproduce the procedure. Use a
new output directory for each benchmark run because the harness refuses to
overwrite an existing database fixture. Run the steps sequentially.

```powershell
$projectDir = (Get-Location).Path
$metricsDir = "metrics/metrics-rerun"
docker build -f dockerfile --target build -t project1-metrics-build .
docker build -f dockerfile -t project1-metrics:latest .

docker run --rm -v "${projectDir}:/src" -w /src -e DATABASE_PATH= project1-metrics-build sh -c 'go test -json ./... -count=1 > metrics/test_results.jsonl'
docker run --rm -v "${projectDir}:/src" -w /src -e DATABASE_PATH= project1-metrics-build sh -c 'go test -race -json ./... -count=1 > metrics/race_results.jsonl'

docker run --rm -v "${projectDir}:/src" -w /src -e DATABASE_PATH= project1-metrics-build sh metrics/run_metrics.sh $metricsDir
python metrics/collect_runtime_metrics.py --output $metricsDir
```

The request harness builds with the release C flags and static-link/build tags,
but does not pack the harness executable; startup is excluded from request timing.
An initialized temporary SQLite database is reused across scenarios. Each cold
wave uses new topics, so coverage is empty for those queries. Each row has one
warm-up repetition and five measured repetitions, each containing 20 requests in
successive waves at the selected concurrency. Identical cold waves use a single
topic; unique waves use one topic per request. Cache-hit waves reuse one warmed
topic. Percentiles use nearest rank over the 100 samples per row. Throughput uses
the sum of fan-out wave durations, excluding compilation, warm-ups, and CSV I/O.

Run the Python collector promptly after the harness: cache freshness lasts 15
minutes. It uses six separate network-disabled release containers against the
same bind-mounted fixture, excluding the first invocation from timing. It then
uses `/usr/bin/time -v` on the packed executable inside the development image,
again excluding one warm-up. These process measurements include startup and
output; Docker timings additionally include container creation/removal. The
collector requires Python 3 and Docker access, with no Python packages to install.

Test/race JSON logs are stored one directory above these artifacts. The collector
records their counts in `runtime.json`. Models and the metrics command have no
tests; this differs from a skipped named test.

The image collector briefly exports an archive, measures its size, gzip size,
and layer blob sizes, then removes that temporary archive. It reports Docker's
inspect `Size` separately because local-store accounting differs from export
size on this Docker installation. Sizes exclude builder caches and SQLite data.
These are short local tests, not sustained capacity or live NewsAPI benchmarks.
