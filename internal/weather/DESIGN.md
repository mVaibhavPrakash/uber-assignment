# Live weather fetching

The client sends one GET for all seeded zones, then converts the hourly response into a forecast snapshot. The client does not write to the database.

Requests use up to three attempts by default. Temporary network errors and HTTP 408, 429 or 5xx responses can be retried. `RETRY_DELAY=1s` is the initial backoff: the first retry waits 0.5–1 second, and the second waits 1–2 seconds. Each retry doubles the backoff and randomizes the wait within its upper half. A longer provider `Retry-After` takes priority. Each attempt has a 10-second timeout; all attempts and waits share a 20-second budget. Cancellation stops requests and waits. Permanent errors and invalid forecast responses are returned to the caller.

There are no quota files or request workers in the application. Offline preparation has its own local quota tooling, outside the assignment module.

The response parser matches locations in request order, requires hourly rain/snow/probability values and checks the requested units and UTC timestamps. The API labels rain, snow and probability at the end of the preceding hour, so those values are assigned to the interval starting one hour earlier. This keeps forecast weather aligned with historical trips.

The weather service validates the complete snapshot and stores it in PostgreSQL. Failed fetches leave the previous forecast in place. The scheduler and recommendation service still enforce freshness and expiry. The retained external API failure test checks bounded retries, keeping usable previous weather, and suppressing expired weather.
