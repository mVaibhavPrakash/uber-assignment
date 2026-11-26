# Operations Control Room - implementation plan

This file preserves the original implementation plan and alternatives. The application is now implemented; use [README.md](README.md) and [DESIGN_AND_SCALABILITY.md](DESIGN_AND_SCALABILITY.md) for current setup and architecture. Earlier proposed preparation, quota and UI details below may have been superseded by later user decisions.

## Current recommendation cache design

Compute a complete result and atomically publish it. Published nested data is read-only: `Service.Current` returns a small value containing shared evidence and current freshness, without deep cloning. The API prepares summaries in `internal/api/response.go`, copying only area headers before omitting hourly details and allocating a separate warning list for stale responses. Insights read cached hourly evidence directly. Keep precise scoring, integer JSON output and all forecast version/window/expiry checks. Use one shared validity rule for reads and scheduler decisions. Retain historical and forecast cache ownership protections. Extend the existing recommendation test for cache isolation, repeated requests, stale warnings, concurrent refreshes, version changes and expiry; keep three top-level tests.

## 1. Assignment and product
Source: `Take Home Exercise_ Operations Control Room (Full Stack) (1) 2.pdf` (three pages). Mandatory: Go backend, public mobility dataset and one live API, simple relational database (2–3 tables), schema or migrations, meaningful SQL aggregation/comparison/ranking, minimal frontend, 2–3 focused tests. React/TS recommended. One current actionable recommendation, evidence, explanation and uncertainty; explain no intervention when appropriate. No general dashboard. Reviewer must run in one or two commands. Submit accessible repository, README, dependencies/config/schema, screenshot/demo, architecture and SQL explanation, limitations/trade-offs and AI disclosure. AI disclosure: tools/uses, accepted and rejected or changed suggestions, validation, risks and challenging parts. Follow-up includes demo, architecture/SQL explanation and small change/debugging. Advanced SQL and deployment optional.

Product: NYC weather-aware mobility advisor. Recommend reviewing driver availability in the single area with the highest eligible expected additional completed trips for the next two complete hours. Historical completed trips are a proxy for activity, not unmet demand, driver supply or causal weather effects. Do not recommend a precise number of drivers. Show one action or explicit no intervention, insufficient evidence, or unavailable. Show area, window, expected additional completed trips, baseline, forecast, sample counts, season, freshness and limitations. Area selector may show evidence only, not competing recommendations.

## 2. Architecture and repository

Submission boundary (latest user instruction): the offline download/aggregation tooling is a separate local-only Go module in Git-ignored `data-preparation/` and is excluded from Docker/submission. The assignment module contains `cmd/server`, `cmd/seed`, shared `internal/dataset` artifact readers/validation, `internal/seeding`, migrations, runtime services, and prepared data. Parquet and offline preparation configuration are not dependencies of the assignment. The local preparation module may consume shared seed/domain contracts through a local replace while preparing artifacts, but the assignment builds and tests without it.
Go net/http + pgx, PostgreSQL, React/TypeScript/Vite, Docker Compose, one modular monolith. Offline prepare-data -> three versioned JSON artifacts -> seed -> PostgreSQL. Runtime: historical SQL -> immutable season/day/hour stats cache; live Open-Meteo -> transactional forecast snapshot -> pure Go scorer -> atomic result cache -> REST -> React. One sequential scheduler. No Redis, Kafka, ML, authentication, spatial DB or distributed jobs.

```
cmd/{prepare-data,seed,server}/
internal/{config,ingestion,database,repository,weather,recommendation,cache,scheduler,api}/
database/migrations/000001_init.{up,down}.sql
frontend/{Dockerfile,nginx.conf,src/}
Dockerfile
compose.yaml
.env.example
data/
README.md
plan.md
DESIGN_AND_SCALABILITY.md
TRADEOFFS.md
MY_REASONING.md
AGENTS.md
```
Use explicit dependency injection, small repository/weather/cache interfaces and pure scoring. Use typed immutable snapshots for coherent publication; no generic byte-cache or Redis implementation is needed. Recommendation reads share read-only evidence and prepare API responses separately; historical/forecast cache ownership remains protected.

