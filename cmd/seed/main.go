// Command seed reads prepared JSON, optionally resets the dedicated disposable
// database, applies the initial migration, and transactionally loads observations.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"mobilitycontrolroom/internal/config"
	"mobilitycontrolroom/internal/database"

	"mobilitycontrolroom/internal/seeding"
)

func main() {
	if err := run(); err != nil {
		slog.Error("seed failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	dir := flag.String("data", "data/public", "directory containing prepared seed.json")
	reset := flag.Bool("reset", false, "DROP/CREATE only the disposable mobility_control_room database; stop app first")
	timeout := flag.Duration("timeout", 30*time.Minute, "overall setup timeout")
	flag.Parse()
	if *timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	root, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(root, *timeout)
	defer cancel()
	// Catch a missing prepared file before starting database setup.
	if _, err := os.Stat(filepath.Join(*dir, "seed.json")); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	summary, err := initializeDatabase(ctx, cfg, *dir, *reset)
	if err != nil {
		return err
	}
	slog.Info("seed committed", "areas", len(summary.Areas), "observations", summary.Matched)
	return json.NewEncoder(os.Stdout).Encode(summary)
}

// initializeDatabase holds the setup lock until seeding finishes and closes its connections.
func initializeDatabase(ctx context.Context, cfg config.Config, dir string, reset bool) (seeding.Summary, error) {
	admin, err := seeding.LockSetup(ctx, cfg.DatabaseURL, reset)
	if err != nil {
		return seeding.Summary{}, err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if e := admin.Close(closeCtx); e != nil {
			slog.Error("close setup admin", "error", e)
		}
	}()
	pool, err := database.Open(ctx, cfg.DatabaseURL, cfg.DBTimeout)
	if err != nil {
		return seeding.Summary{}, err
	}
	defer pool.Close()
	summary, err := seeding.Seed(ctx, pool, dir)
	if err != nil {
		return seeding.Summary{}, err
	}
	return summary, nil
}
