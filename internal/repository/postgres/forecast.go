package postgres

import (
	"context"
	"mobilitycontrolroom/internal/appErrors"

	"fmt"
	"mobilitycontrolroom/internal/database"
	"mobilitycontrolroom/internal/domain"
	"mobilitycontrolroom/internal/repository"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var _ repository.ForecastRepository = (*Repository)(nil)

func (r *Repository) LoadLatest(ctx context.Context) (repository.ForecastSnapshot, error) {
	s := repository.ForecastSnapshot{}
	// A single SELECT sees one committed PostgreSQL snapshot even while another
	// transaction replaces the whole forecast table.
	const query = `
		SELECT area_id, hour_start, forecast_version::text, fetched_at,
		       rain_mm, snowfall_cm, precipitation_probability
		FROM weather_forecasts
		ORDER BY area_id, hour_start`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return s, fmt.Errorf("load forecast snapshot: %w", err)
	}
	defer rows.Close()
	areas := []repository.Area{}
	known := map[int]bool{}
	for rows.Next() {
		var h repository.ForecastHour
		var version string
		var fetched time.Time
		if err = rows.Scan(&h.AreaID, &h.HourStart, &version, &fetched, &h.RainMM, &h.SnowfallCM, &h.PrecipitationProbability); err != nil {
			return repository.ForecastSnapshot{}, fmt.Errorf("scan forecast row: %w", err)
		}
		if len(s.Hours) == 0 {
			s.Version = version
			s.FetchedAt = fetched.UTC()
		} else if version != s.Version || !fetched.Equal(s.FetchedAt) {
			return repository.ForecastSnapshot{}, fmt.Errorf("persisted forecast contains mixed versions or fetched times")
		}
		h.HourStart = h.HourStart.UTC()
		s.Hours = append(s.Hours, h)
		if !known[h.AreaID] {
			areas = append(areas, repository.Area{ID: h.AreaID})
			known[h.AreaID] = true
		}
	}
	if err = rows.Err(); err != nil {
		return repository.ForecastSnapshot{}, fmt.Errorf("read forecast snapshot: %w", err)
	}
	if len(s.Hours) == 0 {
		return repository.ForecastSnapshot{}, appErrors.ErrNoSnapshot
	}
	if err = domain.ValidateForecast(s, areas); err != nil {
		return repository.ForecastSnapshot{}, fmt.Errorf("invalid persisted forecast: %w", err)
	}
	return s, nil
}

func (r *Repository) ReplaceSnapshot(ctx context.Context, s repository.ForecastSnapshot) (err error) {
	tx, err := database.Begin(ctx, r.pool, "forecast replacement")
	if err != nil {
		return err
	}
	defer appErrors.Rollback(tx, "forecast", &err)
	if err = lockForecastPublication(ctx, tx); err != nil {
		return err
	}
	areas, err := listAreas(ctx, tx)
	if err != nil {
		return err
	}
	if err = domain.ValidateForecast(s, areas); err != nil {
		return err
	}
	if err = copyForecast(ctx, tx, s); err != nil {
		return err
	}
	return appErrors.CommitForecast(ctx, tx)
}

// Keeps zone data stable and allows one forecast writer at a time.
func lockForecastPublication(ctx context.Context, tx pgx.Tx) error {
	const command = `
		LOCK TABLE community_areas IN SHARE MODE;
		LOCK TABLE weather_forecasts IN EXCLUSIVE MODE`
	if _, err := tx.Exec(ctx, command); err != nil {
		return fmt.Errorf("lock forecast publication: %w", err)
	}
	return nil
}

// Replaces all forecast rows inside the caller's transaction.
func copyForecast(ctx context.Context, tx pgx.Tx, s repository.ForecastSnapshot) error {
	var err error
	if _, err = tx.Exec(ctx, `DELETE FROM weather_forecasts`); err != nil {
		return fmt.Errorf("delete old forecast: %w", err)
	}
	version := pgtype.UUID{Bytes: [16]byte(uuid.MustParse(s.Version)), Valid: true}
	columns := []string{
		"area_id", "hour_start", "forecast_version", "fetched_at",
		"rain_mm", "snowfall_cm", "precipitation_probability",
	}
	_, err = tx.CopyFrom(ctx, pgx.Identifier{"weather_forecasts"}, columns, pgx.CopyFromSlice(len(s.Hours), func(i int) ([]any, error) {
		h := s.Hours[i]
		return []any{
			h.AreaID, h.HourStart.UTC(), version, s.FetchedAt.UTC().Truncate(time.Microsecond),
			h.RainMM, h.SnowfallCM, h.PrecipitationProbability,
		}, nil
	}))
	if err != nil {
		return fmt.Errorf("copy complete forecast: %w", err)
	}
	return nil
}