### 2.1 Repository boundary (required)
Use small domain-oriented interfaces, not a generic CRUD repository or a database abstraction that exposes SQL. Define domain structs and repository contracts in `internal/repository` (or consumer-owned interfaces with shared domain types in `internal/domain`); implement only PostgreSQL in `internal/repository/postgres`. `internal/database` owns pool creation and lifecycle. The composition root `cmd/server` constructs the adapter and injects interfaces into services. API handlers call services; the scorer is pure and receives typed evidence, never a database connection. Scheduler orchestrates services, not queries.

Illustrative contracts (names may be refined before implementation; types are plain Go domain values):
```go
type AreaRepository interface {
    ListAreas(ctx context.Context) ([]Area, error)
}
type HistoricalRepository interface {
    AggregateHourly(ctx context.Context, query HistoricalQuery) ([]HourlyStats, error)
}
type ForecastRepository interface {
    LoadLatest(ctx context.Context) (ForecastSnapshot, error)
    ReplaceSnapshot(ctx context.Context, snapshot ForecastSnapshot) error
}
```
HistoricalQuery contains rainfall threshold; HourlyStats contains area, season, ISO day, local hour (0–23), nullable dry/rain means, sample counts, nullable absolute/percentage historical uplift and rainy sample standard deviation. Cache keys remain service-owned. ForecastSnapshot carries version, fetched time and complete rows; replacement must be atomic from readers' perspective. Missing snapshot has an explicit domain error; missing evidence is not a database error. Use context cancellation and wrap underlying errors with `%w`; translate driver-specific errors at adapter boundary without losing diagnostic causes. No pgx rows, pool, SQL strings, transaction handles or driver-specific types in service interfaces. All runtime SQL, row scanning, null conversion and forecast transaction/COPY belong to PostgreSQL adapter. Seed/setup may use dedicated PostgreSQL-specific loader and migrations; do not force bulk loading into runtime interfaces. Keep actual SQL aggregation in PostgreSQL, not in Go, to satisfy assignment. Use simple fakes for service tests and actual PostgreSQL for SQL integration.

Future MySQL adapter implements equivalent aggregation (e.g. CASE instead of FILTER), schema, indexes and snapshot transactions. DynamoDB requires different access patterns, precomputed statistics or another aggregation pipeline and versioned snapshot publication; it is not a drop-in SQL replacement. Business scoring, scheduler, API and frontend should remain unchanged if repository semantics are preserved. Connection wiring, migrations, seeding, storage design and adapter integration tests necessarily change. Do not implement alternative adapters, ORM, generic query builder or runtime database switching now.

## 3. Configuration
```dotenv
CITY_TIMEZONE=America/New_York
HISTORICAL_START=2022-01-01
HISTORICAL_END_EXCLUSIVE=2026-01-01
MIN_HISTORICAL_SAMPLES=5
MIN_RAINFALL_MM=0.5
MIN_ADDITIONAL_TRIPS_PER_HOUR=50
RECOMMENDATION_WINDOW=2h
RECOMMENDATION_REFRESH_INTERVAL=1h
WEATHER_REFRESH_INTERVAL=3h
SCHEDULER_CHECK_INTERVAL=15m
WEATHER_MAX_STALENESS=6h
WEATHER_FORECAST_HOURS=30
HTTP_ATTEMPT_TIMEOUT=10s
EXTERNAL_REQUEST_TIMEOUT=20s
MAX_ATTEMPTS=3
RETRY_INITIAL_DELAY=500ms
RETRY_MAX_DELAY=5s
WEATHER_JOB_TIMEOUT=2m
HISTORICAL_QUERY_TIMEOUT=60s
RECOMMENDATION_JOB_TIMEOUT=30s
DB_TIMEOUT=15s
SHUTDOWN_TIMEOUT=15s
FRONTEND_POLL_INTERVAL=60s
ARCHIVE_WORKERS=2
MAX_OPERATIONAL_AREAS=20
WEATHER_BATCH_SIZE=20
WEATHER_FETCH_WORKERS=2
MOBILITY_FETCH_WORKERS=1
PARQUET_READ_BATCH_SIZE=10000
MOBILITY_REQUEST_TIMEOUT=120s
MOBILITY_CONNECT_TIMEOUT=10s
```
Validate positive Parquet batch size. Validate finite positive rain threshold and additional-trips threshold, samples >0, positive durations, forecast coverage >= horizon + max staleness + two hours, valid timezone, date ranges and hourly seed resolution. Mobility and weather resolution are fixed at one hour, not configuration options. Require RECOMMENDATION_WINDOW to be a positive whole number of hours. Use Go time.Duration; one configured horizon, not selectable simultaneously. Configure API URLs, quota budgets, database credentials separately. Defaults are heuristics, not calibrated.

