# Application seed

The prepared dataset consists of two CSV files:

| File | Contents |
|---|---|
| `public/areas.csv` | 20 selected NYC zones with names and representative coordinates. |
| `public/historical_observations.csv` | 697,440 matched hourly HVFHV trip/weather observations for 2022–2025. |

Both files include headers matching the database columns. Optional missing measurements use `\N`, which setup explicitly maps to SQL NULL. Valid zero trips and zero rainfall remain numeric zero. Hour timestamps are UTC and local dates/calendar fields retain the prepared New York interpretation.

Run `docker compose run --rm setup` from the repository root. The setup service runs [seed.psql](../database/seed.psql), applies the migration when necessary, and imports areas before observations using PostgreSQL `\copy`. Schema changes and both imports share one transaction. Default setup rejects existing data; `RESET_SEED=true` explicitly recreates the three assignment tables and requires other clients to be stopped. It never drops the database.

Download, aggregation and cleanup tools are kept in the ignored `data-preparation/` folder. Original inputs and reports are retained locally and excluded from Git. Mandatory null/missing fields are removed offline; optional weather measurements remain nullable. Valid zero trips and zero rainfall are retained.

The original JSON was converted once during preparation. All 697,440 observation records passed a field-by-field CSV round-trip hash check. The resulting CSV data is about 46 MB, fits in regular Git, and needs no LFS download or conversion at application startup. The original JSON is kept in a local ignored archive rather than shipped alongside the CSV files.

The recommendation service builds read-only cached results from database-derived historical statistics and persisted forecasts. API summary preparation does not change the seed, database observations or cached hourly evidence. The cache refactor leaves data cleanup and seeding unchanged.
