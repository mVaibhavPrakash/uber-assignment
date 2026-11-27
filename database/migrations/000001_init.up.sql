CREATE TABLE community_areas (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL
);

CREATE TABLE historical_observations (
    area_id INTEGER NOT NULL REFERENCES community_areas(id),
    hour_start TIMESTAMPTZ NOT NULL,
    local_date DATE NOT NULL,
    day_of_week SMALLINT NOT NULL,
    hour_of_day SMALLINT NOT NULL,
    trip_count INTEGER NOT NULL,
    rain_mm DOUBLE PRECISION NOT NULL,
    snowfall_cm DOUBLE PRECISION NOT NULL,
    temperature_c REAL,
    apparent_temperature_c REAL,
    precipitation_mm REAL,
    wind_speed_kmh REAL,
    PRIMARY KEY (area_id, hour_start)
);

CREATE TABLE weather_forecasts (
    area_id INTEGER NOT NULL REFERENCES community_areas(id),
    hour_start TIMESTAMPTZ NOT NULL,
    forecast_version UUID NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL,
    rain_mm DOUBLE PRECISION NOT NULL,
    snowfall_cm DOUBLE PRECISION NOT NULL,
    precipitation_probability SMALLINT NOT NULL,
    PRIMARY KEY (area_id, hour_start)
);

CREATE INDEX forecast_time_area_idx ON weather_forecasts(hour_start, area_id);