## 4. Offline data preparation
Use NYC TLC High Volume For-Hire Vehicle (HVFHV) monthly Parquet, 2022–2025 inclusive, configurable whole-month date bounds. Official source: https://www.nyc.gov/site/tlc/about/tlc-trip-record-data.page . Download URL template: `https://d37ci6vzurychx.cloudfront.net/trip-data/fhvhv_tripdata_YYYY-MM.parquet`. Official zone lookup: `https://d37ci6vzurychx.cloudfront.net/misc/taxi_zone_lookup.csv` (verify lookup schema). Use the official lookup to exclude unknown, outside-city and non-geographic IDs; do not assume contiguous IDs. Do not download or process taxi-zone polygons. No Python scripts, GIS libraries, centroid calculations, coordinate transformations or automated coordinate discovery.

After ranking zones, generate `community_areas.json` with official IDs/names and placeholder latitude/longitude `0, 0`. Preserve manually entered coordinates on reruns for matching IDs. The developer will manually replace placeholders with actual representative coordinates obtained from internet sources. This is a one-time manual activity, not an engineering task. Allow mobility preparation to complete with placeholders; stop before historical weather fetching with a clear list of unresolved area IDs. Resume preparation after editing the JSON, reusing monthly checkpoints. Validate finite coordinates, latitude [-90,90], longitude [-180,180], and reject the placeholder pair (0,0) before any weather call or final seed publication. Use the same verified coordinates for archive and forecast; record coordinate sources in preparation notes. Do not ship placeholder coordinates in final reviewer seed data. Synthetic coordinates are acceptable only in isolated tests with mocked weather.

### 4.1 Mobility preparation and stable contract
Go prepare-data orchestrates sequential monthly downloads to temporary files, validates successful HTTP response and readable Parquet schema, and aggregates in Go using a pinned `github.com/parquet-go/parquet-go` dependency. No DuckDB or other analytical database is permitted. Pilot one month, inspect physical/logical types and benchmark before expanding. Use bounded retries and dedicated download timeout; checkpoint by source, month, aggregation version and lookup version (not manually edited coordinates); never reuse partial files. Keep raw files only until successful checkpoint.

Project only `pickup_datetime` and `PULocationID` using a typed Parquet reader; verify actual integer width, nullability, timestamp unit and UTC-adjusted flag before choosing Go types. Preserve local wall-clock semantics for naive timestamps; do not blindly interpret them as UTC. Read bounded batches (initially 10,000 rows), process any returned rows before handling EOF, fail on other read errors, and close reader/file on every path. Check cancellation between batches. Maintain a monthly map keyed by area ID and normalized UTC hour with int64 counts, plus per-area totals. Illustrative algorithm:
```text
for each month:
    download to temporary file and validate schema
    for each projected batch:
        validate each pickup zone and timestamp
        normalize local hour to UTC (exclude ambiguous DST dates)
        increment monthly hourly count and area total
    validate counts and sort hourly keys deterministically
    atomically publish all-zone monthly checkpoint and totals
    delete raw file only after successful publication
merge checkpoint totals once per month; rank areas
stream selected hourly records from checkpoints into gzip JSON
```
Do not retain raw trips or all months' hourly maps in memory. At 263 illustrative zones, one 31-day month has at most 195,672 hourly keys before exclusions. Measure actual memory, elapsed time and rows processed; no performance guarantee. Validate checkpoint counts against accepted records and record rejection reasons. Resume without double-counting completed months. PostgreSQL still performs the meaningful historical weather-conditioned SQL aggregation at runtime; Go only converts raw mobility trips into hourly counts.

