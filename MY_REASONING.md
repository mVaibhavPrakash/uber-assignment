# Implementation reasoning and AI use

I built the project as an explainable decision-support prototype: compare historical completed trips under different rainfall conditions, combine that evidence with a forecast, and identify a zone where driver availability merits review. I kept the distinction between projected trip activity and actual driver shortage explicit throughout the design.

## AI use disclosure

I led the implementation, supplied the project code, requirements and design direction, and decided which suggestions to retain, simplify or reject. ChatGPT supported design discussions and challenged assumptions. OpenAI Codex assisted with code revisions, refactoring, SQL, error handling, tests, Docker troubleshooting and documentation. AI was not the sole author of the project.

Accepted refinements included separating storage from scoring, expressing the historical comparison in SQL, distinguishing no intervention from insufficient evidence and unavailability, and checking forecast version and expiry on each cached read. I reviewed generated changes against the running application and revised outputs that added unnecessary complexity or overstated what the data could support.

Important corrections included the initial all-zone eligibility rule, the rejection of valid light rain, excessive test scenarios, and scheduler mutexes that protected no actual concurrent access. Earlier documentation also retained outdated Chicago and synthetic-data assumptions; those were corrected to reflect the submitted NYC dataset.

## Earlier discussions and decisions

The implementation developed in stages. The experiments and earlier proposals below explain how the final design emerged. They are development history: the initial Chicago scope, synthetic fixtures, preparation tooling and larger test suite should not be read as the current application configuration.

### Initial Chicago approach and retrieval problems

The initial approach used Chicago TNP and 77 community areas, with one weather coordinate per area. I first considered a published pickup centroid instead of averaging coordinates, because an average coordinate might not represent a meaningful pickup location. These were early Chicago choices; the final application uses 20 NYC zones with manually supplied coordinates.

Before building around Chicago retrieval, I tested whether the required history could be obtained within practical request limits. A small successful response did not establish that a full historical aggregation would work, so I also tested counts, grouped queries and larger exports.

| Experiment | Recorded result | Effect on the decision |
|---|---|---|
| Chicago date-filtered retrieval of five rows | Succeeded in 1.294196 seconds. | Confirmed basic access, but not bulk feasibility. |
| Chicago single-day count and seven-day aggregation | Each timed out at 60 seconds without a response. | Raised concerns about relying on server-side historical aggregation. |
| Chicago five-column CSV request | Returned 23,259 parsed rows in an incomplete 60-second response; recorded 735,914 transferred bytes and 2.299 MiB locally. | Demonstrated partial retrieval, not a complete usable export. |
| Chicago two-column CSV request for 10,000 rows | HTTP 200 in 13.675634 seconds, with 14,405 transferred bytes; actual row count was not verified. | Was insufficient evidence that the complete history could be collected reliably. |
| Attempted Chicago bulk export endpoint | HTTP 410 in 1.562915 seconds. | That endpoint was unavailable; other export methods were not ruled out. |
| NYC January 2022 Yellow Taxi Parquet | HTTP 200; 38,139,949 bytes in 3.466139 seconds. | Established a concrete downloadable alternative. |
| NYC January 2022 HVFHV Parquet | HTTP 200; 374,619,188 bytes in 31.026937 seconds. | Supported choosing downloadable HVFHV history for preparation. |

These were individual development experiments, not controlled source-performance benchmarks. I considered reducing the historical period, but the daily Chicago query failures made that an uncertain solution. I switched to NYC while preserving the hourly zone/weather comparison architecture. HVFHV was selected for marketplace relevance; Yellow Taxi was smaller but less directly aligned with that focus. I rejected adding DuckDB and kept preparation in Go.

### NYC preparation and operational scope

The first NYC work focused on projected Parquet reads, monthly checkpoints, zone lookup validation and alignment between pickup timestamps and historical weather. On November 25, preparation notes recorded `parquet-go` v0.25.1, zone/year weather checkpoints and quota reservations. These belong to the separate offline preparation work, not the runtime recommendation service.

