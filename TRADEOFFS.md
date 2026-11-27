# Design tradeoffs and limitations

The application estimates rain-related changes in completed trip activity for 20 selected NYC zones. Its output supports a review of driver availability; it does not measure waiting riders, available drivers or the number of drivers to move. The design favors an explainable comparison and a small runtime over a predictive model whose accuracy has not been established.

## Data and scoring decisions

| Decision | Reason | Accepted limitation |
|---|---|---|
| Use 20 manually selected zones with one representative coordinate each | Keeps the operational scope explicit and forecast requests manageable. | Other NYC zones and weather differences within a zone are not represented. |
| Compare the same zone, ISO day of week and New York local hour across all seasons | Preserves weekly patterns such as different Monday and Friday evenings. All 3,360 groups in the bundled data meet the minimum of five dry and five rainy observations. | Pooling seasons can hide differences between winter and summer. The previous season-and-hour grouping had more rainy observations per group. |
| Use a numeric rainfall threshold, default 0.5 mm/hour | Gives historical observations and forecast hours the same grouping rule. Dry conditions and lighter rain form the comparison group; rainfall at or above the threshold forms the rainy group. | Below-threshold rain has zero modeled additional effect. This is an assumption, not evidence that light rain has no physical effect. Snow remains excluded. |
| Require five historical observations per relevant class | Prevents calculating estimates from missing or extremely sparse evidence. | Five is a configurable coverage heuristic, not a statistical significance or confidence threshold. |
| Multiply the historical difference by precipitation probability | Produces a transparent calculation that can be traced to its inputs. | Any-precipitation probability is a proxy for qualifying rainfall. Intensity effects, interactions and uncertainty calibration are not modeled. |
| Rank signed additional trip totals rather than percentage increases | Prioritizes the largest modeled activity increase while preserving negative hourly effects. | Busier zones may dominate. Driver supply, intervention cost and revenue are not included. |
| Use a configurable intervention threshold, default 100 additional trips over two hours | Separates a modeled increase from an action worth reviewing. | The threshold is not calibrated against operational outcomes. |

The original season, exact day and hour grouping qualified 28.42% of historical groups and covered 0 of 20 zones in the inspected rainy window. The later season-and-hour grouping pooled all seven days. It and the final exact-day grouping both qualified 100% of their historical groups and covered 20 of 20 zones. Mean rainy samples per group fell from 25.26 to 14.43 with the final grouping. The decision favors keeping weekly patterns and using one fixed comparison. The evidence covers sample availability. Predictive accuracy remains untested. [Comparison results](plan.md)

## Evidence coverage and user decisions

A zone needs a known rain impact for every hour before receiving a window impact. Below-threshold hours contribute zero even if their typical trip count is unknown. Full-window typical and expected trip totals still require a baseline for every hour. Qualifying rain with insufficient history leaves impact unknown.

Complete zones are ranked even when other zones cannot be assessed. This replaces the original all-zone rule, which could suppress an eligible action because evidence was missing elsewhere. The UI reports assessed/total coverage and warns that excluded zones may have higher impact. The recommendation is the highest eligible estimate among assessed zones, not a claim about all monitored zones.

With partial coverage and no qualifying action, the decision is insufficient evidence. With full coverage and an entirely below-threshold forecast, the decision is no rain-related action needed. When the forecast is unavailable, the decision is also unavailable.

## Scheduling and storage decisions

Prepared CSV files replace the original nested JSON seed. PostgreSQL's `psql` client imports them directly, removing the Go JSON reader, batching and seed-specific types. The smaller files fit in regular Git, and the API image no longer contains seed data or a seeding executable. This makes setup PostgreSQL-specific and requires the `psql` client or its Docker image. Explicit reset recreates only the assignment tables, within the import transaction, rather than dropping the database. Import failures roll back schema changes and both CSV loads.

Setup skips the CSVs when both seed tables are populated and preserves saved forecasts. It rejects a partial schema or only one populated seed table. This keeps repeat setup cheap, with the trade-off that it cannot detect arbitrary edits inside a populated dataset. Obsolete development schemas require an explicit reset and reseed, which removes saved forecasts along with the other assignment data.