Validate pickup timestamp semantics against TLC dictionary; treat naive timestamps as America/New_York local time, exclude ambiguous DST transition dates conservatively, convert to UTC. Filter to valid NYC zones using metadata. Count completed trip records, not requests or supply. Aggregate all valid zones first, then rank by total valid pickups across the entire configured period (tie by ID); select top MAX_OPERATIONAL_AREAS=20, or all valid geographic zones when 0. Generate the editable area metadata file after selection; freeze scope in artifact manifest; do not rerank hourly. Retain all-zone monthly aggregate checkpoints for future expansion. Fill absent hours with zero only for verified complete source coverage; otherwise unknown. Report rejected/out-of-month records and missing months. Do not claim full-city coverage for selected zones. Optional held-out evaluation must choose zones using training data only to avoid selection leakage.

The normalized contract is identical to Chicago: area ID/name/latitude/longitude, hourly trip count keyed by area ID and UTC hour_start, and hourly weather keyed by the same pair. No TLC columns or Parquet library types leak into runtime repositories or scorer. Keep existing `community_areas` table and artifact name as compatibility names for generic operational areas; UI calls them taxi zones. Do not hardcode 20, 77 or 263 in runtime. No Chicago ingestion adapter needed now.

### 4.2 Historical and live weather
Archive endpoint `https://archive-api.open-meteo.com/v1/archive`; forecast `https://api.open-meteo.com/v1/forecast`. Require manually populated, validated coordinates before any provider call. Historical requests: one selected zone and one calendar year per request, latitude/longitude, start_date/end_date inclusive, timezone=GMT, hourly=temperature_2m,rain,snowfall,apparent_temperature,precipitation,wind_speed_10m. Pad UTC boundaries to cover local dates and preceding-hour accumulation. Two workers share a weighted quota limiter; checkpoint each validated zone/year, resume without duplicate requests. Split failed long requests only for transient size/time problems, not to evade quota. Verify current provider accounting at https://open-meteo.com/en/pricing before execution. Planning assumptions: 600/min, 5000/hour, 10000/day, 300000/month; long archive calls roughly days/14 per location, round up conservatively and reserve retry/live headroom. Twenty zones x four years ~2,087 estimated units before rounding/padding; 263 illustrative zones ~27,446, requiring multiple days on free quota. Persist quota usage/checkpoints across restarts; stop/defer when budget exhausted, honor Retry-After. Workers alone do not enforce quotas. No automatic spatial deduplication.

Live: dynamically list seeded areas, batch up to WEATHER_BATCH_SIZE=20 coordinate pairs, two bounded workers, shared quota limiter, three-hour refresh. Example GET parameters latitude=40.7580,40.7128; longitude=-73.9855,-74.0060; hourly=rain,snowfall,precipitation_probability; forecast_hours=30; timezone=GMT. Validate response count/order and coordinate association (returned grid coordinates can differ), arrays, complete hours and all selected IDs. Batch reduces HTTP overhead not location quota. At 20 zones ~160 location units/day; 263 ~2104, excluding retries/other multipliers. Publish only complete validated snapshot; retain prior usable snapshot on failure. No frontend-triggered provider calls. Adjust job deadline after measuring larger scope. Verify accumulation/probability interval semantics: shift preceding-hour rain/snow to hour start; instantaneous temperature uses hour-start value. Do not delete individual array elements. Missing weather excludes pair, not zero.