I rejected Python preparation and automated polygon or coordinate discovery. The intermediate approach used editable JSON coordinates with `0,0` placeholders until representative locations were supplied manually. An automatically selected top-volume scope was also considered. When the manually edited zone list differed from that approach, I explicitly chose the edited 20-zone list. The final seed preserves that scope rather than replacing it with a calculated top 20.

Early validation used fictional inputs: six pickup rows produced three accepted records, while missing timestamps, out-of-month pickups and unknown zones were rejected. Other checks covered integer zone types, local timestamp conversion, DST exclusions, checkpoint reuse and preserving manual metadata edits. A 192-observation synthetic seed across two noncontiguous zone IDs supported initial SQL and startup checks. Those fixtures tested implementation behavior; they were not public-data accuracy or performance results.

At that stage, the full historical run and runtime features were still unfinished. The final submitted seed later replaced those development fixtures with 697,440 matched observations for 2022-2025. The earlier statement that full public data was not yet prepared described that intermediate stage, not the submitted dataset.

I also separated preparation from reviewer setup. Downloading, aggregation, checkpoint and quota tooling moved outside the assignment runtime. Prepared JSON, mandatory-field cleanup performed offline, and transactional COPY became the setup boundary. This removed preparation-only dependencies and repeated cleanup work from startup.

### Choosing a historical comparison

I challenged both exact calendar dates and all-year weekday/hour averages. Exact dates offer few comparable observations across years, while all-year groups mix seasons. Monthly groups, rolling calendar windows and adaptive fallback were explored as alternatives.

AI proposed rolling seasonal windows with bounded fallback. I questioned whether widening the window to gain only a few rainy observations justified the extra rules, whether those observations remained comparable, and how the fallback would be explained. I initially chose fixed meteorological seasons, the same day of the week and local hour, and one reusable startup aggregation. That approach treated the seven days separately using ISO `day_of_week` numbering.

Weights for rainy versus dry hours and temporal decay were also discussed. I deferred them and retained signed additional trips, so a decrease in one hour offsets an increase in another. The horizon remains configurable in whole hours. More trip records within an hour do not create more independent rainy-hour observations.

The SQL was subsequently extended with absolute and percentage uplift and rainy-hour sample standard deviation. Its CTE reuses historical averages, its snow filter is shared, and unknown statistics remain null. Earlier PostgreSQL fixtures checked negative uplift, a single rainy observation, zero-baseline percentage handling and the then-current exclusion of trace rain. The later light-rain decision described below deliberately changed that grouping.

### Forecast reliability and interval alignment

A full-period Open-Meteo archive experiment returned about 1.69 MiB for one area, but that alone did not establish quota usage or total preparation runtime. Early designs explored batching, workers and persistent quota tracking. I later simplified runtime fetching to one seeded-area batch with bounded retries; offline preparation retained its own quota tooling.

A live Open-Meteo check validated 600 forecast rows across the selected 20 zones and exposed a request-contract issue: the provider rejected `forecast_hours` together with explicit hour bounds. The request was corrected. That validated response handling, not recommendation accuracy.

The weather fields also needed interval alignment. Provider rain, snow and probability values label the preceding hour, so the parser assigns them to the interval starting one hour earlier. Forecast persistence was kept transactional, with publication only after a complete save. Uncertain commit outcomes trigger a reload rather than repeating the write blindly.

Earlier verification covered rollback, retained forecasts on failures, cancellation, Retry-After, invalid responses and cache ownership. Some of those checks were later removed from the retained suite, and preparation-only worker/quota machinery was removed from the runtime. The current defaults and test scope are described below.

### AI suggestions and my decisions

