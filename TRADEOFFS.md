# Engineering trade-offs

The goal is one actionable, explainable recommendation within the 8–10 hour assignment, not a general analytics platform.

| Decision | Why / limitation |
|---|---|
| 20 selected NYC taxi zones, one manually supplied representative coordinate each | Simple geography and weather lookup; representative point and weather grid cannot represent every location. |
| Three source artifacts, projected Go Parquet aggregation | Independent regeneration and smaller downloads; requires version/coverage validation and merge. |
| Three DB tables | Combine historical trip/weather observations, retain areas and persisted forecasts; meets brief, but independent historical queries need original artifacts. |
| Minimal migration and Docker | Reproducible two-command setup; explicit reset only for disposable DB, production needs tracked migrations. |
| Fixed meteorological seasons, same day/hour | Balances seasonality and sample size without rolling arithmetic; abrupt boundaries and sparse rain remain. No adaptive fallback. |
| Startup seasonal SQL cache | One aggregation, max 13,440 at 20 zones keys; rebuild on restart after dataset/config changes. Startup scan cost. |
| Primary keys and candidate forecast index | Full startup aggregation may prefer sequential scan; defer historical secondary index until selective workload is measured. |
| Configurable duration and threshold; absolute signed extra trips | Simple ranking, favors busy areas; defer rain/dry weights and temporal decay. Defaults uncalibrated. |
| Fixed hourly data, 3h weather fetch | Both selected weather feeds are hourly; finer mobility alone cannot identify subhour rainfall. No shipped raw or 15-minute artifacts or dynamic buckets. Retain source references for future reaggregation. Fetch cadence may miss sudden changes. |
| Domain-oriented repository interfaces | Isolate PostgreSQL SQL, pgx and transactions; inject only one adapter now. MySQL/DynamoDB need new storage design, migration and tests, not changes throughout business logic. No generic CRUD or speculative adapters. |
| One scheduler and in-memory cache | Simple single-instance reliability; multi-replica coordination and Redis deferred. |
| Three focused tests | SQL, scoring and external failure; manual checks cover other operational scenarios. |
| One primary action, evidence and uncertainty | Avoid competing recommendations; insufficient evidence distinct from no intervention. |

Completed trips do not measure unmet demand or driver shortage. Rain probability is an approximation; old data, holidays, pandemic, temperature and sparse observations may mislead. Strict configured-zone coverage may yield insufficient evidence; inspect actual data before revising. No ML, Kafka, Redis, partitioning, minute ingestion or multi-horizon UI until justified. Index performance, sample coverage and heuristic accuracy remain unvalidated.

## Recommendation alternatives
Exact date: too few samples. All-year weekday/hour: more evidence but seasonal mixing. Monthly: sparse rain. Rolling ±45 and adaptive ±60/90: smoother calendar matching and possible coverage gain, but extra samples may not improve accuracy and wider windows mix seasons. Fixed seasons chosen for explainability, not proven accuracy. Relative uplift is evidence only; weighted ranking and ML deferred. Optional held-out comparison and actual coverage check before demo; no automatic fallback. See DESIGN_AND_SCALABILITY.md for full comparison.

NYC replaces Chicago after measured API failures; monthly Parquet is fast to download but requires the pinned Go Parquet reader and temporary disk. Top 20 limits weather quota and excludes other zones; disclose scope. All-zone support is a configuration/reseed operation with additional quota and performance validation. No runtime redesign.

## Dataset decision and measured experiments
Chicago TNP was the original source. Basic date-filtered retrieval (5 rows) succeeded in 1.294196s; single-day COUNT and seven-day aggregation timed out at 60s with no response. Five-column CSV returned 23,259 parsed rows in an incomplete 60s response (735,914 transferred bytes, 2.299 MiB local); two-column CSV requested 10,000 rows in 13.675634s (HTTP 200, 14,405 transferred bytes; actual row count not verified). The attempted bulk export endpoint returned HTTP 410 in 1.562915s; other exports were not ruled out. NYC January 2022 Yellow Taxi Parquet downloaded 38,139,949 bytes in 3.466139s; HVFHV downloaded 374,619,188 bytes in 31.026937s, both HTTP 200. These are user-measured download experiments, not controlled database comparisons. HVFHV is selected for marketplace relevance; Yellow Taxi is smaller but less directly representative. Go Parquet aggregation on a full official month, official zone metadata and live weather coverage remain unverified. Four years and 20 zones are initial scope, not proven optimal. More trips per hour do not increase the number of independent rainy hours.
