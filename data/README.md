# Application seed

The prepared dataset consists of two CSV files:

| File | Contents |
|---|---|
| `public/areas.csv` | 20 selected NYC zones with names and representative coordinates. |
| `public/historical_observations.csv` | 697,440 matched hourly HVFHV trip/weather observations for 2022–2025. |

Both files include headers matching the database columns. Optional missing measurements use `\N`, which setup explicitly maps to SQL NULL. Valid zero trips and zero rainfall remain numeric zero. Each observation keeps its UTC timestamp, New York local date, ISO weekday and local hour. The 697,440 observations produce 3,360 groups in the startup cache.

Run `docker compose run --rm setup` from the repository root. The setup service runs [seed.psql](../database/seed.psql), creates the current schema or drops the obsolete season field on an older installation, and stages the CSVs in temporary tables using PostgreSQL `\copy`. Empty assignment tables are populated from those rows. On repeat runs, both stored datasets must match every staged field, including nulls and duplicate counts; matching data produces a successful skip and preserves weather forecasts. Partial tables or different seed data cause an error. Schema changes and imports share one transaction. `RESET_SEED=true` explicitly recreates the three assignment tables and requires other clients to be stopped. It never drops the database.

Download, aggregation and cleanup tools are kept in the ignored `data-preparation/` folder. Original inputs and reports are retained locally and excluded from Git. Mandatory null/missing fields are removed offline; optional weather measurements remain nullable. Valid zero trips and zero rainfall are retained.

The original JSON was converted once during preparation. All 697,440 observation records passed a field-by-field CSV round-trip hash check. The current CSV data is about 44 MB, fits in regular Git, and needs no LFS download or conversion at application startup. The original JSON is kept in a local ignored archive rather than shipped alongside the CSV files.

The recommendation service builds read-only cached results from database-derived historical statistics and persisted forecasts. API summary preparation does not change the seed, database observations or cached hourly evidence. The cache refactor leaves data cleanup and seeding unchanged.