| Topic or suggestion | My challenge and resulting change |
|---|---|
| Automatically optimize the historical period | Kept the period configurable and focused on a functional comparison rather than treating period selection as a validated optimization. |
| Server-side Chicago aggregation instead of raw downloads | Tested the retrieval path; timeouts and an unavailable export endpoint contributed to the NYC Parquet decision. |
| Combine preparation artifacts | Discussed separate area, trip and weather artifacts, initially used one nested JSON seed, then replaced it with two CSV files to simplify PostgreSQL setup. |
| Fetch hourly because forecasts are hourly | Distinguished data resolution from fetch cadence; chose three-hour weather refresh and hourly recommendation updates. |
| Generic 15-minute or arbitrary-duration buckets | Questioned the benefit when both historical preparation and provider data are hourly; retained configurable whole-hour windows. |
| Retain raw timestamps and add 15-minute outputs | Deferred them because finer mobility granularity would not establish finer weather information. |
| Direct database access throughout business code | Requested repository contracts and dependency injection. Another database still needs its own queries, storage adapter and validation. |
| Add historical indexes before measuring | Revisited the query pattern: startup aggregates the full history, so extra indexes require a demonstrated query-plan benefit. |
| Rigid area-ID assumptions and repeated seed validation | Kept database keys, foreign keys and required columns; moved mandatory-field cleanup to preparation instead of imposing fixed zone-ID bounds. |
| Schema creation without an explicit migration deliverable | Requested minimal up/down migrations and an explicit setup/reset workflow. |
| Broad reliability machinery and test expansion | Retained bounded requests, underlying error information and graceful shutdown, but later reduced runtime complexity and tests to the actual assignment scope. |

Subsequent review of the running application led to the partial-coverage, light-rain, cache, scheduler-lock and test-scope changes below. Those changes were responses to concrete behavior and code ownership, rather than simply accepting the first AI-generated design.

## Current data selection and preparation

The submitted seed contains 20 manually selected NYC zones and 697,440 matched hourly HVFHV trip/weather observations for 2022-2025. The selection is an explicit operational scope, not an automatically chosen top-20 list. Each zone has a manually supplied representative coordinate.

Chicago TNP was explored first, but bounded retrieval experiments encountered timeouts for daily counts and weekly aggregation. NYC downloadable Parquet data provided a practical preparation path. Those experiments informed source selection; they do not establish that every Chicago retrieval method would fail or that NYC is universally faster.

I chose Go for offline preparation and editable zone metadata rather than Python preparation or automated coordinate discovery. Downloading, aggregation and mandatory-field cleanup remain outside the submitted runtime. Setup now imports two prepared CSV files transactionally through `psql`, replacing the initial JSON loader. This avoids repeating a large preparation pipeline during reviewer startup while preserving valid zero values and nullable optional measurements.

## Historical comparison and scoring

I initially selected fixed meteorological seasons with the same zone, exact day of week and New York local hour. A later rainy window had usable forecasts for all 20 zones, but none had enough historical rainy samples for both hours. A read-only PostgreSQL comparison found that 28.42% of exact-day groups met the minimum, compared with 93.07% for weekday/weekend groups and 100% for groups that pool all days within a season and hour.

I chose to pool all seven days within each zone, season and local hour. Both broader groupings covered 20 of 20 zones in the rainy example, so one group per season and hour was simpler to explain and gave more samples. This can hide commute and weekend patterns. The comparison measured evidence coverage, while predictive accuracy remains untested.

I later checked whether the history actually showed differences by day before switching to exact ISO day and local hour across all seasons. At 8 AM, dry-weather trips per zone averaged 244–256 from Monday through Friday, 158 on Saturday and 136 on Sunday. At 6 PM, the average rose from 266 on Monday to 326 on Friday. All 20 zones contributed to each comparison. The full set of averages and coverage figures is in [the implementation plan](plan.md).

I kept `day_of_week` in the stored observations, the history cache key and the hourly API evidence. The forecast timestamp is converted to New York time before lookup, so a Thursday at 1 PM matches past Thursdays at 1 PM in the same zone. The hourly panel displays the time and explains that comparison in its caption. I removed `season` from the lookup and API evidence. Once the lookup and evidence used the same weekly key, I removed the derived season field from the CSV and database. Keeping the original timestamp and local date preserves each observation for future comparisons.
I replayed the same saved 1–3 PM New York forecast with both groupings. Both assessed all 20 zones. With the previous season-and-hour grouping, Williamsburg North Side led with 215.94 modeled additional trips. Under the final exact-day grouping, Williamsburg dropped to 75.22, and Upper East Side North led with 97.93. The rainy-minus-dry averages were smaller under the final grouping for those hours. Rainfall and precipitation probabilities stayed the same, and the highest modeled increase was below the same 100-trip threshold. The wording now makes clear that significant rain is forecast and the estimate is below the threshold. The replay explains the decision change, while predictive accuracy remains untested.

