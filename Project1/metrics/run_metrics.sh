#!/bin/sh
# Run inside project1-metrics-build with Project1 mounted at /src.
set -eu
export CGO_ENABLED=1
export CGO_CFLAGS='-Oz -ffunction-sections -fdata-sections'
go build -trimpath -tags 'netgo osusergo sqlite_omit_load_extension' \
  -ldflags='-s -w -linkmode external -extldflags "-static -Wl,--gc-sections"' \
  -o /tmp/search-metrics ./metrics/harness
/tmp/search-metrics -output "${1:-metrics/metrics-2026-10-03}"
