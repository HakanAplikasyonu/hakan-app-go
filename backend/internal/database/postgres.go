package database

import (
	"context"
	"fmt"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/config"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RuntimeRole is the role every application connection runs as when
// DBConfig.RLSRuntime is on (migration 000074).
const RuntimeRole = "hakan_rls_runtime"

// NewPostgresPool opens a pgx connection pool and verifies connectivity.
func NewPostgresPool(ctx context.Context, cfg config.DBConfig) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("database: parse config: %w", err)
	}

	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = 1
	poolCfg.MaxConnLifetime = time.Hour
	poolCfg.HealthCheckPeriod = 30 * time.Second
	if cfg.RLSRuntime {
		// Every connection runs as hakan_rls_runtime (issue #166): row level
		// security applies to all queries. rls.WithScope and rls.System
		// switch roles per transaction (SET LOCAL ROLE) and fall back here.
		poolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
			_, err := conn.Exec(ctx, "SET ROLE "+RuntimeRole)
			return err
		}
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("database: connect: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database: ping: %w", err)
	}

	return pool, nil
}

// Ping checks database availability.
func Ping(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return fmt.Errorf("database: pool is nil")
	}
	return pool.Ping(ctx)
}

// NewQueries returns sqlc Queries backed by the given pool.
func NewQueries(pool *pgxpool.Pool) *db.Queries {
	// Statements join the context's transaction when there is one (issue
	// #166): code inside rls.WithScope / rls.System stays in that scope.
	return db.New(NewAmbient(pool))
}