PostgreSQL calculates historical means, counts, rainy-hour dispersion and signed differences. Go checks eligibility, applies forecast probability, aggregates the planning window and ranks zones. Keeping these responsibilities separate makes the calculation traceable and keeps the scoring engine independent of SQL and HTTP clients.

For a qualifying rainy hour, the model uses `(rainy mean - below-threshold mean) * precipitation probability / 100`. Negative effects remain negative when summed. Five historical observations per relevant class and 100 additional trips over the default two-hour window are configurable heuristics, not calibrated confidence or operational thresholds.

### Partial coverage

During my review of the running dashboard, I observed only 13 of 20 zones assessable and challenged the AI-assisted all-zone eligibility rule. A zone with complete evidence could exceed the threshold but still receive no recommendation because another zone lacked history.

I chose to rank complete zones and qualify the result with assessed/total coverage. Excluded zones can have higher impact, so the UI does not describe the winner as highest across the entire monitored scope. With partial coverage and no qualifying action, it avoids declaring normal operations. Each recommended zone still needs evidence for every planning-window hour.

### Light rain

I rejected describing valid 0.4 mm rainfall as unsupported weather. Historical observations and forecasts now share a numeric boundary: nonnegative rain below the configured threshold belongs to the comparison group; rain at or above the threshold belongs to the rainy group. Below-threshold hours require baseline history and contribute zero modeled additional trips.

This is a modeling assumption about the incremental effect below the threshold, not a claim that light rain has no real effect. I retained "Typical dry-weather trips" for readability and added a tooltip explaining light-rain inclusion. Snow remains excluded. The SQL change requires rebuilding and restarting the API to refresh its statistics, not reseeding existing observations.

## Scheduler and concurrency decisions

I separated forecast resolution, provider refresh cadence and recommendation cadence. Hourly observations do not require hourly provider requests. The current defaults fetch 30 forecast hours per zone every three hours, compute recommendations hourly or when their inputs change, and check for work every 15 minutes or at the next hour boundary.

The next-hour wake-up follows recommendation validity. At 10:53, the result covers 11:00-13:00 and normally expires at 11:00. Waiting 15 minutes would postpone replacement until 11:08, so the scheduler wakes at 11:00 instead. Jobs remain sequential; a provider delay can still delay computation.

I reviewed the callers before removing the scheduler's tick mutex and state RWMutex. The server starts one `Run()` goroutine, which calls each tick synchronously and waits for its jobs to finish. Only that loop reads and updates job state; HTTP handlers do not access it. Neither lock was protecting concurrent access in the implemented execution path.

I removed both mutex fields and their lock/unlock calls, deleted the unused `State()` getter and scheduler-busy error, and made `Tick()` a private `tick()` helper. The helper reads `s.state` directly. This reduces code and makes state ownership explicit instead of retaining locks for hypothetical callers. `Run()` must still be called only once per scheduler instance; concurrent manual triggers or external state readers would require revisiting synchronization.

Shared forecast and recommendation caches retain their concurrency protections because HTTP reads do overlap background updates. The weather service also retains its separate refresh guard. After the scheduler changes, Go tests, vet and build passed; the SQL integration test requires its database environment variable. The smaller test suite does not establish race coverage, so the reason for removing these locks is the verified caller and ownership structure.

Provider failures receive up to three attempts by default, with exponential jitter and respect for Retry-After. A failed refresh retains usable previous weather. Stale warnings, forecast coverage, version matching and expiry prevent that fallback from becoming an indefinitely valid recommendation. The scheduler's next weather attempt is still governed by the refresh interval; its regular checks do not create continuous provider retries.

