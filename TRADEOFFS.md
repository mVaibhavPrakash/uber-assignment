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

The retained tests cover historical SQL grouping, the rainfall precision boundary, partial-coverage recommendations and provider failure. Separate development checks covered daylight-saving transitions, Docker setup and browser interactions. These checks establish application behavior. Predictive accuracy, causal rain effects and the operational impact of acting on a recommendation remain unmeasured.

The separate rain demo uses explicitly fictional inputs and the actual scoring engine. It demonstrates calculation and presentation, not live forecast accuracy or the production refresh lifecycle. Authentication, supply-demand integration, additional weather models and outcome evaluation are outside the implemented scope.

Historical associations do not establish causation. Holidays, events, pricing, road closures and changes in travel behavior can affect completed trips. No held-out accuracy, causal effect or intervention benefit is established by this implementation.

The source experiments, rejected approaches and incremental changes leading to these decisions are recorded in [implementation reasoning and development history](MY_REASONING.md#earlier-discussions-and-decisions).