### 4.3 Artifacts and validation
Preserve `community_areas.json`, `hourly_trips.json.gz`, `historical_weather.json.gz` with shared version, source dates, selected IDs, timezone, resolution, counts and records. Stream gzip/JSON, publish validated complete set atomically; mandatory fields validated, optional nulls retained, identical duplicates dedup, conflicting duplicates fail. Seed merge by area/hour unchanged. Record coverage and unmatched rows, source URLs, Go aggregation version, selection rule, model and coordinates. Ship prepared seed artifacts for offline reviewer setup; no historical downloads on runtime startup. No raw or subhour artifacts shipped. Maximum 20*1461*24=701,280 hourly rows per source before exclusions. All-zone expansion: set MAX_OPERATIONAL_AREAS=0, reuse monthly checkpoints, manually populate coordinates for newly selected zones, fetch missing zone weather across quota windows, regenerate artifacts, reseed and restart. Runtime schema, SQL and scoring unchanged; benchmark startup, cache, forecast deadline and UI at larger scope. Shared weather locations are a future optimization, not required now.

## 5. Three-table database and indexes
One initial up/down migration pair; no migration framework. Proposed schema (no rigid numeric CHECK constraints; validate values in Go on every write):
```sql
CREATE TABLE community_areas (
 id INTEGER PRIMARY KEY, name TEXT NOT NULL,
 latitude DOUBLE PRECISION NOT NULL, longitude DOUBLE PRECISION NOT NULL
);
CREATE TABLE historical_observations (
 area_id INTEGER NOT NULL REFERENCES community_areas(id),
 hour_start TIMESTAMPTZ NOT NULL,
 local_date DATE NOT NULL, day_of_week SMALLINT NOT NULL,
 season SMALLINT NOT NULL, hour_of_day SMALLINT NOT NULL, trip_count INTEGER NOT NULL,
 rain_mm REAL NOT NULL, snowfall_cm REAL NOT NULL,
 temperature_c REAL, apparent_temperature_c REAL,
 precipitation_mm REAL, wind_speed_kmh REAL,
 PRIMARY KEY(area_id,hour_start)
);
CREATE TABLE weather_forecasts (
 area_id INTEGER NOT NULL REFERENCES community_areas(id),
 hour_start TIMESTAMPTZ NOT NULL, forecast_version UUID NOT NULL,
 fetched_at TIMESTAMPTZ NOT NULL, rain_mm REAL NOT NULL,
 snowfall_cm REAL NOT NULL, precipitation_probability SMALLINT NOT NULL,
 PRIMARY KEY(area_id,hour_start)
);
CREATE INDEX forecast_time_area_idx ON weather_forecasts(hour_start,area_id);
```
Down migration drops forecasts, observations, areas in reverse order. ISO day_of_week Monday=1 through Sunday=7; normalize Go Sunday=0 to 7. Derive local_date, season, day_of_week and hour_of_day (0–23) from UTC in Go, validate consistency. Seed merges sorted trip/weather streams by area+hour (or bounded per-area merge), only complete valid pairs become observations; report unmatched counts. Keep separate artifacts for independent reprocessing. Seed via pgx.CopyFrom in transaction, areas first, rollback on failure; ANALYZE after commit. No staging domain tables required. Primary key indexes cover area/time; full historical startup aggregation may prefer a sequential scan; no historical secondary index by default; forecast time index candidate for configured-scope window. Consider (season,day_of_week,hour_of_day,area_id) only if selective queries are introduced. Benchmark with EXPLAIN (ANALYZE, BUFFERS), compare plans and timings, remove unused secondary indexes if justified. Sequential scan may be correct. No measured performance claims.

## 6. Fixed seasonal SQL and startup cache
Final choice: four meteorological seasons, Winter=1 (Dec–Feb), Spring=2 (Mar–May), Summer=3 (Jun–Aug), Autumn=4 (Sep–Nov). Derive season from New York local month during ingestion and use the same pure Go helper at runtime. All seven ISO days independently; same area, season, day and hour. No rolling dates, fallback or daily cache rebuild. November 30 and December 1 switch groups; document this boundary limitation. Exact date is sparse; all-year matching mixes seasons; fixed season is an explainable compromise, not proven more accurate.