## Cache ownership and request consistency

I replaced deep copies of every recommendation read with atomic publication of a completed result. API summaries copy area headers before omitting hourly evidence, and stale responses allocate their own warning list. Readers must not modify published nested data; Go does not enforce that contract.

Every request still validates weather usability, forecast version, planning window and expiry. A cached pointer can remain present after the decision expires, so presence alone is insufficient. Insight requests can bind to the displayed version and window to avoid mixing evidence from different decisions.

## Validation and retained tests

I subsequently requested explicit configuration getters and setters. `Config` fields are now private, callers read through typed `Get...()` methods, and overrides use `Set...()` methods. This adds accessor boilerplate intentionally rather than reducing line count. Environment parsing, defaults and validation retain their existing behavior; setters preserve ordinary assignment semantics and affect only the config value being changed. Services still receive copies, so these setters are not a live-reconfiguration mechanism. Local checks verified all accessor pairs and compared parsed values and errors with the preceding implementation.

I then narrowed configuration validation to presence and type conversion. Explicitly empty or whitespace-only values are rejected, missing optional settings use their defaults, and integers, numbers, durations and time zones must parse into their configured types. I removed config-level positivity rules, provider range limits, address/URL format checks and cross-field comparisons. This intentionally accepts a wider set of typed settings; operational checks in the recommendation engine, database and weather components remain separate and unchanged. Getters and setters keep their existing behavior.

### Simplifying supporting code without changing the workflow

After making the recommendation manager's steps explicit, I reviewed the supporting packages for reductions that would not obscure that flow. PostgreSQL area and historical queries now share one row collector for iteration, cleanup and scan/read errors. The single-caller transaction-start wrapper was inlined into forecast replacement, preserving its error context and rollback handling. Unused historical query-plan and cache-length methods were removed after checking their callers. Configuration connection checks and weather fetching no longer pass through forwarding-only helpers, and the numeric environment parser was renamed to describe its actual return value.

The cleanup also simplified zone membership checks, historical cache misses and forecast badge formatting. Recommendation calculation, scheduler timing, retry policy, forecast validation, transaction boundaries and cache ownership were retained. Temporary before/after checks matched 2,000 configuration cases, 48 API cases, weather request/response handling, historical cache ownership, PostgreSQL reads/writes and rendered UI markup. All four tests at that stage, including the PostgreSQL integration test, plus Go vet/build and the frontend build passed. These comparisons remain local refactor checks rather than additions to the retained suite.

I then made the call order explicit because fewer lines alone did not make the recommendation path easy to follow. The coordinator is named `Manager`, and `ComputeAndPublish()` reads as four numbered steps: cached weather, cached historical evidence, scoring, and publication. The existing publication policy moved into `publishRecommendation()`. This is a Go struct with methods rather than an additional framework or generic pipeline. The scheduler still performs live weather fetching separately; API requests still only read the saved result. The scoring engine also labels its three stages: validate, estimate zones, and select a decision.

I revisited the recommendation package to reduce helper calls and repeated selection passes without changing decisions. Previously, a missing-weather flag passed through hourly aggregation into the engine, while separate helpers scanned the areas for a winner and a no-action explanation. Selection now derives those facts from completed area evidence in one pass. Sorting by zone ID before selection preserves tie-breaking without a second comparison rule. Sample checks are written directly in the hourly eligibility switch instead of a trivial forwarding helper. Validation, evidence copies, cache version/window/expiry checks and JSON formatting remain because they protect distinct behavior.

For this refactor, a temporary comparison against the original package matched 2,000 generated scoring cases at full precision and in serialized JSON, including messages and errors. Additional comparisons checked empty caches, version changes, freshness, expiry, weather failures and cancellation. These were local refactor checks, not additional retained tests or forecast-accuracy validation.

The submitted suite keeps four focused tests on the decision, its SQL inputs, forecast precision after restore and a provider outage. The decision test uses one rainy hour and one dry hour with no baseline.

