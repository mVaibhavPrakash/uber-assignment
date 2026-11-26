# Design tradeoffs and limitations

The application estimates rain-related changes in completed trip activity for 20 selected NYC zones. Its output supports a review of driver availability; it does not measure waiting riders, available drivers or the number of drivers to move. The design favors an explainable comparison and a small runtime over a predictive model whose accuracy has not been established.

## Data and scoring decisions

| Decision | Reason | Accepted limitation |
|---|---|---|
| Use 20 manually selected zones with one representative coordinate each | Keeps the operational scope explicit and forecast requests manageable. | Other NYC zones and weather differences within a zone are not represented. |
| Compare the same zone, season, weekday and local hour | Accounts for some calendar patterns with a reusable SQL aggregation. | Seasonal boundaries are abrupt; observations from different years receive equal weight and may not reflect recent behavior. |
| Use a numeric rainfall threshold, default 0.5 mm/hour | Gives historical observations and forecast hours the same grouping rule. Dry conditions and lighter rain form the comparison group; rainfall at or above the threshold forms the rainy group. | Below-threshold rain has zero modeled additional effect. This is an assumption, not evidence that light rain has no physical effect. Snow remains excluded. |
| Require five historical observations per relevant class | Prevents calculating estimates from missing or extremely sparse evidence. | Five is a configurable coverage heuristic, not a statistical significance or confidence threshold. |
| Multiply the historical difference by precipitation probability | Produces a transparent calculation that can be traced to its inputs. | Any-precipitation probability is a proxy for qualifying rainfall. Intensity effects, interactions and uncertainty calibration are not modeled. |
| Rank signed additional trip totals rather than percentage increases | Prioritizes the largest modeled activity increase while preserving negative hourly effects. | Busier zones may dominate. Driver supply, intervention cost and revenue are not included. |
| Use a configurable intervention threshold, default 100 additional trips over two hours | Separates a modeled increase from an action worth reviewing. | The threshold is not calibrated against operational outcomes. |

Exact-date comparisons were rejected because they provide few observations across years. Monthly groups can still have sparse rainy samples, while all-year comparisons mix seasons. Rolling windows, adaptive fallback and machine learning introduce additional choices that require validation. Fixed seasons were selected for explainability, not measured superiority. Alternative comparisons are detailed in [the architecture document](DESIGN_AND_SCALABILITY.md#recommendation-design-history-alternatives-and-final-decision).

## Evidence coverage and user decisions

A zone must have assessable evidence for every hour in its planning window before receiving a window total. Missing evidence remains unknown rather than becoming zero or a misleading partial estimate.

Complete zones are ranked even when other zones cannot be assessed. This replaces the original all-zone rule, which could suppress an eligible action because evidence was missing elsewhere. The UI reports assessed/total coverage and warns that excluded zones may have higher impact. The recommendation is the highest eligible estimate among assessed zones, not a claim about all monitored zones.

With partial coverage and no qualifying action, the UI states that no assessed zone reaches the threshold rather than declaring normal operations. If no zones are assessable, the result is insufficient evidence, or unavailable when forecast evidence is missing. Forecast freshness and complete provider-snapshot validation remain separate requirements.

## Scheduling and storage decisions

| Decision | Reason | Accepted limitation |
|---|---|---|
| Assess the next two complete future hours | Keeps forecast intervals aligned with full hourly historical observations. | The result does not cover the remainder of the current hour. At 10:53, the window is 11:00-13:00, not a remaining-time estimate. |
| Aggregate historical evidence once at API startup | Keeps full-history scans out of scoring and HTTP requests. | Historical-data or grouping-configuration changes require restart. Embedded SQL changes also require rebuilding. |
| Refresh weather every three hours, compute recommendations hourly, and check every 15 minutes or at the next hour boundary | Separates provider usage, decision validity and scheduling cadence. | Sequential execution means a slow weather job can delay recommendation computation. Replacement at expiry is not instantaneous. |
| Retry temporary provider failures within a bounded request | Handles short failures without unbounded requests or waits. | After a failed fetch, the next scheduler weather attempt normally waits for the three-hour interval from the previous attempt. |
| Retain stale weather within a maximum age | Allows continued operation through a provider outage without silently treating old data as fresh. | Defaults of three hours to stale and six hours maximum age are operational choices, not validated accuracy guarantees. Version, coverage and expiry checks can still make a result unavailable. |
| Persist complete forecasts transactionally in PostgreSQL | Supports restart recovery and avoids publishing partial writes. | Only the latest complete snapshot is stored; this is not a forecast archive. |
| Use one scheduler loop that owns its job state | Makes execution order explicit and removes unnecessary scheduler mutexes. | Each scheduler instance must have one Run invocation. Concurrent manual triggers or external state readers would require revisiting synchronization. |
| Publish recommendations through an atomic pointer and treat nested evidence as read-only | Avoids deep-copying hourly evidence on every HTTP request. API response preparation copies only the headers or warnings it changes. | Immutability is a caller contract. Atomic replacement does not protect shared data from arbitrary mutation. |

The application assumes one instance. Distributed scheduling, shared recommendation caches and automatic historical refresh are deferred. Actual concurrent access remains protected where HTTP readers share caches with background writers. The full execution flow is documented in [Scheduler and recommendation architecture](docs/SCHEDULER_AND_RECOMMENDATIONS.md).

## Presentation and validation scope

The controller view shows one primary action, coverage, supporting hourly evidence and concise warnings. It avoids presenting sample counts or historical variation as calibrated confidence. The label "Typical dry-weather trips" is retained for readability; its tooltip explains that the comparison includes light rain below the threshold.

Trip quantities are rounded only when serialized to JSON. Scoring and ranking retain full precision, missing values remain null, and percentages and weather measurements retain decimals. Hourly estimates and window totals are rounded independently, so displayed values may not add up exactly.

The retained suite has four tests: assessed-zone recommendation scoring, light rain without intervention, historical SQL comparisons, and provider outage with stale/expired forecast handling. Removing the larger case matrix and cache/concurrency checks reduces maintenance and explanation cost, but leaves less regression coverage for those behaviors.

The separate rain demo uses explicitly fictional inputs and the actual scoring engine. It demonstrates calculation and presentation, not live forecast accuracy or the production refresh lifecycle. Authentication, supply-demand integration, additional weather models and outcome evaluation are outside the implemented scope.

Historical associations do not establish causation. Holidays, events, pricing, road closures and changes in travel behavior can affect completed trips. No held-out accuracy, causal effect or intervention benefit is established by this implementation.

The source experiments, rejected approaches and incremental changes leading to these decisions are recorded in [implementation reasoning and development history](MY_REASONING.md#earlier-discussions-and-decisions).
