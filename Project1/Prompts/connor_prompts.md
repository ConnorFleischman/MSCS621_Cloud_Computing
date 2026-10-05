# Connor's Prompts

1. (GPT-6 Luna) Build the newsAPI CLI with flags for topic, day range (including today), and article count, so the tool can fetch and print a usable result set.
   1.1. What’s the cleanest CLI structure for parsing flags and handling invalid inputs?
   1.2. How should the app format the API output so it’s readable and still testable?
   1.3. What edge cases matter here: empty results, invalid dates, or bad topic input?

2. (GPT-6 Luna) Move the API code out of /tests and into /Project1, clean out the old structure, and keep the existing tests working without breaking compatibility.
   2.1. Why did the old imports break when the code moved, and how do we fix that cleanly?
   2.2. What’s the best way to preserve the existing test contract while restructuring the package layout?
   2.3. Should the old test files stay in place, or should we refactor them to match the new architecture?

3. (GPT-6 Luna) Expand the concurrency layer and add tests for multi-user API requests and cache behavior under load.
   3.1. How does concurrent access affect shared cache state, and what kinds of race conditions should we worry about?
   3.2. What’s the best way to simulate multi-user traffic in tests without making the suite flaky?
   3.3. Should we test both success cases and failure cases under concurrency, or only the happy path?

4. (GPT-6 Luna) Prevent duplicate concurrent API requests by reusing cached results, and add regression tests to confirm the fix holds.
   4.1. How do we tell the difference between a normal cache hit and a duplicate in-flight request?
   4.2. What happens when two goroutines hit the same query at the same time, and how do we prevent double fetches?
   4.3. Can we build a regression test that proves duplicate requests collapse into a single upstream call?

5. (GPT-6 Luna) Add timeout handling for both the NewsAPI client and MongoDB interactions, and make sure it integrates cleanly with the existing codebase.
   5.1. What’s the right timeout strategy for external API calls versus local DB operations?
   5.2. How do we surface timeout errors without making the CLI feel broken or inconsistent?
   5.3. Is there a cleaner pattern for supporting retries, deadlines, and graceful cancellation across the stack?
   5.4. What’s the simplest way to keep the codebase readable while adding concurrency and caching?
   5.5. How do we validate that the cache fix actually reduces redundant external calls instead of just masking the issue?
   5.6. Is the timeout behavior best handled in the client layer, the service layer, or both?
   5.7. What metrics or logging would help confirm the project is behaving correctly under load?
