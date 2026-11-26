# Application seed

`public/seed.json` is the only data file needed by setup. It contains the 20 selected NYC zones and 697,440 matched hourly HVFHV trip/weather observations for 2022–2025. The seeder reads prepared records and copies them into PostgreSQL in one transaction.

Download, aggregation and cleanup tools are kept in the ignored `data-preparation/` folder. Original inputs and reports are retained locally and excluded from Git. Mandatory null/missing fields are removed offline; optional weather measurements remain nullable. Valid zero trips and zero rainfall are retained.

The cleanup script writes a separate output before replacing the application seed. Its last run retained every observation. The original seed remains in the ignored preparation archive.