| Test | Decision it protects |
|---|---|
| Assessed-zone ranking | A zone with insufficient rainy samples neither blocks nor outranks an eligible zone; the result reports partial coverage and the expected trip calculation. |
| Historical SQL | PostgreSQL groups below-threshold and qualifying rain correctly, excludes snow and returns the expected historical comparison. |
| Forecast precision after restore | The adjacent values below and above 0.5 mm and the threshold itself survive PostgreSQL storage and yield the same recommendation as the live forecast. |
| Provider failure | Bounded retries preserve a usable prior forecast, while expired weather is rejected. |

The PostgreSQL tests use isolated schemas and require `TEST_DATABASE_URL`. Checks for cache concurrency, serialization, configuration and daylight-saving transitions ran during development. The submitted suite focuses its repeatable coverage on the weather-to-decision path.

Validation performed during development included Go tests, vet/build, frontend builds, PostgreSQL checks, live API requests, UI rendering checks, and Docker setup checks. Earlier broader checks are development history rather than retained automated coverage. Fictional rain scenarios are explicitly labeled and use the real scoring function; they do not demonstrate live prediction accuracy.

## Reviewer setup and local debugging

Reviewer setup also changed in response to practical failures. The original JSON seed exceeded GitHub's regular per-file limit, so it initially moved to Git LFS and setup pulled LFS data before building. The later CSV conversion described below removed that requirement for the current dataset. Database credentials were removed from Go and Compose defaults; local-only examples remain in `.env.example`.

The first rain demo reused the normal API/UI ports. When it collided with an existing backend, I separated the sample onto 8081/3001 so it could run alongside the normal application on 8080/3000. An interrupted `npm ci` also exposed a Windows file lock: a running Vite preview held `esbuild.exe`, leaving dependencies partly removed. Stopping that project process and reinstalling repaired it; the instructions now distinguish initial installation from ordinary startup.

For terminal debugging, I retained PostgreSQL in Docker and exposed it only on localhost, avoiding the need to discover credentials for a separately installed PostgreSQL instance. Windows blocked the first chosen port, 5433, so the mapping changed to the verified available port 15432. Existing database data was preserved, and the historical SQL integration test passed through the host connection. The Go backend can then run in a terminal or debugger using the same assignment database.

## Replacing the JSON loader with CSV imports

I questioned the amount of Go seeding code needed for an already-prepared dataset. The JSON loader still decoded nested records, maintained seed-specific types, built batches and converted rows before using PostgreSQL COPY. Preparing files in the format PostgreSQL could consume directly removed that intermediate application layer.

The JSON was converted once to `areas.csv` and `historical_observations.csv`. All 697,440 observation records passed a CSV round-trip hash comparison of every imported field. The prepared data shrank from about 230 MB of JSON to 46 MB of CSV and now fits in regular Git. The original JSON remains in an ignored local archive; it is not included alongside the new seed.

I replaced the Go seed command and package with `database/seed.psql`, run by the PostgreSQL client image. The first version checked the dedicated database/schema, serialized setup with a transaction advisory lock, refused existing data by default, applied the migration when needed and imported both files in one transaction. CSV headers must match the expected columns, null measurements use an explicit marker, and malformed imports fail rather than silently skip rows. The API image now contains only the application, not seed files or a seed executable.

Reset remains explicit but now recreates the three assignment tables inside the import transaction instead of dropping the database. It refuses other connected clients. This allows failed reset/import attempts to roll back the prior tables and data together. It also makes setup intentionally PostgreSQL-specific. Recommendation scoring, historical grouping and forecast behavior are unchanged.

### Earlier repeat setup experiment

After the CSV conversion, rerunning the documented setup command failed because the database already contained data. That protected the existing dataset, but made a normal reviewer action look like a broken installation. I changed setup to load empty tables, report a successful skip when the expected zones and history are already present, and reject partial or different data. Reset still requires an explicit choice.

I chose to compare the stored data with temporary copies of the bundled CSVs, including every field, null and duplicate count. Checking row counts alone could accept different observations, while a new seed marker would not identify databases initialized by the earlier script. This also lets those existing installations use the same command without a reset. Weather forecasts are excluded from the seed comparison because the running application updates them; a repeat setup preserves them.

