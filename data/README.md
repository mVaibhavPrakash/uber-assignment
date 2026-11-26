# Data artifacts

The public-data path is `go run ./data-preparation/cmd/prepare-data -out data/public -start 2019-11-29 -end 2019-12-01` (end date exclusive). It starts with a two-day pilot. Preparation uses official community names, source-side hourly trip/centroid aggregations, modal published coordinates, and Open-Meteo archive weather. It publishes a new directory only after the three artifact files validate together. The output directory must not already exist.

No complete public dataset was published in this session. After initial sandbox restrictions, an approved live pilot retrieved official metadata for all 77 areas. The initial hourly trip query exceeded the 20-second runtime request budget. A retry with longer offline timeouts advanced through hourly trip aggregation, but centroid aggregation exhausted its three-minute request budget. Archive weather was not reached. `data/public` is therefore absent; a default public seed/setup correctly fails until preparation succeeds. Offline fixtures are never substituted for that missing directory. The provider request path and field names are only partially live verified.

Once source access is available, inspect the pilot and then prepare the full planned period into a new directory with `-start 2018-11-01 -end 2023-01-01`. Use `go run ./data-preparation/cmd/seed -data <directory> -validate-only` to inspect dropped records, missing hours and matched coverage. The pilot cannot provide five rainy/dry seasonal samples and does not validate recommendation accuracy. Preparation request limits count HTTP attempts, not provider quota units; long archive queries can cost more units. Set pacing and request budgets explicitly for wider runs.

Preparation currently retains source aggregates and prepared rows in memory, although JSON/gzip encoding and seed merging stream records. Full-period memory use and external query behavior remain unmeasured. A disk-backed per-area spool is a future improvement before scaling well beyond this dataset. No raw individual trips are requested or retained.

Provider references: [Chicago TNP](https://data.cityofchicago.org/resource/m6dm-c72p.json), [community areas](https://data.cityofchicago.org/resource/igwz-8jzy.json), [Open-Meteo historical weather documentation](https://open-meteo.com/en/docs/historical-weather-api), [SoQL query documentation](https://dev.socrata.com/docs/queries/). Archive accumulation at timestamp `t+1h` is assigned to interval starting at `t`; temperature and wind use the value at `t`. Provider array order and units must validate together. Historical rounded local trip timestamps exclude both DST transition dates.

`synthetic/` contains **fictional development fixtures**, not downloaded Chicago trips or weather. They do not satisfy the assignment's public mobility dataset requirement and must not be presented as measured evidence. Production ingestion must reject their `synthetic: true` metadata and incomplete two-area coverage. The full preparation path still needs all 77 official areas and validated public observations.

Regenerate from the repository root using only the Go standard library:

```sh
go run ./data-preparation/cmd/prepare-sample-data/main.go
```

Use `-out <directory>` to generate elsewhere. Generation is deterministic, including gzip headers. The command validates matched keys and nonnegative mandatory values, stages all three outputs, then publishes them. Separate file renames are not an atomic dataset transaction; consumers must validate metadata and coverage together before loading.

Each file is a JSON object with `dataset_version: "synthetic-fixture-v1"`, `synthetic: true`, `timezone: "America/Chicago"`, `resolution: "1h"`, `source_start: "2019-11-29"`, `source_end_exclusive: "2019-12-03"`, `record_count`, and a `records` array. Dates describe the Chicago local calendar interval; record timestamps are UTC instants.

| File | Records | Record fields |
|---|---:|---|
| `synthetic/community_areas.json` | 2 | `id`, `name`, `latitude`, `longitude` |
| `synthetic/hourly_trips.json.gz` | 192 | `area_id`, `hour_start`, `trip_count` |
| `synthetic/historical_weather.json.gz` | 192 | `area_id`, `hour_start`, `rain_mm`, `snowfall_cm`, nullable `temperature_c`, `apparent_temperature_c`, `precipitation_mm`, `wind_speed_kmh` |

Records are sorted by area then UTC hour, with one trip/weather pair per hour and area. Each area has 96 consecutive hours starting `2019-11-29T06:00:00Z`. This covers Saturday/Sunday, November 30 to December 1 (autumn/winter), valid zero trip counts, dry hours, qualifying rain (1 mm), snow (0.2 cm), trace rain (0.1 mm), and optional null weather values. Rain/snow already describe the hour beginning at `hour_start`; no provider timestamp conversion is required. Coordinates are illustrative Chicago coordinates and are not modal published centroids.

These small fixtures intentionally lack the five rainy/dry samples per seasonal day/hour needed by the default scoring policy. Do not weaken production thresholds to force an action from them. Real sample coverage, provider alignment, SQL performance, and prediction accuracy remain unvalidated.
