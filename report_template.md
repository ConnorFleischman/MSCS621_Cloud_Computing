# Project 1 Report Template

## Title and Submission Information
- Project title:
- Course / term:
- Authors:
- Submission date:
- Repository or source archive:
- Docker image name, tag, and digest:

## Abstract
- State the user problem, implementation, database choice, deployment approach, and main measured results.
- Keep results factual; distinguish measured values from design goals.

## 1. Requirements and Scope
- Summarize the assignment requirements addressed.
- State the supported search inputs, date-window interpretation, and result limit.
- Identify any differences between the assignment wording and the submitted implementation.
- State scope limitations and any instructor-approved deviations.

## 2. Architecture
### 2.1 Components and Request Flow
- Explain the Go CLI, models, NewsAPI client, cache, database, and Docker image.
- Include a compact request-flow diagram or numbered flow.
- Identify the database actually implemented. Do not describe an unimplemented database as part of the runtime.

### 2.2 Architectural Decisions
- Explain why Go, the selected database, the NewsAPI endpoint, and Docker were chosen.
- Describe alternatives considered and the important tradeoffs.
- Explain image build/runtime separation and persistent data placement.

### 2.3 Cache Semantics
- Define cache key dimensions and normalization.
- Explain freshness duration, covered date ranges, requested article limits, and when an API refresh occurs.
- Explain duplicate prevention and behavior when fewer articles than requested are available.
- Describe persistence and restart behavior.

### 2.4 Concurrency and Reliability
- Explain where goroutines are started and how result/error channels are consumed.
- Describe same-query request coalescing and how unrelated requests can overlap.
- Explain database transaction, locking, timeout, and cross-process behavior.
- State tested concurrency limits and known limits; do not imply unlimited capacity.

## 3. Build, Configuration, and Usage
### 3.1 Prerequisites and Configuration
- List Go/compiler or Docker prerequisites.
- Explain API-key configuration without including the key.
- Explain database/cache path and timeout settings.

### 3.2 Compile and Run
- Include reproducible native and/or Docker build commands.
- Include a single-search command and a concurrent/batch invocation.
- Explain flags and expected output location/format.

### 3.3 Test and Input/Output Files
- List the test commands and fixture inputs.
- Identify expected-output files and explain what each verifies.
- Include representative output or link to checked-in expected output.

## 4. Tests and Findings
- Describe unit, integration, fixture, concurrency, race, and container tests actually run.
- State exact command, environment, date/revision, pass/fail count, and notable findings.
- Explain failures or skipped tests; do not report unrun tests as passed.

## 5. Performance Method and Results
### 5.1 Method
- Record host, OS/architecture, Go and Docker versions, image digest, database state, and test configuration.
- Separate in-process request latency from end-to-end CLI/process latency.
- Use a deterministic mock API for repeatable load tests; document mock delay and response size.
- Define warm-up, repetitions, duration, concurrency levels, and percentile calculation.
- Include cache-hit, API-fetch/cache-miss, unique concurrent searches, and identical concurrent searches.

### 5.2 Results
| Scenario | Concurrent searches | Completed | Errors | API calls | Throughput (searches/s) | p50 (ms) | p95 (ms) | p99 (ms) | Max (ms) |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Cache hit | | | | | | | | | |
| API fetch / cache miss | | | | | | | | | |
| Unique cold searches | | | | | | | | | |
| Identical cold searches | | | | | | | | | |

- Attach or link raw CSV results and the command/harness used to produce them.
- Discuss trends, errors, cache effectiveness, bottlenecks, and limitations.
- Keep Docker image-size and startup measurements distinct from application request benchmarks.

## 6. Related Work and Citations
- Use one consistent IEEE or ACM citation style throughout.
- Cite primary documentation for Go, NewsAPI, the selected database, and Docker.
- Cite relevant prior work on concurrency, caching, or latency where useful.
- Explain specifically how this implementation differs from each cited work; avoid claiming novelty for established techniques.

## 7. AI Use and Reproducibility
- State every AI product/provider and exact model/version used, verified from the tool metadata.
- Include the complete text of every prompt used, including follow-up prompts, in an appendix or linked prompt log.
- For each prompt, record tool/model, date, relevant supplied context/files, and resulting code/document areas.
- Describe human review, testing, and modifications after AI output.
- Do not include API keys, credentials, or other secrets in prompts or the report.
- If an exact model/version or original prompt cannot be recovered, disclose that limitation explicitly.

## 8. Limitations and Future Work
- State deployment, database, API, concurrency, and benchmark limitations.
- List only concrete next steps relevant to the assignment.

## References
[1] Author/organization, “Title,” publication/site, date if available. [Online]. Available: URL. [Accessed: date].
[2] Author/organization, “Title,” publication/site, date if available. [Online]. Available: URL. [Accessed: date].

## Appendix A: AI Prompt Log
- Prompt identifier:
- Tool/provider and exact model/version:
- Date:
- Context/files supplied:
- Exact prompt text:
- Output used and human changes:

## Appendix B: Reproduction Checklist
- [ ] Build succeeds from a clean checkout.
- [ ] Runtime API key is provided outside source control and the report.
- [ ] Unit/integration tests pass with recorded commands.
- [ ] Fixture inputs and expected outputs are included.
- [ ] Docker image builds and runs with persistent storage.
- [ ] Performance harness and raw results are included.
- [ ] References and AI prompt disclosure are complete.