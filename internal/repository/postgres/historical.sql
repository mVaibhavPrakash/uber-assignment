WITH historical_stats AS (
    SELECT area_id, season, day_of_week, hour_of_day,
           AVG(trip_count) FILTER (WHERE rain_mm >= 0 AND rain_mm < $1) AS dry_avg,
           COUNT(*) FILTER (WHERE rain_mm >= 0 AND rain_mm < $1) AS dry_n,
           AVG(trip_count) FILTER (WHERE rain_mm >= $1) AS rain_avg,
           COUNT(*) FILTER (WHERE rain_mm >= $1) AS rain_n,
           STDDEV_SAMP(trip_count) FILTER (WHERE rain_mm >= $1) AS rain_stddev
    FROM historical_observations
    WHERE snowfall_cm = 0
    GROUP BY area_id, season, day_of_week, hour_of_day
)
SELECT area_id, season, day_of_week, hour_of_day,
       dry_avg, dry_n, rain_avg, rain_n, rain_stddev,
       rain_avg - dry_avg AS absolute_uplift,
       100.0 * (rain_avg - dry_avg) / NULLIF(dry_avg, 0) AS percentage_uplift
FROM historical_stats
