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

	"mobilitycontrolroom/internal/api"
	"mobilitycontrolroom/internal/config"
	"mobilitycontrolroom/internal/database"
	"mobilitycontrolroom/internal/history"

	"mobilitycontrolroom/internal/recommendation"
	"mobilitycontrolroom/internal/repository"
	"mobilitycontrolroom/internal/repository/postgres"
	"mobilitycontrolroom/internal/scheduler"
	"mobilitycontrolroom/internal/weather"
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

	pool, err := database.Open(ctx, cfg.DatabaseURL, cfg.DBTimeout)
	if err != nil {
		return err
	}

	defer pool.Close()
	forecasts, recommendations, err := initializeServices(ctx, postgres.New(pool), cfg)
	if err != nil {
		return err
	}
	jobs := scheduler.New(forecasts, recommendations, cfg, time.Now)
	workerCtx, cancelWorker := context.WithCancel(ctx)
	var workers sync.WaitGroup
	defer func() { cancelWorker(); workers.Wait() }()
	workers.Add(1)
	go func() { defer workers.Done(); jobs.Run(workerCtx) }()
	return serveHTTP(ctx, cfg, api.Handler(recommendations, time.Now))
}

// initializeServices loads database evidence and connects weather and scoring.
func initializeServices(ctx context.Context, adapter *postgres.Repository, cfg config.Config) (*weather.Service, *recommendation.Manager, error) {

	historicalCtx, cancelHistorical := context.WithTimeout(ctx, cfg.HistoricalQueryTimeout)
	evidence, err := history.Load(historicalCtx, adapter, adapter, repository.HistoricalQuery{MinRainfallMM: cfg.MinRainfallMM}, cfg.Location)
	cancelHistorical()
	if err != nil {
		return nil, nil, fmt.Errorf("initialize historical evidence: %w", err)
	}
	coverage := evidence.Coverage(int64(cfg.MinHistoricalSamples))
	slog.Info("historical evidence loaded", "areas", len(evidence.Areas()), "groups", coverage.Groups, "dry_supported_groups", coverage.DrySupportedGroups, "rain_supported_groups", coverage.RainSupportedGroups)
	provider, err := weather.NewClient(cfg, &http.Client{})
	if err != nil {
		return nil, nil, err
	}
	forecasts := weather.NewService(adapter, adapter, provider, cfg, time.Now)
	loadCtx, cancelLoad := context.WithTimeout(ctx, cfg.DBTimeout)
	err = forecasts.RestoreSavedForecast(loadCtx)
	cancelLoad()
	if err != nil {
		return nil, nil, fmt.Errorf("restore weather: %w", err)
	}
	recommendations, err := recommendation.NewManager(evidence, forecasts, cfg)
	if err != nil {
		return nil, nil, err
	}
	return forecasts, recommendations, nil
}

// serveHTTP runs the HTTP server and waits for it to stop or for shutdown.
func serveHTTP(ctx context.Context, cfg config.Config, handler http.Handler) error {

	srv := &http.Server{Addr: cfg.ListenAddr, Handler: handler, ReadHeaderTimeout: cfg.HTTPAttemptTimeout, ReadTimeout: cfg.HTTPAttemptTimeout, WriteTimeout: cfg.ExternalRequestTimeout, IdleTimeout: cfg.ShutdownTimeout}
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
			return err
		}
		return nil
	}
}
