# Design considerations and scalability

Companion to plan.md. Current decisions are proposed implementation, not measured results. The assignment values one useful decision over infrastructure; production alternatives below are discussion material.

## Recommendation design history: alternatives and final decision
All counts below are approximate potential observations per area/hour before weather filtering, assuming four complete years. Selected dataset is NYC HVFHV 2022–2025, with missingness and changing travel patterns.

| Strategy | Potential samples | Pros | Cons / decision |
|---|---:|---|---|
| Exact month/day | 4 | Calendar-specific | Different weekdays, tiny rain sample; rejected |
| All-year same day/hour | 208 | Simple, more evidence | Mixes seasonal patterns; baseline for optional evaluation |
| Same month/day-of-week/hour | 17 | More seasonal relevance | Sparse rain, artificial month boundaries; rejected |
| Fixed season/day/hour | 52 | Simple SQL, reusable cache, explainable | Abrupt boundaries, sparse rain possible; selected |
| Rolling ±45 days | 52 | Smoothly moving calendar range | Date arithmetic, leap years, cache rebuild; no proven gain |
| Adaptive ±45/60/90 | 52/69/103 | More coverage when sparse | Extra rain samples may be only 3–4, broader seasons and inconsistent evidence; rejected |
| Distance-weighted calendar average | Varies | Smooth weights | Calibration, effective sample size; future |
| Regression / boosted trees / time series | Varies | Multiple factors, nonlinearities | Training, validation, serving, explainability; future |

Final: winter Dec–Feb, spring Mar–May, summer Jun–Aug, autumn Sep–Nov. All seven ISO days independently, same area and hour. Compare rainy/dry means and counts from the same group. Five samples per category is a heuristic, not statistical significance. Adding three observations to three can reduce sampling variance if representative, but cannot guarantee better accuracy; broader windows may increase bias. Fixed season is selected for clarity and scope, not because measured superiority has been established. No automatic fallback. Report insufficient evidence, coverage and old data/holiday/pandemic/temperature confounding. Conservative citywide coverage policy retained; assess actual coverage early and seek approval before relaxing it.

## Database, aggregation and indexes

The historical query now uses a CTE with a shared `WHERE snowfall_cm = 0`, computes dry/rain means and counts plus `STDDEV_SAMP` for rainy trip counts, and returns signed absolute/percentage uplift using `NULLIF(dry_avg, 0)`. Snow-only groups are absent; trace-only groups can remain with unknown class statistics. All derived values stay nullable and unrounded in the repository and immutable cache. Dispersion is descriptive trips/hour, not mean uncertainty, causal confidence or an eligibility/ranking input. The existing PostgreSQL integration fixture verifies the plan's 1200/1700 means, 500 uplift, 41.6667 percent and 282.8427 sample standard deviation, along with one-sample/zero-baseline NULL cases and negative uplift.

Implementation update: `internal/repository/postgres/historical.sql` is the actual parameterized seasonal aggregation. `internal/history` loads it once through repository interfaces into a bounded, immutable `internal/cache` snapshot; ingestion and lookup share `internal/domain.Calendar`. Tests exercise the same SQL against isolated PostgreSQL rows, including December/January/February grouping, all ISO weekdays, another season and zone, snow/trace exclusion, zero versus NULL, and missing evidence. The adapter's `ExplainHistorical` runs `EXPLAIN (ANALYZE, BUFFERS)` on this same SQL. Fixture query plans are inspection evidence only; full prepared-data startup performance remains unmeasured.
Three tables: areas, joined historical observations, persisted live forecasts. Separate offline artifacts allow regeneration. Four normalized tables would allow independent historical queries but exceed brief. Combined rows eliminate runtime join; unmatched observations excluded and reported. Primary key (area_id,hour_start) supports area/time. Prior day/date index suited rolling selective query, now removed because full startup aggregation reads all history. Consider (season,day_of_week,hour_of_day,area_id) only for proven selective workload. Forecast time/area index is a candidate; tiny table may prefer scan. Inspect EXPLAIN (ANALYZE, BUFFERS) after ANALYZE; indexes cost writes/storage. No measured performance claim.

| Aggregation | Pros | Cons / decision |
|---|---|---|
| Runtime SQL per refresh | Low startup | Repeated scans; rejected |
| Full startup seasonal SQL | One query, max 13,440 at 20 zones keys, no daily rebuild | Startup scan and memory; selected |
| Daily rolling cache | Calendar continuity | Date rollover and fallback complexity; rejected |
| GROUPING SETS | Multiple reusable levels | Unneeded hierarchy; deferred |
| Materialized view / summary table | Durable reusable stats | Refresh and schema complexity; deferred |
| Incremental stream rollups | Continuous updates | Event time, late data, operations; future |

Immutable map keyed by area, season, ISO day, hour; restart after historical data or rainfall threshold changes. Cross-midnight and season boundaries use existing keys. At larger scale use measured selective processing, shared summaries and partitioning.

