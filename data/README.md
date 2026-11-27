# Application seed

The prepared dataset consists of two CSV files:

| File | Contents |
|---|---|
| `public/areas.csv` | 20 selected NYC zones with names and representative coordinates. |
| `public/historical_observations.csv` | 697,440 matched hourly HVFHV trip/weather observations for 2022–2025. |

Both files include headers matching the database columns. Optional missing measurements use `\N`, which setup explicitly maps to SQL NULL. Valid zero trips and zero rainfall remain numeric zero. Each observation keeps its UTC timestamp, New York local date, ISO weekday and local hour. The 697,440 observations produce 3,360 groups in the startup cache.

Run `docker compose run --rm setup` from the repository root. [seed.psql](../database/seed.psql) creates the assignment tables when absent and copies both CSVs directly into empty tables. Repeat setup skips the CSVs and preserves populated data and saved forecasts. Setup rejects a partial schema or only one populated seed table. Schema creation, reset and CSV imports share one transaction. Stop the application and other database clients before using `RESET_SEED=true`. It removes all existing assignment data, including persisted forecasts, and reseeds the three tables.

Download, aggregation and cleanup tools are kept in the ignored `data-preparation/` folder. Original inputs and reports are retained locally and excluded from Git. Mandatory null/missing fields are removed offline; optional weather measurements remain nullable. Valid zero trips and zero rainfall are retained.

The original JSON was converted once during preparation. All 697,440 observation records passed a field-by-field CSV round-trip hash check. The current CSV data is about 44 MB, fits in regular Git, and needs no LFS download or conversion at application startup. The original JSON is kept in a local ignored archive rather than shipped alongside the CSV files.

The recommendation service builds read-only cached results from database-derived historical statistics and persisted forecasts. API summary preparation does not change the seed, database observations or cached hourly evidence.
