package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
	log "github.com/sirupsen/logrus"
)

// DB wraps a PostgreSQL connection pool
type DB struct {
	*sql.DB
}

// NewPostgresDB creates a new PostgreSQL connection pool
func NewPostgresDB(dsn string, maxConns, minConns int) (*DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(minConns)
	db.SetConnMaxLifetime(time.Hour)
	db.SetConnMaxIdleTime(10 * time.Minute)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Info("PostgreSQL connection established")

	return &DB{db}, nil
}

// HealthCheck checks if the database is healthy
func (db *DB) HealthCheck(ctx context.Context) error {
	return db.PingContext(ctx)
}

// Close closes the database connection pool
func (db *DB) Close() error {
	log.Info("Closing PostgreSQL connection")
	return db.DB.Close()
}

// ExecContext executes a query without returning rows
func (db *DB) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	start := time.Now()
	result, err := db.DB.ExecContext(ctx, query, args...)
	duration := time.Since(start)

	log.WithFields(log.Fields{
		"query":    query,
		"duration": duration,
		"error":    err,
	}).Debug("SQL exec")

	return result, err
}

// QueryContext executes a query that returns rows
func (db *DB) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	start := time.Now()
	rows, err := db.DB.QueryContext(ctx, query, args...)
	duration := time.Since(start)

	log.WithFields(log.Fields{
		"query":    query,
		"duration": duration,
		"error":    err,
	}).Debug("SQL query")

	return rows, err
}

// QueryRowContext executes a query that returns a single row
func (db *DB) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	start := time.Now()
	row := db.DB.QueryRowContext(ctx, query, args...)
	duration := time.Since(start)

	log.WithFields(log.Fields{
		"query":    query,
		"duration": duration,
	}).Debug("SQL query row")

	return row
}

// BeginTx starts a new transaction
func (db *DB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return db.DB.BeginTx(ctx, opts)
}

// RunMigrations runs database migrations
func RunMigrations(db *sql.DB, migrationsPath string) error {
	log.WithField("path", migrationsPath).Info("Running database migrations")

	// For now, migrations are in the migrations directory
	// In production, use a migration tool like golang-migrate or goose
	// For this implementation, migrations are applied via migration files

	log.Info("Database migrations completed (migrations handled externally)")
	return nil
}