## Duration, granularity and cadence
Independent dimensions: mobility resolution 1h, weather 1h, planning horizon 2h, recommendation refresh 1h, weather fetch 3h, scheduler check 15m. Use time.Duration for scheduling and whole-hour horizons, fixed hourly hour_start and hour_of_day (0–23). Source metadata must declare hourly resolution. 4/6/12/24h use same engine, UTC iteration, local date lookup; N*H evaluations (40/80/120/240/480 at 20 zones). Cross-midnight uses each hour’s season/day/hour key; forecast coverage >= horizon+staleness+alignment. Longer horizon threshold scales 50 trips/hour, but can dilute short spikes; future peak-hour gate. Multiple simultaneous horizons need API parameter, cache key/window/version and scheduling changes; prefix sums useful for frequent windows, preserve missing-evidence counts.

| Mobility resolution | Dense rows, 20 areas x 1461 days | Evaluations for 2h |
|---|---:|---:|
| 60m | 701,280 | 40 |
| 15m | 2,805,120 | 160 |
| 5m | 8,415,360 | 480 |
| 1m | 42,076,800 | 2,400 |

Theoretical, not measured. NYC pickup timestamps may support finer aggregation, but selected weather is hourly. Hourly total cannot be divided into quarters reliably (100/200/500/400 vs assumed 300 each). Future 15m ingestion reaggregate source, change SQL alignment to containing hourly weather interval, avoid summing repeated weather, rebuild stats; no exact subhour rain claim. Minute data requires new actual event source and suitable weather resolution. At scale use COPY, partitioning, retention and incremental 1m -> 5m/15m/1h rollups preserving sums/counts and classification; choose resolution compatible with requested window. Fetching every minute does not improve model resolution and increases quotas/writes. Current implementation has no configurable data resolution and rejects non-whole-hour horizons. No raw or 15-minute artifacts are generated; retain source references and reproducible queries. The selected historical and forecast weather feeds are hourly, so finer mobility alone cannot establish subhour rain timing.

## Scoring alternatives and final policy
E=(R-D)*p for qualifying rain; p is any-precipitation probability proxy, not calibrated rain probability. Dry E=0 with dry evidence; snow/trace unsupported. Sum signed effects, require complete area window, eligibility >=50 trips/hour times duration. Rank absolute additional completed trips, tie by area ID. Example rainy D1200,R1700,p=.9 ->450; next dry D1400 ->0; baseline2600, expected3050. Hypothetical only.

| Ranking | Pros | Cons / decision |
|---|---|---|
| Absolute extra trips | Direct operational scale, explainable | Favors busy areas; selected |
| Relative uplift | Highlights proportional changes | Tiny baselines can dominate; evidence only |
| Rain/dry weights | Flexible business priorities | Extra policy knobs, dry activity may distract; deferred |
| Temporal decay | Prioritizes nearer hours | Calibration, can reverse rank; deferred |
| Peak-hour risk | Captures spikes | Extra rule, longer windows can dilute spikes; future |
| Supply-gap/cost optimization | Direct operational objective | No live supply, requests or cost data; future |

Earlier weighted formula was sum decay^elapsedHours*(rainWeight*E+dryWeight*(1-p)*D). A with extra450/0 vs B100/400: unweighted B wins500 vs450, decay .8 A wins450 vs420. This illustrates why policy weights are not predictions and why we defer them. No weights in current config/API.

## Validation and industry context
Optional small held-out comparison: train on 2022–2024, evaluate 2025; compare fixed season vs all-year same-day/hour, report MAE on common cases, coverage separately, rainy-hour errors. Observed weather tests conditional estimates only; archived forecasts needed for end-to-end evaluation. No intervention outcome or causal validation possible. Do not claim results before running. Mature marketplace systems can combine live supply/demand, events, weather and learned forecasting with separate decision engines and outcome feedback; our dataset lacks those signals. Published reading leads (not reverified in this revision): Uber engineering forecasting introduction and airport driver availability, DoorDash engineering ensemble forecasting and causal forecasting. These motivate future work, not requirements or claims about current internal systems.

## Data preparation and weather geography
NYC HVFHV monthly Parquet, projected Go Parquet aggregation, official taxi zone lookup, editable JSON placeholders and manually supplied representative coordinates. Top 20 valid NYC zones by historical pickup volume; configurable all-zone expansion. Same hourly area contract, three artifacts and three runtime tables. Historical weather per zone/year with weighted quota pacing and checkpoints; live coordinate batches of 20. No spatial runtime or automatic grid deduplication. See plan.md for exact preparation contract.

## Reliability and scaling stages

