package seeding

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// LockSetup serializes setup commands on a dedicated admin session. Reset is
// explicitly restricted to the disposable assignment database and never forces
// other clients off it. Keep this connection open through the entire seed.
func LockSetup(ctx context.Context, dsn string, reset bool) (*pgx.Conn, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse setup connection: %w", err)
	}
	if cfg.Database != "mobility_control_room" {
		return nil, fmt.Errorf("setup only supports the dedicated mobility_control_room database")
	}
	cfg.Database = "postgres"
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect setup admin: %w", err)
	}
	cleanup := func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = conn.Close(closeCtx)
	}
	if err = lockSetupSession(ctx, conn); err != nil {
		cleanup()
		return nil, err
	}
	if reset {
		if err := resetDisposableDatabase(ctx, conn); err != nil {
			cleanup()
			return nil, err
		}
	}
	return conn, nil
}

// Prevents two setup commands from resetting or seeding at once.
// Closing the admin connection releases this lock.
func lockSetupSession(ctx context.Context, conn *pgx.Conn) error {
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock(73192026)"); err != nil {
		return fmt.Errorf("lock setup: %w", err)
	}
	return nil
}

// resetDisposableDatabase drops and recreates only the dedicated assignment database.
// The caller checks its name and holds the setup lock before calling this.
func resetDisposableDatabase(ctx context.Context, conn *pgx.Conn) error {
	if _, err := conn.Exec(ctx, "DROP DATABASE IF EXISTS mobility_control_room"); err != nil {

		return fmt.Errorf("reset dedicated database (stop the app first): %w", err)
	}
	if _, err := conn.Exec(ctx, "CREATE DATABASE mobility_control_room"); err != nil {

		return fmt.Errorf("create dedicated database: %w", err)
	}
	return nil
}
