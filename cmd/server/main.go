package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"mobilitycontrolroom/internal/config"
	"mobilitycontrolroom/internal/infra/openmeteo"
	"mobilitycontrolroom/internal/repository/postgres"
	"mobilitycontrolroom/internal/scheduler"
	"mobilitycontrolroom/internal/transport/httpApi"
	"mobilitycontrolroom/internal/usecase/history"
	"mobilitycontrolroom/internal/usecase/recommendation"
	"mobilitycontrolroom/internal/usecase/weather"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	check := flag.Bool("check-config", false, "validate configuration and exit without connecting")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	if *check {
		fmt.Println("configuration valid")
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.Open(ctx, cfg.DatabaseURL, cfg.DBTimeout)
	if err != nil {
		return err
	}

	defer pool.Close()
	forecasts, recommendations, err := initializeServices(ctx, postgres.New(pool), cfg)
	if err != nil {
		return err
	}
	jobs := scheduler.New(forecasts, recommendations, scheduler.Options{
		CheckInterval: cfg.SchedulerCheck, WeatherInterval: cfg.WeatherRefresh,
		RecommendationInterval: cfg.RecommendationRefresh, WeatherTimeout: cfg.WeatherJobTimeout,
		RecommendationTimeout: cfg.RecommendationJobTimeout,
	}, time.Now)
	workerCtx, cancelWorker := context.WithCancel(ctx)
	var workers sync.WaitGroup
	workers.Go(func() { jobs.Run(workerCtx) })
	defer func() {
		cancelWorker()
		workers.Wait()
	}()
	return serveHTTP(ctx, cfg, httpApi.Handler(recommendations, time.Now))
}

// initializeServices loads database evidence and connects weather and scoring.
func initializeServices(ctx context.Context, adapter *postgres.Repository, cfg config.Config) (*weather.Service, *recommendation.Manager, error) {
	historicalCtx, cancelHistorical := context.WithTimeout(ctx, cfg.HistoricalQueryTimeout)
	evidence, err := history.Load(historicalCtx, adapter, adapter, cfg.Scoring.RainThreshold, cfg.Scoring.Location)
	cancelHistorical()
	if err != nil {
		return nil, nil, fmt.Errorf("initialize historical evidence: %w", err)
	}
	coverage := evidence.Coverage(cfg.Scoring.MinSamples)
	slog.Info("historical evidence loaded", "areas", len(evidence.Areas()), "groups", coverage.Groups, "dry_supported_groups", coverage.DrySupportedGroups, "rain_supported_groups", coverage.RainSupportedGroups)
	provider, err := openmeteo.NewClient(cfg.ForecastURL, &http.Client{}, openmeteo.Options{
		ForecastHours: cfg.ForecastHours, MaxAttempts: cfg.MaxAttempts, RetryDelay: cfg.RetryDelay,
		AttemptTimeout: cfg.HTTPAttemptTimeout, RequestTimeout: cfg.ExternalRequestTimeout,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("initialize weather provider: %w", err)
	}
	forecasts := weather.NewService(adapter, adapter, provider, weather.Options{
		Window: cfg.Scoring.Window, RefreshInterval: cfg.WeatherRefresh,
		MaxStaleness: cfg.WeatherMaxStaleness, RecoveryTimeout: cfg.DBTimeout,
	}, time.Now)
	loadCtx, cancelLoad := context.WithTimeout(ctx, cfg.DBTimeout)
	err = forecasts.RestoreSavedForecast(loadCtx)
	cancelLoad()
	if err != nil {
		return nil, nil, fmt.Errorf("restore weather: %w", err)
	}
	recommendations, err := recommendation.NewManager(evidence, forecasts, cfg.Scoring, cfg.WeatherMaxStaleness)
	if err != nil {
		return nil, nil, fmt.Errorf("initialize recommendation manager: %w", err)
	}
	return forecasts, recommendations, nil
}

// serveHTTP runs the HTTP server and waits for it to stop or for shutdown.
func serveHTTP(ctx context.Context, cfg config.Config, handler http.Handler) error {
	srv := &http.Server{
		Addr: cfg.ListenAddr, Handler: handler,
		ReadHeaderTimeout: cfg.HTTPAttemptTimeout,
		ReadTimeout:       cfg.HTTPAttemptTimeout,
		WriteTimeout:      cfg.ExternalRequestTimeout,
		IdleTimeout:       cfg.ShutdownTimeout,
	}
	done := make(chan error, 1)
	go func() { done <- srv.ListenAndServe() }()
	slog.Info("server listening", "address", cfg.ListenAddr)

	select {
	case err := <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
		return nil
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			closeErr := srv.Close()
			return errors.Join(fmt.Errorf("shutdown HTTP: %w", err), closeErr)
		}
		if err := <-done; !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
		return nil
	}
}