Live weather implementation: seeded areas are fetched in configurable batches with bounded workers. Rain, snow and precipitation probability are all shifted from the preceding-hour label to the same hour-start interval. Explicit UTC `start_hour`/`end_hour` bounds include the extra boundary sample; Open-Meteo rejects combining these bounds with `forecast_hours`, so the client does not send both. Response counts, optional location IDs/order, grid-coordinate proximity, units, parallel arrays and complete hourly coverage validate before publication. Grid coordinates are allowed to differ from requested points; the documented response order associates them with the requested zones.

Forecast replacement uses one PostgreSQL transaction for validation, delete and COPY. Readers use one SELECT, and the in-memory snapshot is published only after commit. Unknown commit outcomes trigger a bounded reload and never a blind write retry; failed reconciliation suppresses the cache. Runtime and offline preparation share the persisted weighted quota ledger. The current single weather worker runs at startup and every configured weather interval; the combined weather/recommendation scheduler remains the later scheduler task. Historical-only or forecast-only startup never claims recommendation readiness.
Single instance: sequential scheduler, immutable versioned snapshots, persistent forecast, in-memory cache, bounded retry/timeouts, stale/expired distinction, graceful shutdown. No distributed lock needed. Multiple replicas: shared stats/results cache (Redis adapter), single elected scheduler or dedicated worker, fencing/version checks, transactional publication; Redis alone does not coordinate jobs. High event throughput: queue/stream only when continuous ingestion exists, idempotent consumers, event-time windows and backpressure. Large historical store: partition, incremental summaries, retention and query profiling. High frontend traffic: cached API, CDN static assets, conditional requests; no websocket needed for hourly changes. At 20 areas every 3h ~160 area forecast evaluations/day vs every minute ~28,800, before batching/quotas; actual API units differ. Monitor duration, failures, coverage, freshness, cache hit and DB plan; scale based on measured bottleneck.

## Interview extensions
Explain why seasonal matching instead of exact dates; why a full scan may beat an index for startup aggregation; why startup cache rather than repeated SQL; how 12h crosses midnight; why minute resolution needs new data; how forecast failure preserves prior version; why weights were deferred; why three tables; how to migrate to multi-instance safely. Demonstrate one actual query plan and actual tests after implementation. Current limitations: old historical data, sparse rain samples, strict citywide coverage may suppress action, uncalibrated thresholds, weather spatial/temporal resolution, no supply or causal inference. No benchmarks or validation completed in this planning document.

## Repository portability and storage migration
The Go service layer depends on domain-oriented AreaRepository, HistoricalRepository and ForecastRepository contracts. PostgreSQL adapter owns SQL, pgx, null mapping and atomic forecast replacement; the composition root injects it. Scoring remains pure. Use real PostgreSQL integration tests for SQL and fakes for service failure scenarios. Avoid generic CRUD, ORM and leaky query abstractions.

MySQL can implement equivalent aggregation using CASE expressions, appropriate indexes and transactions. DynamoDB needs access-pattern-driven keys and likely offline/precomputed seasonal summaries; do not scan raw history per recommendation. For atomic forecast publication, write immutable versioned items, verify completeness, then conditionally update a current-version pointer; readers use that version. These are future designs, not implemented. Migrations, seeding, connection wiring, operational guarantees and adapter tests must change; stable domain contracts protect scoring, scheduler and API from storage-specific changes. A repository does not make database migration free.

## All-zone extension
At illustrative 263 zones: 9,221,832 hourly observations before exclusions, 176,736 historical keys, 526 evaluations per two-hour window, 14 live HTTP batches per refresh and ~2,104 daily location units. Historical quota ~27,446 before padding/rounding/retries; spread over days or appropriate paid plan. No schema or scorer changes for independent zone weather. Future shared weather locations could reduce quotas but need validated grid mapping and potentially normalized five-table schema; deferred to preserve assignment scope. Revalidate thresholds, coverage, startup time, scheduler deadlines and frontend usability.

## Dataset decision and measured experiments
Chicago TNP was the original source. Basic date-filtered retrieval (5 rows) succeeded in 1.294196s; single-day COUNT and seven-day aggregation timed out at 60s with no response. Five-column CSV returned 23,259 parsed rows in an incomplete 60s response (735,914 transferred bytes, 2.299 MiB local); two-column CSV requested 10,000 rows in 13.675634s (HTTP 200, 14,405 transferred bytes; actual row count not verified). The attempted bulk export endpoint returned HTTP 410 in 1.562915s; other exports were not ruled out. NYC January 2022 Yellow Taxi Parquet downloaded 38,139,949 bytes in 3.466139s; HVFHV downloaded 374,619,188 bytes in 31.026937s, both HTTP 200. These are user-measured download experiments, not controlled database comparisons. HVFHV is selected for marketplace relevance; Yellow Taxi is smaller but less directly representative. Go Parquet aggregation on a full official month, official zone metadata and live weather coverage remain unverified. Four years and 20 zones are initial scope, not proven optimal. More trips per hour do not increase the number of independent rainy hours.