The extra cost is loading and comparing the CSVs on repeat setup, rather than immediately returning from a marker check. Verification permits API reads but pauses writes to the assignment tables. Ordinary application startup is unchanged, and recommendation, historical grouping and scheduling logic are untouched.

I verified first-time setup and repeat runs with all 20 zones and 697,440 observations in a disposable PostgreSQL container. A repeat through Docker Compose preserved the existing seed row versions and a saved forecast. Changing an observation without changing row counts still failed verification. Smaller local fixtures checked missing observations, partial schemas, null/zero values, explicit reset and rollback after a failed import. These checks did not add tests to the retained recommendation suite or modify the running assignment database.

### Reviewing the architecture for the interview

I asked for another pass focused on how I would follow and explain the code in an interview. The scheduler previously stored last success and error fields even though scheduling only used the last attempt times. It now keeps those two timestamps and logs failures where each job runs. The next UTC hour is calculated in one place and used by scheduling, forecast coverage and recommendation planning.

I kept the scoring path explicit. Zone comparison gathers the facts, and decision selection uses them to explain the outcome. For weather failures, the forecast client owns provider retry rules, the PostgreSQL adapter handles transaction errors, and the weather service handles reloading a forecast when the commit result is uncertain. The dashboard shares the forecast and window comparison between fetching and rendering zone details.

The terminal debug instructions also exposed a configuration conflict. PowerShell's `DATABASE_URL` pointed at the published Windows port, and Compose used it to replace the container URL. Both containers now take their URL from `.env`, which uses the database service hostname. I verified repeat setup with the Windows URL still set in the shell.

Local comparisons matched 2,000 scoring cases at full precision and in JSON. I also checked scheduler timing around the hour boundary, forecast recovery after an uncertain commit, and the rendered decision cards. These checks supplemented the tests retained at that stage.

## Technical challenges and remaining limits

Final validation reproduced a precision error in a disposable Docker stack. A forecast of `0.49999999` mm was below the 0.5 mm threshold in Go, but `REAL` stored it as `0.5`. Restarting with the provider offline changed the decision reason for the same forecast version. Rain and snowfall now use `DOUBLE PRECISION` in both forecast and historical storage. The new regression test checks the two adjacent values around 0.5 and 0.5 itself. The Docker restart check confirmed the same recommendation before and after restore, apart from the computation timestamp.

The final seed script replaces the earlier full-data comparison with direct CSV imports and a populated-table check. A repeat setup leaves existing data and forecasts alone. An obsolete experimental schema is replaced through explicit reset after the application is stopped. The Docker checks also confirmed that a failed CSV import during reset restores the original tables, data and forecast.

The runtime rejects a timezone other than New York because the bundled history is keyed to that timezone. Forecasts and planning intervals stay in UTC across daylight-saving transitions. The frontend ranks zones by exact estimates and rounds for display using the existing API fields. It binds the selected zone's evidence to the forecast version and planning window.

The most demanding parts were aligning historical and forecast intervals with New York calendar fields, handling sparse rainy evidence without confusing missing values with zero, and keeping persisted forecasts, cached recommendations and UI insights consistent across refreshes and expiry. Separating offline preparation from runtime setup also required keeping the reviewer workflow reproducible without downloading raw history.

The application remains a single-instance prototype. Planning uses the next two complete hours and does not estimate activity remaining in the current hour. It does not include live driver supply, waiting riders, holidays, events or weather effects beyond the implemented rainfall comparison. Historical association, probability weighting and sample counts do not establish causality or calibrated forecast confidence.

Further work would start with held-out evaluation and operational input quality before introducing more complex models. Multiple replicas would also require explicit scheduler coordination and cache consistency. Neither is claimed as implemented.

See [design tradeoffs](TRADEOFFS.md), [scheduler and recommendation architecture](docs/SCHEDULER_AND_RECOMMENDATIONS.md), and [database design and SQL](DESIGN_AND_SCALABILITY.md#database-design-and-important-sql) for the corresponding implementation details.
