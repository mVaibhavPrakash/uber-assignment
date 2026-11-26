# NYC Operations Control Room

A weather-informed mobility dashboard for 20 selected NYC taxi zones. It shows one action to review driver availability, or explains why no intervention is recommended. Completed trips measure activity, not unmet demand or driver supply.

Recommendations rank zones with complete evidence for the planning window. Incomplete zones do not block an otherwise eligible action: the UI shows assessed/total coverage and warns that excluded zones may have higher impact. With partial coverage, no qualifying result means no assessed zone reaches the threshold, not that all monitored zones are clear.

Rain below the configured threshold (default 0.5 mm/hour), including light rain, uses the historical below-threshold comparison and contributes zero modeled extra trips. At or above the threshold, the engine applies the rainy-versus-below-threshold comparison. The UI calls the comparison "Typical dry-weather trips" and explains the light-rain inclusion in a tooltip. Restart/rebuild the API after this change to refresh its historical statistics; reseeding is unnecessary.

## Run

Copy `.env.example` to `.env` before starting. The example includes credentials for the local assignment database and uses the Compose host `db`. If you change `POSTGRES_PASSWORD`, update the URL-encoded password in `DATABASE_URL` to match. The `.env` file is ignored by Git.

From a Git clone of the repository with Docker Compose available. Prepared CSV files are included in regular Git; Git LFS and historical downloads are not required for setup:

```sh
docker compose run --rm setup
docker compose up --build api frontend
```

Open http://localhost:3000. The API is at http://localhost:8080/api/recommendations. Setup refuses nonempty or partial assignment tables by default; normal startup never resets data. To intentionally reload the sample dataset, stop the API and other database clients, then run `docker compose run --rm -e RESET_SEED=true setup`. **Reset recreates the three assignment tables in `mobility_control_room` and erases their contents.** It does not drop the database or force clients off it.

After startup, run `powershell -File scripts/verify-reviewer-setup.ps1` to check Docker, Compose configuration, the served frontend and the API through Nginx. The check never resets data. It reports a weather-related HTTP 503 for review rather than treating it as a normal recommendation. The API image contains neither seed data nor a seed executable. The separate setup service uses PostgreSQL's `psql` client and reads CSV files through read-only mounts.

For local development, use Go 1.25+, PostgreSQL 17+, its `psql` client, and Node.js 22. Configure environment variables using [.env.example](.env.example), using your local database host instead of `db`. The dedicated `mobility_control_room` database must already exist. Go requires `DATABASE_URL` in the process environment and does not load `.env` itself. Compose reads `.env` or exported environment variables and rejects missing database settings.

```sh
psql "$DATABASE_URL" -X -f database/seed.psql
go run ./cmd/server
```

The example above uses Git Bash. In PowerShell, use `psql $env:DATABASE_URL -X -f database/seed.psql`. Run setup only for empty assignment tables. For an intentional reset, add `-v reset=true` before `-f`; this requires other clients to be stopped. Both imports and any schema changes are committed together, and import failures roll them back. In another terminal:

```sh
cd frontend
npm ci
npm run dev
```

The frontend proxies requests to port 8080. It refreshes every 60 seconds and suppresses expired recommendations and results from failed refreshes.

## Debug the backend with PostgreSQL in Docker

Keep PostgreSQL in Docker and run Go directly in your terminal or debugger. The database is exposed only on `127.0.0.1:15432`, leaving locally installed PostgreSQL on port 5432 separate. It uses the same assignment database volume and credentials from `.env`.

From the repository root, start the database and stop the containerized API to free port 8080:

```sh
docker compose up -d db
docker compose stop api
```

For an already seeded database, do not run setup again. For a first-time empty database, use the setup command in the Run section before starting the local backend. The default setup does not reset existing data.

In PowerShell, read the connection URL from `.env` and change only the host and port for this terminal:

```powershell
$databaseSetting = Get-Content .env | Where-Object { $_.StartsWith('DATABASE_URL=') }
$env:DATABASE_URL = $databaseSetting.Substring(13).Replace('@db:5432/', '@127.0.0.1:15432/')
go run ./cmd/server
```

Alternatively, in Git Bash:

```sh
export DATABASE_URL="$(sed -n 's/^DATABASE_URL=//p' .env | tr -d '\r' | sed 's/@db:5432\//@127.0.0.1:15432\//')"
go run ./cmd/server
```

The API runs at http://localhost:8080. The normal local frontend (`npm run dev`) can use it. If Delve is installed, replace `go run ./cmd/server` with `dlv debug ./cmd/server` to set breakpoints. This approach leaves `.env` configured for Docker and does not require credentials for your separately installed PostgreSQL server.

## Try a rainy recommendation in the UI

This separate local harness uses fictional inputs with the existing scoring engine and API handlers. It needs Go 1.25+ and Node.js 22; Docker, PostgreSQL, seed data and live weather are unnecessary. The sample uses API port **8081** and UI port **3001**, so the normal application on ports 8080/3000 can keep running.

