# NYC Operations Control Room

A weather-informed mobility dashboard for 20 selected NYC taxi zones. It shows one action to review driver availability, or explains why no intervention is recommended. Completed trips measure activity, not unmet demand or driver supply.

Recommendations rank zones with a known rain impact for every hour in the planning window. The UI shows assessed/total coverage and warns that excluded zones may have higher impact. Partial coverage with no qualifying action produces an insufficient-evidence decision.

With a usable, snow-free forecast, rain below the configured threshold (default 0.5 mm/hour) contributes zero modeled extra trips. Typical and expected trip totals require dry historical evidence and may be unavailable. At or above the threshold, the engine requires sufficient dry and rainy samples. If every zone stays below the threshold, the dashboard recommends no rain-related action.

Historical samples are grouped by zone, exact ISO day of week, and New York local hour across all seasons. A Thursday at 1 PM uses historical Thursdays at 1 PM in the same zone. The startup log reports 3,360 groups (20 zones × 7 days × 24 hours) drawn from 697,440 hourly observations.

## Follow one recommendation

1. At startup, PostgreSQL calculates the dry and rainy trip averages and sample counts for each group. [Historical query](internal/repository/postgres/historical.sql)
2. On a scheduler tick, `ComputeAndPublish` reads the cached forecast and matches each upcoming hour to its historical group. [Recommendation manager](internal/recommendation/service.go)
3. `estimateHourTrips` decides whether the hour has zero, estimated, or unknown rain impact. [Hourly rules](internal/recommendation/hour.go)
4. The engine adds up each zone’s hourly estimates, checks coverage, and selects the decision. The API serves that published result to the dashboard. [Zone ranking and decision](internal/recommendation/engine.go)

## Run

Copy `.env.example` to `.env` before starting. Docker's API and setup containers read `DATABASE_URL` from this file. Keep its host and port at `db:5432` for the bundled PostgreSQL service. If you change `POSTGRES_PASSWORD`, update the URL-encoded password in `DATABASE_URL` to match. Git ignores `.env`.

From a Git clone of the repository with Docker Compose available. Prepared CSV files are included in regular Git; Git LFS and historical downloads are not required for setup:

```sh
docker compose run --rm setup
docker compose up -d --build api frontend
```

Open http://localhost:3000. The API is at http://localhost:8080/api/recommendations. Setup loads empty assignment tables and upgrades an older database by removing its derived season column. Running it again verifies that zones and history match the bundled CSVs, prints `Database already initialized; skipping seed.`, and exits successfully. Existing weather forecasts are preserved. Partial tables or different seed data still cause an error; normal startup never resets data.

To intentionally reload the sample dataset, stop the API and other database clients, then run `docker compose run --rm -e RESET_SEED=true setup`. **Reset recreates the three assignment tables in `mobility_control_room` and erases their contents.** It does not drop the database or force clients off it.

After startup, run `powershell -File scripts/verify-reviewer-setup.ps1` to check Docker, Compose configuration, the served frontend and the API through Nginx. The check never resets data. It reports a weather-related HTTP 503 for review rather than treating it as a normal recommendation. The API image contains neither seed data nor a seed executable. The separate setup service uses PostgreSQL's `psql` client and reads CSV files through read-only mounts.

For local development, use Go 1.25+, PostgreSQL 17+, its `psql` client, and Node.js 22. Set `DATABASE_URL` in your terminal using the format in [.env.example](.env.example) and the host and port of your local PostgreSQL installation. The dedicated `mobility_control_room` database must already exist. Go reads its configuration from the process environment.

```sh
psql "$DATABASE_URL" -X -f database/seed.psql
go run ./cmd/server
```

The example above uses Git Bash. In PowerShell, use `psql $env:DATABASE_URL -X -f database/seed.psql`. The same command works for first-time setup and an already initialized database. For an intentional reset, add `-v reset=true` before `-f`; this requires other clients to be stopped. Both imports and any schema changes are committed together, and import failures roll them back. In another terminal:

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

Use the setup command in the Run section before starting the local backend if the database has not been initialized. Repeating it is safe: matching zones and history are verified and left unchanged.

In PowerShell, set the terminal's connection URL to use `127.0.0.1:15432`. This lets terminal Go reach PostgreSQL through the published port, while Docker setup and the containerized API keep using the URL in `.env`.

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

Follow the main flow from [server startup](cmd/server/main.go) to the [scheduler](internal/scheduler/scheduler.go), which fetches weather when due and then starts scoring. The [recommendation manager](internal/recommendation/service.go) joins cached forecasts with historical trip statistics, calls the pure scorer and publishes a complete result. The [HTTP handlers](internal/api/api.go) serve that result and its hourly evidence after checking its forecast version, planning window and expiry.

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
| Three focused automated tests | [recommendations](tests/recommendation_test.go), [SQL evidence](tests/historical_test.go), [external API failure](tests/weather_test.go) |
| Working product demonstration | Screenshots below; the two Docker commands above run the application locally. |

Reviewers can run the application locally using the Docker commands above.

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

The recommendation test checks that a zone with 450 modeled extra trips can trigger an action when another zone has too few rainy observations. The SQL test checks how those historical averages are built. The weather test checks how long a saved forecast remains usable during a provider outage.