| Decision | Reason | Accepted limitation |
|---|---|---|
| Assess the next two complete future hours | Keeps forecast intervals aligned with full hourly historical observations. | The result does not cover the remainder of the current hour. At 10:53, the window is 11:00-13:00, not a remaining-time estimate. |
| Aggregate historical evidence once at API startup | Keeps full-history scans out of scoring and HTTP requests. | Historical-data or grouping-configuration changes require restart. Embedded SQL changes also require rebuilding. |
| Refresh weather every three hours, compute recommendations hourly, and check every 15 minutes or at the next hour boundary | Separates provider usage, decision validity and scheduling cadence. | Sequential execution means a slow weather job can delay recommendation computation. Replacement at expiry is not instantaneous. |
| Retry temporary provider failures within a bounded request | Handles short failures without unbounded requests or waits. | After a failed fetch, the next scheduler weather attempt normally waits for the three-hour interval from the previous attempt. |
| Retain stale weather within a maximum age | Allows continued operation through a provider outage without silently treating old data as fresh. | Defaults of three hours to stale and six hours maximum age are operational choices, not validated accuracy guarantees. Version, coverage and expiry checks can still make a result unavailable. |
| Persist complete forecasts transactionally in PostgreSQL | Supports restart recovery and avoids publishing partial writes. | Only the latest complete snapshot is stored; this is not a forecast archive. |
| Use one scheduler loop that owns both job attempt times | Makes execution order and retry timing visible in one place. Failure details are available in logs. | Each scheduler instance must have one Run invocation. Concurrent manual triggers would require revisiting synchronization. |
| Publish recommendations through an atomic pointer and treat nested evidence as read-only | Avoids deep-copying hourly evidence on every HTTP request. API response preparation copies only the headers or warnings it changes. | Immutability is a caller contract. Atomic replacement does not protect shared data from arbitrary mutation. |

The application assumes one instance. Distributed scheduling, shared recommendation caches and automatic historical refresh are deferred. Actual concurrent access remains protected where HTTP readers share caches with background writers. The full execution flow is documented in [Scheduler and recommendation architecture](docs/SCHEDULER_AND_RECOMMENDATIONS.md).

## Presentation and validation scope

The controller view shows one primary action, coverage, supporting hourly evidence and concise warnings. It avoids presenting sample counts or historical variation as calibrated confidence. The label "Typical dry-weather trips" is retained for readability; its tooltip explains that the comparison includes light rain below the threshold.

Trip quantities are rounded only when serialized to JSON. Scoring and ranking retain full precision, missing values remain null, and percentages and weather measurements retain decimals. Hourly estimates and window totals are rounded independently, so displayed values may not add up exactly.

The tests cover a recommendation with partial coverage, a PostgreSQL round trip that exercises historical rain classification and forecast precision, and provider failure. Separate development checks covered daylight-saving transitions, Docker setup and browser interactions. These checks establish application behavior. Predictive accuracy, causal rain effects and the operational impact of acting on a recommendation remain unmeasured.

The separate rain demo uses explicitly fictional inputs and the actual scoring engine. It demonstrates calculation and presentation, not live forecast accuracy or the production refresh lifecycle. Authentication, supply-demand integration, additional weather models and outcome evaluation are outside the implemented scope.

Historical associations do not establish causation. Holidays, events, pricing, road closures and changes in travel behavior can affect completed trips. No held-out accuracy, causal effect or intervention benefit is established by this implementation.

