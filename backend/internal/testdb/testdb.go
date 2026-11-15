// Package testdb gives tests a real, freshly migrated PostgreSQL database.
//
// Set TEST_DATABASE_URL to a server the tests may create databases on (for
// example postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable).
// Each Open call creates a new database, applies every migrations/*.up.sql in
// order and drops the database when the test ends.
//
// Without TEST_DATABASE_URL the test is skipped, unless TESTDB_REQUIRED=1
// (CI sets it), in which case it fails: isolation tests must not silently
// stop running.
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EnvURL names the admin connection string.
const EnvURL = "TEST_DATABASE_URL"

// DB is one throwaway database.
type DB struct {
	// URL connects to the new database as the admin user.
	URL  string
	Pool *pgxpool.Pool
	Name string
}

// Open creates and migrates a fresh database for t.
func Open(t testing.TB) *DB {
	t.Helper()
	admin := strings.TrimSpace(os.Getenv(EnvURL))
	if admin == "" {
		if os.Getenv("TESTDB_REQUIRED") == "1" {
			t.Fatalf("%s is not set but TESTDB_REQUIRED=1", EnvURL)
		}
		t.Skipf("%s not set: skipping database test", EnvURL)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	adminConn, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Fatalf("testdb: connect admin: %v", err)
	}
	defer func() { _ = adminConn.Close(context.Background()) }()

	name := "hakan_test_" + randomSuffix()
	if _, err := adminConn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("testdb: create database: %v", err)
	}
	t.Cleanup(func() {
		c, err := pgx.Connect(context.Background(), admin)
		if err != nil {
			return
		}
		defer func() { _ = c.Close(context.Background()) }()
		_, _ = c.Exec(context.Background(), "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
	})

	dbURL, err := withDatabase(admin, name)
	if err != nil {
		t.Fatalf("testdb: %v", err)
	}
	if err := migrate(ctx, dbURL); err != nil {
		t.Fatalf("testdb: migrate: %v", err)
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("testdb: pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return &DB{URL: dbURL, Pool: pool, Name: name}
}

// MigrationsDir is backend/migrations, found from this source file.
func MigrationsDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "migrations")
}

func migrate(ctx context.Context, dbURL string) error {
	files, err := filepath.Glob(filepath.Join(MigrationsDir(), "*.up.sql"))
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no migrations in %s", MigrationsDir())
	}
	sort.Strings(files)
	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(context.Background()) }()
	seen := map[string]string{}
	for _, f := range files {
		version := strings.SplitN(filepath.Base(f), "_", 2)[0]
		if prev, dup := seen[version]; dup {
			return fmt.Errorf("duplicate migration version %s: %s and %s", version, filepath.Base(prev), filepath.Base(f))
		}
		seen[version] = f
		raw, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		// Simple protocol: a migration file holds many statements.
		if _, err := conn.PgConn().Exec(ctx, string(raw)).ReadAll(); err != nil {
			return fmt.Errorf("%s: %w", filepath.Base(f), err)
		}
	}
	return nil
}

func withDatabase(raw, name string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", EnvURL, err)
	}
	u.Path = "/" + name
	return u.String(), nil
}

func randomSuffix() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