One parameterized SQL aggregation at startup. Business question: for comparable local hours, how much higher or lower are completed trips in rainy conditions, and how variable are those observations? The database computes evidence, not the final recommendation.
```sql
WITH historical_stats AS (
    SELECT area_id, season, day_of_week, hour_of_day,
        AVG(trip_count) FILTER (WHERE rain_mm = 0) AS dry_avg,
        COUNT(*) FILTER (WHERE rain_mm = 0) AS dry_n,
        AVG(trip_count) FILTER (WHERE rain_mm >= $1) AS rain_avg,
        COUNT(*) FILTER (WHERE rain_mm >= $1) AS rain_n,
        STDDEV_SAMP(trip_count) FILTER (WHERE rain_mm >= $1) AS rain_stddev
    FROM historical_observations
    WHERE snowfall_cm = 0
    GROUP BY area_id, season, day_of_week, hour_of_day
)
SELECT *,
    rain_avg - dry_avg AS absolute_uplift,
    100.0 * (rain_avg - dry_avg) / NULLIF(dry_avg, 0) AS percentage_uplift
FROM historical_stats;
```
$1 is the configured rainfall threshold. AVG and COUNT compare like-for-like local hours; the CTE avoids repeating expressions; NULLIF prevents division by zero. STDDEV_SAMP measures dispersion of rainy hourly trip counts in trips/hour, not uncertainty of the estimated mean or causal effect. It is NULL for fewer than two rainy observations. Keep nullable means, uplift and standard deviation; do not substitute zero for missing evidence. Round only for display. Go applies the existing minimum sample rule (dry-only forecasts still need only dry evidence), forecast weighting, window summation, eligibility and ranking. No additional thresholds, confidence score, SQL ranking or schema changes.

Interview example (synthetic): same zone, winter Wednesdays 17:00, dry mean 1200, rainy mean 1700, dry n=35, rain n=12, rainy standard deviation 180. SQL reports +500 trips/hour and +41.7% historical uplift. At 90% forecast precipitation probability Go estimates +450 trips for that hour. The sample counts and dispersion help explain why the estimate may be wrong; neither proves causation nor measures unmet demand. Show rainy standard deviation in hourly insights alongside sample counts, not as a separate chart or ranking input. Explain that comparing equivalent seasonal weekday hours controls some calendar variation, not all confounders. One startup scan computes all reusable groups, avoiding repeated historical scans on each forecast refresh.

NULL means unknown, not zero. Snow and trace rain excluded from both groups. Map key (area, season, ISO day, hour), maximum N*4*7*24 (13,440 for 20 zones) entries. Immutable map loaded through HistoricalRepository; rebuild on restart after seed/config changes, not midnight. Startup query failure fails readiness/startup; do not publish partial statistics. At runtime convert each UTC forecast hour to New York local calendar fields. Cross-midnight/season transitions need only another map lookup. No GROUPING SETS, materialized views or historical secondary index required. Use EXPLAIN to inspect actual plan; full scan may be optimal.

Inspect real rainy/dry counts and coverage before demo. Five samples per class is a heuristic, not statistical significance. Optional time-boxed comparison against all-year day/hour baseline on held-out 2025 using 2022–2024; report MAE on common observations and coverage separately. Observed weather evaluation is conditional on known weather, not live forecast or intervention validation. No automated window selection or backtesting framework. If coverage is poor, report it and seek approval before changing grouping or thresholds.

## 7. Recommendation engine
Next complete UTC hour strictly after now through configured whole-hour duration. For each area/hour, look up season/day/hour historical statistics and aligned forecast. Rain >= threshold and no snow: require at least MIN_HISTORICAL_SAMPLES in each class. Let D=dry mean, R=rainy mean, p=precipitation_probability/100 (proxy for qualifying rain, not calibrated). Signed extra E=(R-D)*p; estimated trips D+E. Dry forecast rain=0 and snow=0: require dry evidence only, E=0, baseline D. Trace rain, snow, missing forecast or inadequate evidence -> unassessable, not zero. Sum signed E and D across complete horizon. Eligibility sum E >= MIN_ADDITIONAL_TRIPS_PER_HOUR * duration hours (100 for 2h). Rank by sum E descending, tie by area ID. No weights, decay, separate priority score or relative-uplift ranking. Relative uplift is evidence only if baseline>0. Example D=1200,R=1700,p=.9 ->450 extra; next dry D=1400 ->0 extra; total baseline2600, estimated3050, extra450, uplift17.3%. 40 evaluations for 2h at 20 zones. These are illustrative numbers, not measured data.

