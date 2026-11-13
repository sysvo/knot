// Package db opens the SQLite database and applies embedded migrations.
package db

import (
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	_ "modernc.org/sqlite" // pure-Go driver, no cgo required
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Open opens (and creates) a SQLite database at path.
//
// WAL lets readers run while a writer commits, so the pool holds several
// connections: one request no longer queues behind another request's queries. What
// must stay serialised is the writers, and that is handled inside SQLite rather
// than by limiting the pool.
//
// _txlock=immediate is the part that makes it safe. A deferred transaction that
// reads before it writes has to upgrade its lock, and SQLite refuses that upgrade
// immediately instead of waiting when another writer got there first — the classic
// "database is locked" on a busy forum. Taking the write lock at BEGIN turns that
// into a wait bounded by busy_timeout.
func Open(path string) (*sql.DB, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}

	dsn := fmt.Sprintf(
		"file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(1)&_txlock=immediate",
		path,
	)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	conns := max(4, runtime.NumCPU())
	conn.SetMaxOpenConns(conns)
	conn.SetMaxIdleConns(conns)
	conn.SetConnMaxLifetime(30 * time.Minute)

	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("open database %s: %w", path, err)
	}
	return conn, nil
}

// OpenIsolated opens a second, single-connection handle.
//
// It exists for work that holds a long read transaction — a backup copying the
// database — so that work cannot occupy a connection the request path needs.
//
// It is not opened with query_only: VACUUM INTO writes its snapshot to a separate
// file, and SQLite rejects the statement outright on a read-only connection. What
// matters here is the dedicated connection, not the permission.
func OpenIsolated(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"file:%s?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)",
		path,
	)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open isolated database: %w", err)
	}
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)
	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("open isolated database %s: %w", path, err)
	}
	return conn, nil
}

// Migrate applies every embedded migration that has not been applied yet.
// Each migration runs inside its own transaction.
func Migrate(conn *sql.DB) error {
	if _, err := conn.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT PRIMARY KEY,
			applied_at INTEGER NOT NULL
		)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied := map[string]bool{}
	rows, err := conn.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("read schema_migrations: %w", err)
	}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		if applied[name] {
			continue
		}
		body, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		tx, err := conn.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := tx.Exec(
			`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`,
			name, time.Now().Unix(),
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
	}
	return nil
}
