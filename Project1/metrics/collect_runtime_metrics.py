"""Collect image metadata, release-container cache-hit latency, and packed-process RSS.

Run from Project1 after metrics/harness seeds the runtime fixture. No live API is used.
"""
import argparse
import gzip
import json
import os
from pathlib import Path
import platform
import re
import subprocess
import tarfile
import time


def command(args):
    result = subprocess.run(args, capture_output=True, text=True, check=True)
    return result.stdout, result.stderr


parser = argparse.ArgumentParser()
parser.add_argument("--output", default="metrics/metrics-2026-10-03")
args = parser.parse_args()
out = Path(args.output).resolve()
image = "project1-metrics:latest"
builder = "project1-metrics-build:latest"
inspect = json.loads(command(["docker", "image", "inspect", image])[0])[0]
version = json.loads(command(["docker", "version", "--format", "{{json .}}"])[0])
engine = json.loads(command(["docker", "info", "--format", "{{json .}}"])[0])
versions = command(["docker", "run", "--rm", builder, "sh", "-c",
                    "go version; gcc --version | head -1; upx --version | head -1; wc -c /app/app"])[0]
result = {
    "image": image, "image_id": inspect["Id"], "docker_inspect_size_bytes": inspect["Size"],
    "image_descriptor": inspect.get("Descriptor"), "repo_digests": inspect["RepoDigests"],
    "platform": inspect["Os"] + "/" + inspect["Architecture"],
    "docker_version": version, "docker_vm_cpus": engine["NCPU"],
    "docker_vm_memory_bytes": engine["MemTotal"], "host_os": platform.platform(),
    "host_processor": os.environ.get("PROCESSOR_IDENTIFIER", platform.processor()),
    "builder_versions_and_executable_bytes": versions,
    "source_commit": command(["git", "rev-parse", "HEAD"])[0].strip(),
    "source_note": "working tree includes the metrics harness and report changes",
}
archive = out / "release-image.tar"
command(["docker", "image", "save", "-o", str(archive), image])
result["docker_save_archive_bytes"] = archive.stat().st_size
result["gzip_save_archive_bytes"] = len(gzip.compress(archive.read_bytes(), mtime=0))
with tarfile.open(archive) as saved:
    manifest = json.load(saved.extractfile("manifest.json"))
    result["saved_layer_blob_bytes"] = sum(saved.getmember(layer).size for layer in manifest[0]["Layers"])
result["image_size_scope"] = "Docker inspect Size is the local store accounting on this engine; save/gzip sizes describe exports, not unpacked executable size. Build caches and database data are excluded."
archive.unlink()
mount = f"{out}:/metrics"
common = ["docker", "run", "--rm", "--network", "none", "-v", mount,
          "-e", "DATABASE_PATH=/metrics/runtime-fixture.db"]
query = ["-topic", "runtime-smoke", "-days", "1", "-articles", "5"]
elapsed = []
for repetition in range(6):
    started = time.perf_counter()
    stdout, stderr = command(common + [image] + query)
    seconds = time.perf_counter() - started
    if "Results: cache" not in stdout or "Fetched 5 of 5" not in stdout:
        raise RuntimeError("release-image smoke test did not return five cached articles")
    if repetition:
        elapsed.append(seconds * 1000)
    (out / "release_cache_hit.txt").write_text(stdout, encoding="utf-8")
result["release_container_cache_hit_ms"] = elapsed
result["release_smoke"] = "six separate network-disabled containers reused SQLite; five measured after one warmup"
result["container_timing_scope"] = "host wall clock including Docker client, container creation/removal, packed executable startup, SQLite lookup, and printed output"
process = []
for repetition in range(6):
    stdout, stderr = command(common + [builder, "/usr/bin/time", "-v", "/app/app"] + query)
    if "Results: cache" not in stdout:
        raise RuntimeError("packed-process measurement did not hit cache")
    if repetition:
        process.append(stderr)
result["packed_process_time_verbose"] = process
result["peak_rss_kib"] = [int(re.search(r"Maximum resident set size \(kbytes\):\s*(\d+)", log)[1]) for log in process]
result["process_timing_scope"] = "packed release executable inside development container; excludes Docker startup; /usr/bin/time values are coarse"
for name in ["test_results", "race_results"]:
    events = [json.loads(line) for line in (out.parent / (name + ".jsonl")).read_text().splitlines()]
    counts = {}
    for action in ["pass", "fail", "skip"]:
        counts[action] = sum(e.get("Action") == action and "Test" in e for e in events)
    counts["packages"] = [{k: e[k] for k in ["Package", "Action", "Elapsed"] if k in e}
                           for e in events if "Test" not in e and e.get("Action") in ["pass", "fail", "skip"]]
    result[name] = counts
(out / "runtime.json").write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")
print(json.dumps({k: result[k] for k in ["docker_inspect_size_bytes", "docker_save_archive_bytes", "gzip_save_archive_bytes", "saved_layer_blob_bytes", "image_id", "image_descriptor",
      "docker_vm_cpus", "docker_vm_memory_bytes", "builder_versions_and_executable_bytes",
      "release_container_cache_hit_ms", "peak_rss_kib", "test_results", "race_results"]}, indent=2))