Require complete evidence across hours for each area. Preserve conservative configured-scope policy: if any area is unassessable, return insufficient_evidence with assessed/total and reasons, not a configured-scope winner. If this prevents useful demo, inspect actual coverage and obtain approval for assessed-area-only ranking with explicit scope; never silently relax. All assessed but none eligible -> no_intervention with highest modeled impact and threshold. Unavailable forecast is distinct from insufficient history. Show one action to review driver availability, not a driver allocation quantity. Completed trips are not demand or supply; weather association is not causation. Explain old data, holidays, pandemic, temperature, grid resolution, precipitation proxy and sample limitations.

## 8. Runtime, persistence and failures
Single scheduler checks every 15m, weather every 3h, recommendations every 1h and after successful weather refresh; initial jobs at startup. Historical seasonal cache built once before scoring; weather first if both due. Synchronous jobs, TryLock guard for concurrent manual tick, no overlap, bounded context timeouts, track last attempt/success separately, no unbounded queue. Fetch 30h rolling forecast with bounded workers and quota-aware batching, validate complete all-area snapshot and interval coverage; transaction delete+COPY new version, no network in transaction. Old version survives failure. Load persisted snapshot at startup; publish immutable cache after commit; reload if commit ambiguous, no blind write retry. Forecast age >=3h stale, >6h unusable; also require actual window coverage. Cached result valid only for current window, its own forecast version and age; suppress expired action, expose stale/degraded state if usable old result. Recompute when window rolls; valid_until earliest window rollover/forecast expiry. Cache TTL is not freshness.

Retry idempotent GET only for transient network, 408/429/5xx, bounded exponential jitter, Retry-After within budget; no retry malformed/permanent 4xx/cancel. Structured slog with op/run/version/area/attempt/duration/error chain, wrap %w, errors.Is/As, no secrets. Startup invalid config/schema/DB fatal; runtime transient degrade. signal.NotifyContext or cancel cause; stop new jobs, cancel workers, HTTP Shutdown with fresh timeout, wait for handlers/workers before DB close; force close HTTP on timeout, log cleanup errors, nonzero fatal exit, clean signal exit zero. No guarantees for SIGKILL.

## 9. API and frontend
GET /api/areas -> metadata. GET /api/recommendations -> status action_required/no_intervention/insufficient_evidence/unavailable; freshness, window start/end/duration_seconds, generated_at, forecast_fetched_at/version, valid_until, coverage assessed/total, primary_recommendation nullable, area evidence summaries, warnings. GET /api/areas/{id}/insights -> same version/window, hourly baseline/rain means/counts, nullable historical absolute/percentage uplift and rainy standard deviation, season, forecast, extra, aggregate evidence. GET /api/health -> ready/degraded or 503. 200 for usable no-intervention/insufficient; 503 unavailable with safe error code/message/request_id, null action. 400 invalid ID/query, 404 unknown, 405 method, 500 unexpected; log actual cause. Frontend polls 60s, compare version/window when fetching details, no stale action after valid_until. Minimal responsive UI, one action, evidence, uncertainty, area selector, loading/error states. No dashboard or map required.

## 10. Docker and reviewer setup
Pinned Go/Postgres/Node images, multi-stage Go build and React build served by Nginx same-origin API proxy. Compose db healthcheck, setup, api, frontend. Two commands:
```
docker compose run --rm --build setup
docker compose up --build api frontend
```
Setup explicitly resets only dedicated disposable mobility_control_room DB (connect to postgres admin DB, DROP/CREATE outside transaction; stop running app first), applies up migration, validates artifacts and seeds. Never reset on normal startup or delete entire volume. Ship validated seed artifacts or documented reproducible retrieval in setup; reviewer should not need lengthy live historical downloads. README explains localhost:3000, credentials/config, reset warning, test command and screenshot/demo. No local Go/Node/Postgres required.

