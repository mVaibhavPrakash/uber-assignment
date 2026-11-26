# Assumptions, trade-offs and limitations

| Choice or assumption | Reason and limitation |
|---|---|
| 20 manually selected zones with one representative coordinate each | Keeps the comparison manageable, but excludes the rest of NYC and cannot capture within-zone weather variation. |
| Same season, weekday and local hour; equal weighting across years | Explainable comparison with reusable SQL. It assumes historical comparability, introduces season boundaries and does not adapt to recent trends. |
| Fixed rain threshold and five observations per relevant class | Simple configurable eligibility. Neither the sample minimum nor the intervention threshold is statistically calibrated. Snow and trace rain are unsupported rather than treated as dry. |
| Linear probability-weighted rainy–dry difference | Easy to explain. It treats precipitation probability as a qualifying-rain proxy and does not model rain intensity, interactions or intervention outcomes. |
| Signed additional trips, ranked by absolute trip count rather than percentage | Prioritizes larger modeled activity changes and preserves decreases. It may favor busier zones; it does not account for available drivers, intervention cost or revenue. |
| Complete evidence across every monitored zone/hour | Avoids naming a scope-wide winner when a zone cannot be assessed, but one unsupported zone can suppress an action. No automatic fallback relaxes the rule. |
| Startup historical cache and one scheduler instance | Keeps request handling small and avoids repeated scans/provider calls. Multi-replica scheduling, distributed caches and automatic historical refresh are deferred. |
| PostgreSQL plus a small Go/React application | Meets the relational/SQL requirement with three tables and explicit behavior. Authentication, ML, maps and additional dashboards were omitted to keep focus on one decision. |

Historical associations do not establish causation. Old observations may not reflect current travel patterns, holidays, pricing or disruptions. Precipitation probability is a proxy for qualifying rain, and weather varies within zones. Counts and standard deviation describe historical evidence, not calibrated confidence. Only selected zones are monitored; the application cannot establish a driver shortage. The dashboard keeps controller-facing warnings concise and omits sample counts, historical-statistics tables and calculation details.

Backend JSON responses round trip quantities to whole integers; missing values remain null. Scoring, ranking and threshold comparisons retain full precision before serialization. Percentages and weather measurements retain decimals. Hourly values and window totals are rounded independently, so displayed hourly figures can differ slightly from a displayed total.

## Alternatives and history

The fixed-season comparison was chosen for explainability over exact-date, monthly, rolling-window or machine-learning approaches; no measured superiority is claimed. The selection was the user's edited 20-zone list, not an automatically selected top-20 list. See [recommendation alternatives](DESIGN_AND_SCALABILITY.md#recommendation-design-history-alternatives-and-final-decision) and [AI decision history](MY_REASONING.md#earlier-discussions-and-decisions) for the original discussion and source-retrieval experiments.
