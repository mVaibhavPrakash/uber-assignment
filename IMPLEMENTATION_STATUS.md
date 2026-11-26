# Assignment status

The approved backend and frontend changes are complete. The dashboard shows one primary decision, impact comparisons, hourly estimates, freshness and concise operational warnings. Sample counts, historical-statistics tables and calculation details have been removed from the frontend. Inspecting another zone preserves the main recommendation; a new decision resets selection.

Trip quantities serialize as whole integers in backend JSON, including hourly and area estimates, historical trip quantities and recommendation-level totals/thresholds. Null estimates remain null. Calculations and ranking retain full precision; weather and percentages retain decimals. Coverage includes a subcase in the existing recommendation test for fractional input, integer serialization, negative rounding, missing values and unchanged scoring.

The algorithm, SQL aggregation, schema, thresholds and real dataset were preserved during the UI work. Preparation remains in the separate ignored data-preparation module. Initialization reads data/public/seed.json through internal/seeding.

Validation covers three focused backend tests, Go vet/build, the TypeScript production build and desktop/mobile checks for normal, intervention, insufficient-evidence, stale and unavailable states. Browser checks also cover selection, expiry suppression and JavaScript exceptions. Explicitly fictional development scenarios use the real scorer without production database writes.

README documents setup, SQL/scoring, limitations, AI assistance and screenshots. docs/normal-operations.png shows the real API; docs/intervention-required.png is visibly labeled illustrative development data.

Docker verification completed on November 26, 2026: the documented setup built successfully and committed 20 zones and 697,440 observations. Compose built and started PostgreSQL, API and frontend. Frontend HTTP 200, the Nginx API proxy, area insights and ready health were verified. The real recommendation used fresh weather, assessed 20/20 zones and returned no_intervention / no_qualifying_rain. Services remain available at localhost:3000 and localhost:8080.

Docker's installation directory was added to the verification process PATH so the credential helper could be found. No permanent machine configuration was changed. The backend image includes only the prepared seed JSON. The optional PowerShell verification script parses successfully; execution remains subject to the machine's execution policy. Its HTTP checks were performed directly during Docker verification.

Remaining delivery work: publish/deliver the accessible repository. This workspace has no Git repository or remote, so publication remains outstanding.
