# NYC Operations Control Room

A weather-informed mobility dashboard for 20 selected NYC taxi zones. It shows one action to review driver availability, or explains why no intervention is recommended. Completed trips measure activity, not unmet demand or driver supply.

## Run

From the repository root with Docker Compose:

```sh
docker compose run --rm --build setup
docker compose up --build api frontend
```

Open http://localhost:3000. The API is at http://localhost:8080/api/recommendations. **Setup resets the dedicated assignment database. Stop the application first and never use setup against a database whose contents you need to preserve.** Normal startup does not reset data. The Docker setup/build/start workflow was verified on November 26, 2026.

After startup, run `powershell -File scripts/verify-reviewer-setup.ps1` to check Docker, Compose configuration, the served frontend and the API through Nginx. The check never resets data. It reports a weather-related HTTP 503 for review rather than treating it as a normal recommendation. The backend image includes only the prepared seed JSON, not original download artifacts.

For local development, use Go 1.25+, PostgreSQL and Node.js 22. Configure environment variables using [.env.example](.env.example). Go reads environment variables directly; Compose loads an optional `.env` file.

```sh
go run ./cmd/seed -data data/public
go run ./cmd/server
```

Seeding requires an empty database; the explicit `-reset` option recreates the dedicated assignment database. In another terminal:

```sh
cd frontend
npm ci
npm run dev
```

The frontend proxies requests to port 8080. It refreshes every 60 seconds and suppresses expired recommendations and results from failed refreshes.

## Architecture

React/TypeScript is served by Nginx, which proxies API requests to Go. The Go application loads historical SQL statistics from PostgreSQL, periodically fetches and persists Open-Meteo forecasts, and computes one current recommendation. A one-shot setup container loads the prepared seed; no historical downloads are required at startup.

See [architecture and package responsibilities](DESIGN_AND_SCALABILITY.md#architecture), [data and scoring](DESIGN_AND_SCALABILITY.md#data-and-decisions), and [database design and SQL](DESIGN_AND_SCALABILITY.md#database-design-and-important-sql) for the detailed flow, endpoints and implementation.

## Documentation and deliverables

| Deliverable | Where to find it |
|---|---|
| Architecture, endpoints, schema explanation and important SQL | [DESIGN_AND_SCALABILITY.md](DESIGN_AND_SCALABILITY.md) |
| Assumptions, alternatives, trade-offs and known limitations | [TRADEOFFS.md](TRADEOFFS.md) |
| AI tools, accepted/rejected suggestions, validation, risks and technical challenges | [MY_REASONING.md](MY_REASONING.md#ai-use-disclosure) |
| Prepared public dataset and offline preparation boundary | [data/README.md](data/README.md) |
| Executable schema and SQL | [migrations](database/migrations/), [historical aggregation](internal/repository/postgres/historical.sql) |
| Dependencies and configuration | [go.mod](go.mod), [go.sum](go.sum), [frontend/package.json](frontend/package.json), [lockfile](frontend/package-lock.json), [.env.example](.env.example), [compose.yaml](compose.yaml) |
| Three focused automated tests | [recommendations](tests/recommendation_test.go), [SQL evidence](tests/historical_test.go), [external API failure](tests/weather_test.go) |
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