## 11. Three focused automated tests
Extend the existing SQL integration test (not a fourth test) with known rainy counts 1500 and 1900, dry counts 1100 and 1300: assert means 1700 and 1200, uplift 500 and approximately 41.6667%, sample standard deviation approximately 282.8427, counts 2 each. Assert NULL standard deviation for a single rainy observation and NULL percentage for zero dry mean; verify trace rain and snow exclusion. Use float tolerances. Sample eligibility remains tested in Go.
1. Historical SQL integration on isolated PostgreSQL fixture: same season/ISO day/hour, Saturday/Sunday, Dec–Feb grouping, rain/dry/snow/trace, missing, counts/means. Verify real SQL.
2. Pure recommendation table-driven test: mixed rain/dry example, signed impact, duration threshold, absolute ranking and deterministic tie, cross-midnight/season lookup, insufficient vs no intervention, non-whole-hour horizon rejection and configurable values.
3. External API failure integration: httptest transient 503/timeout/invalid snapshot, bounded retry/cancel, previous version survives, stale/expired response semantics and safe error. Keep three top-level tests with focused subcases. Manual verification: scheduler overlap/cancellation, SIGTERM, API DB failure, seed rollback/reset, frontend, EXPLAIN, Docker two commands. Run go test ./... and race where feasible; record actual results only.

## 12. Incremental tasks (9.5h maximum planning estimate)
1. Scaffold/config/Docker (30m): build skeleton, invalid config fails; define repository contracts and composition boundary.
2. Data prep (90m): inspect Parquet schema, benchmark projected Go batch reader on one month, validate counts and checkpoints, then expand; generate area metadata with placeholders, wait for developer to supply real coordinates before weather fetching, then resume and validate artifacts and coverage; external downloads and manual coordinate entry separate.
3. Migration/seed/indexes (60m): three tables, transactional merge, reset and ANALYZE.
4. Seasonal SQL/cache (60m): PostgreSQL historical repository, real fixture, all seven days, season boundaries, EXPLAIN.
5. Live weather (60m): forecast repository and alignment, validated atomic snapshot, retry and persistence.
6. Scoring (60m): pure engine, signed impact/thresholds, evidence and status.
7. Scheduler/API/failure (75m): inject repositories through services, no direct driver imports in business layers, no overlap, freshness, shutdown, safe errors.
8. React (75m): one action, evidence and uncertainty, area details.
9. Tests/docs/demo (60m): three tests, README, screenshots, AI disclosure and final run.
Simplification should free time for coverage checks and interview preparation; actual savings unmeasured. If time tight, defer optional backtest, secondary index tuning and extra UI polish, not core correctness or tests. Record actual timings and blockers. Before submission verify real sample coverage; if insufficient, explain honestly and revisit fixed grouping/data coverage with approval. Update AI disclosure with actual Codex use, code review, rejected outputs and validation, not invented results.

## Dataset decision and measured experiments
Chicago TNP was the original source. Basic date-filtered retrieval (5 rows) succeeded in 1.294196s; single-day COUNT and seven-day aggregation timed out at 60s with no response. Five-column CSV returned 23,259 parsed rows in an incomplete 60s response (735,914 transferred bytes, 2.299 MiB local); two-column CSV requested 10,000 rows in 13.675634s (HTTP 200, 14,405 transferred bytes; actual row count not verified). The attempted bulk export endpoint returned HTTP 410 in 1.562915s; other exports were not ruled out. NYC January 2022 Yellow Taxi Parquet downloaded 38,139,949 bytes in 3.466139s; HVFHV downloaded 374,619,188 bytes in 31.026937s, both HTTP 200. These are user-measured download experiments, not controlled database comparisons. HVFHV is selected for marketplace relevance; Yellow Taxi is smaller but less directly representative. DuckDB was considered but explicitly rejected by the user to keep mobility preparation in Go. Go Parquet aggregation performance, timestamp compatibility, zone metadata and weather coverage remain untested. Four years and 20 zones are initial scope, not proven optimal. More trips per hour do not increase the number of independent rainy hours.


Coordinate decision: automated polygon processing was rejected by the user as unnecessary for the assignment. Use editable static metadata with initial placeholders and manually supplied real coordinates. No runtime architecture or recommendation changes.