From the repository root, start the sample API:

```sh
go run ./tools/rain-demo
```

In a second terminal, start the frontend in sample mode. Run `npm ci` only for the initial dependency installation (skip it if dependencies are already installed). Before reinstalling dependencies, stop local Vite dev/preview processes: Windows can lock esbuild while they run. After installation, normal and sample UIs can run together.

```sh
cd frontend
npm ci
npm run dev:sample
```

Open http://localhost:3001. This UI proxies API requests exclusively to the sample server on port 8081. The page explicitly labels the scenario as fictional sample data. Select Astoria to inspect the hourly breakdown.

| Input | First hour | Second hour |
|---|---:|---:|
| Rain | 1 mm | 0 mm |
| Precipitation probability | 90% | 90% |
| Historical dry average | 1,200 trips | 1,400 trips |
| Historical rainy average | 1,700 trips | 1,700 trips (unused for a dry hour) |
| Historical samples | 5 per class | 5 per class |

Expected: **review driver availability in Astoria**, **450 additional trips**, **2,600 baseline trips**, **3,050 expected trips**, and a **100-trip intervention threshold**. The existing engine calculates `(1700 - 1200) × 90% = 450`; the dry hour adds zero. Times follow the current planning window so the UI can display the result without it being expired.

Verify the same calculation without the UI:

```sh
go test ./tests -run TestRecommendationRanksAssessedZones -v
```

Stop the two sample processes with Ctrl+C when finished. The normal application remains available at http://localhost:3000. The harness adds no scoring rules and never reads or changes the database.

## Architecture

React/TypeScript is served by Nginx, which proxies API requests to Go. The Go application loads historical SQL statistics from PostgreSQL, periodically fetches and persists Open-Meteo forecasts, and computes one current recommendation. A one-shot setup container loads the prepared seed; no historical downloads are required at startup.

Each completed recommendation is published atomically and then treated as read-only. API handlers prepare summaries without changing cached hourly evidence; readers do not deep-copy the full recommendation. Version, window and expiry checks still run on every read.

See [architecture and package responsibilities](DESIGN_AND_SCALABILITY.md#architecture), [data and scoring](DESIGN_AND_SCALABILITY.md#data-and-decisions), and [database design and SQL](DESIGN_AND_SCALABILITY.md#database-design-and-important-sql) for the detailed flow, endpoints and implementation.

## Documentation and deliverables

| Deliverable | Where to find it |
|---|---|
| Architecture, endpoints, schema explanation and important SQL | [DESIGN_AND_SCALABILITY.md](DESIGN_AND_SCALABILITY.md) |
| Scheduler and recommendation flow with timing examples | [Scheduler and recommendation architecture](docs/SCHEDULER_AND_RECOMMENDATIONS.md) |
| Assumptions, alternatives, trade-offs and known limitations | [TRADEOFFS.md](TRADEOFFS.md) |
| AI tools, accepted/rejected suggestions, validation, risks and technical challenges | [MY_REASONING.md](MY_REASONING.md#ai-use-disclosure) |
| Prepared public dataset and offline preparation boundary | [data/README.md](data/README.md) |
| Executable schema and SQL | [migrations](database/migrations/), [historical aggregation](internal/repository/postgres/historical.sql) |
| Dependencies and configuration | [go.mod](go.mod), [go.sum](go.sum), [frontend/package.json](frontend/package.json), [lockfile](frontend/package-lock.json), [.env.example](.env.example), [compose.yaml](compose.yaml) |
| Four focused automated tests | [recommendations](tests/recommendation_test.go), [SQL evidence](tests/historical_test.go), [external API failure](tests/weather_test.go) |
| Working product demonstration | Screenshots below; the two Docker commands above run the application locally. |

Repository publication/access verification remains a submission step; no remote URL is currently configured. A hosted deployment is optional and is not supplied here.

## Screenshots

Normal operations captured against the real API and selected 20-zone dataset:

![Normal operations using the real API](docs/normal-operations.png)

Intervention example: **illustrative development scenario with fictional weather and historical inputs**, using two zones. The 450 additional trips are not a live forecast or observed result. The actual scoring function produces this example.

![Clearly labeled illustrative intervention scenario](docs/intervention-required.png)

The intervention screenshot was captured using explicitly labeled development inputs and the real scoring function. It is a saved demonstration, not a production simulation mode. There is no fixture endpoint, production demo toggle or silent replacement of live weather.

## Validation

```sh
go test ./...
go vet ./...
go build ./...
npm run build --prefix frontend
```

Set `TEST_DATABASE_URL` to a dedicated PostgreSQL test database to include SQL verification. That test creates/removes its own temporary schema and skips without the variable. Docker setup was also verified from a clean copy with no local dependencies/build artifacts and a new database volume.

The suite contains exactly four tests: a qualifying recommendation with partial coverage, light rain without intervention, historical SQL comparisons, and retaining usable forecasts during an outage while rejecting expired forecasts. There are no nested scenario matrices or cache/serialization test suites.