The source experiments, rejected approaches and incremental changes leading to these decisions are recorded in [implementation reasoning and development history](MY_REASONING.md#earlier-discussions-and-decisions).

## Extensibility and scalability

### Planning window and data granularity

The implemented planning window is configurable independently of the fixed hourly historical and forecast inputs.

| Decision | Reason | Accepted limitation |
|---|---|---|
| Hourly observations | Aligns trip and weather inputs. | Within-hour activity spikes are not represented. |
| Shorter planning windows | Give more temporally specific recommendations. | Less preparation time and potentially more variable decisions. |
| Longer planning windows | Give operators more preparation time. | Negative hourly effects can offset significant increases elsewhere in the window. |
| One configured window per process | Keeps scheduling and the published result simple. | Users cannot request different horizons simultaneously. |

`RECOMMENDATION_WINDOW` accepts positive whole-hour durations, including 1, 2, 3, 6, 9 and 12 hours. These require no schema or scoring changes. With the default `MIN_ADDITIONAL_TRIPS_PER_HOUR=50`, the implemented threshold is 50 multiplied by the window's duration in hours.

| Planning window | Default intervention threshold |
|---|---:|
| 1 hour | 50 trips |
| 2 hours | 100 trips |
| 3 hours | 150 trips |
| 6 hours | 300 trips |
| 9 hours | 450 trips |
| 12 hours | 600 trips |

Configuration requires `WEATHER_FORECAST_HOURS >= window hours + 1`, because stored coverage starts in the current hour and planning starts in the next complete hour. This is a minimum at fetch time, not a guarantee until the next refresh: the horizon must also cover advancing windows while a snapshot is reused. The default is 30 forecast hours. Provider responses must supply the requested hours; accepted snapshots contain the same contiguous UTC-hour range for every monitored zone. Runtime checks require coverage of the entire current planning window and acceptable forecast age. See [configuration validation](internal/config/config.go) and [forecast lifecycle](internal/usecase/weather/service.go).

Simultaneous horizons remain a future improvement requiring results indexed by window and scoring configuration, scheduling changes and an API contract for selecting a horizon. Sub-hour windows are currently rejected. Supporting them would require reprocessing historical trips, compatible finer weather inputs, and changes to hourly validation, grouping, scheduling and thresholds. Splitting hourly trip totals uniformly would introduce an unsupported assumption.

For 20 zones over 2022–2025 (1,461 days), theoretical observation counts are:

| Granularity | Potential observations | Relative row count |
|---|---:|---:|
| 1 hour | 701,280 | 1x |
| 30 minutes | 1,402,560 | 2x |
| 15 minutes | 2,805,120 | 4x |
| 5 minutes | 8,415,360 | 12x |
| 1 minute | 42,076,800 | 60x |

These precede missing/invalid-data exclusions; the bundled dataset contains 697,440 hourly observations. Finer resolution does not establish better predictive accuracy.

### Caching and horizontal scaling

The implemented caches belong to use-case services, rather than the PostgreSQL repository.

| Owner and cached data | Reason | Accepted limitation |
|---|---|---|
| History service: zones and indexed historical aggregates | Avoids repeated historical aggregation. | Each process loads its own copy at startup; changes require restart. |
| Weather service: latest forecast snapshot | Avoids database reads during scoring and freshness checks. | Each process restores or publishes its own snapshot and does not automatically observe another instance's writes. |
| Recommendation manager: latest computed result | Keeps HTTP requests inexpensive. | Process-local result is lost on restart and must be recomputed. |
| No distributed cache | Keeps deployment and dependencies small. | Cross-instance cache consistency is not implemented. |

PostgreSQL persists the latest complete forecast for recovery; it does not persist historical aggregates or computed recommendations. The weather service clones published forecast hours and returns caller-owned copies for scoring; status checks avoid copying. Recommendation reads share nested evidence under the read-only contract documented above.

Multiple instances would duplicate memory, computation and scheduled provider calls, and could serve different versions. PostgreSQL serializes forecast replacement transactions, but this does not elect a scheduler owner or synchronize in-memory caches. A dedicated worker or leader election, plus an explicit snapshot propagation strategy, would be future work.

A shared cache such as Redis is optional future infrastructure. Keys and invalidation would need historical dataset and forecast versions, planning window and scoring configuration. Network latency, serialization and failure handling are additional costs; a shared cache alone does not coordinate jobs or guarantee consistency.

### Concurrency and dashboard interactions

| Implemented capability | Reason | Accepted limitation |
|---|---|---|
| Sequential scheduler jobs | Keeps job order and attempt ownership explicit. | Slow weather refresh delays recommendation computation. |
| Separate process-local update mutexes in weather and recommendation services | Rejects overlapping updates within each service. | Does not coordinate processes or queue/deduplicate requests. |
| Atomic snapshot publication | Readers see a complete published snapshot. | Shared recommendation evidence must remain immutable. |
| Precomputed ranking and zone insights | Dashboard reads do not trigger scoring. | Request-specific windows and thresholds are unsupported. |
| Frontend polling every 60 seconds, with expiry refresh | Keeps update delivery simple. | Poll traffic grows with users and updates are not immediate. |

The scheduler still requires one `Run` invocation per instance; service mutexes do not protect its attempt timestamps. Existing insight requests can identify the expected forecast version and window start to detect a changed result.

Custom scoring or manual computation would need an API and concurrency policy beyond the current rejection of overlapping updates. Request deduplication, bounded concurrency, cancellation and version-aware publication are possible improvements. Server-Sent Events or WebSockets could replace polling if immediate delivery becomes necessary.

### Database portability

| Implemented decision | Reason | Accepted limitation |
|---|---|---|
| Use-case interfaces for areas, historical statistics and forecast storage | Separates business logic from PostgreSQL access. | Each database still needs its own queries and transaction behavior. |
| Aggregate history in PostgreSQL | Avoids loading raw observations into Go for aggregation. | A non-relational store needs a different aggregation strategy. |
| PostgreSQL CSV setup and forecast replacement | Provides efficient imports and atomic snapshot persistence. | Setup, SQL, table locking and bulk-copy operations are database-specific. |

MySQL support would require a repository adapter and changes to SQL, migrations and setup. DynamoDB would additionally require access-pattern design, potentially precomputed historical statistics, and a way to preserve complete snapshot publication. Neither backend is implemented; interfaces do not make databases interchangeable.

### Recommendation model improvements

These are future options; the current scoring and evidence limitations are documented above.

| Current approach | Potential improvement | Additional trade-off |
|---|---|---|
| Pool seasons within zone, weekday and hour | Seasonal comparisons or weighting | Smaller effective samples and more model complexity. |
| Binary rainfall classification | Model rainfall intensity | Requires evidence across intensity ranges. |
| Historical averages | Recent trends or regression estimates | Requires calibration and predictive validation. |
| One coordinate per zone | Multiple weather locations | More weather inputs, storage and aggregation. |
| 20 selected zones | Broader NYC coverage | More data preparation, provider usage and processing. |
| Completed trips as a mobility proxy | Rider demand and driver availability | Requires marketplace data. |
| Fixed configured threshold rate, scaled by window | Calibrate against costs and outcomes | Requires intervention and outcome data. |

Evaluate the existing heuristic on held-out historical data before adopting a more complex model. Additional complexity should earn measurable gains in prediction quality or operational usefulness.
