// Package db provides SQLite database access for tsuite.
// Schema is compatible with the Python version's ~/.tsuite/results.db.
package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

var (
	db     *sql.DB
	dbPath string
	once   sync.Once
)

// Schema matches the Python version for compatibility
const schema = `
-- Schema version tracking
CREATE TABLE IF NOT EXISTS schema_version (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    version INTEGER NOT NULL DEFAULT 1
);
INSERT OR IGNORE INTO schema_version (id, version) VALUES (1, 3);

-- Registered test suites (for dashboard settings)
CREATE TABLE IF NOT EXISTS suites (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    folder_path TEXT UNIQUE NOT NULL,
    suite_name TEXT NOT NULL,
    mode TEXT DEFAULT 'docker' CHECK(mode IN ('standalone', 'docker')),
    config_json TEXT,
    test_count INTEGER DEFAULT 0,
    last_synced_at TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);

-- Each test run session
CREATE TABLE IF NOT EXISTS runs (
    run_id TEXT PRIMARY KEY,
    suite_id INTEGER REFERENCES suites(id),
    suite_name TEXT,
    started_at TEXT NOT NULL,
    finished_at TEXT,
    status TEXT DEFAULT 'pending',
    cli_version TEXT,
    sdk_python_version TEXT,
    sdk_typescript_version TEXT,
    docker_image TEXT,
    total_tests INTEGER DEFAULT 0,
    pending_count INTEGER DEFAULT 0,
    running_count INTEGER DEFAULT 0,
    passed INTEGER DEFAULT 0,
    failed INTEGER DEFAULT 0,
    skipped INTEGER DEFAULT 0,
    duration_ms INTEGER,
    filters TEXT,
    mode TEXT DEFAULT 'docker' CHECK(mode IN ('standalone', 'docker')),
    cancel_requested INTEGER DEFAULT 0
);

-- Individual test case results (also used for live tracking)
CREATE TABLE IF NOT EXISTS test_results (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id TEXT NOT NULL REFERENCES runs(run_id) ON DELETE CASCADE,
    test_id TEXT NOT NULL,
    use_case TEXT NOT NULL,
    test_case TEXT NOT NULL,
    name TEXT,
    tags TEXT,
    status TEXT DEFAULT 'pending',
    started_at TEXT,
    finished_at TEXT,
    duration_ms INTEGER,
    error_message TEXT,
    error_step INTEGER,
    skip_reason TEXT,
    steps_json TEXT,
    steps_passed INTEGER DEFAULT 0,
    steps_failed INTEGER DEFAULT 0,
    UNIQUE(run_id, test_id)
);

-- Step-level execution tracking
CREATE TABLE IF NOT EXISTS step_results (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    test_result_id INTEGER NOT NULL REFERENCES test_results(id),
    step_index INTEGER NOT NULL,
    phase TEXT NOT NULL,
    handler TEXT NOT NULL,
    description TEXT,
    status TEXT DEFAULT 'pending',
    started_at TEXT,
    finished_at TEXT,
    duration_ms INTEGER,
    exit_code INTEGER,
    stdout TEXT,
    stderr TEXT,
    error_message TEXT,
    UNIQUE(test_result_id, phase, step_index)
);

-- Assertion results
CREATE TABLE IF NOT EXISTS assertion_results (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    test_result_id INTEGER NOT NULL REFERENCES test_results(id),
    assertion_index INTEGER NOT NULL,
    expression TEXT NOT NULL,
    message TEXT,
    passed INTEGER NOT NULL,
    actual_value TEXT,
    expected_value TEXT,
    UNIQUE(test_result_id, assertion_index)
);

-- Captured values during execution
CREATE TABLE IF NOT EXISTS captured_values (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    test_result_id INTEGER NOT NULL REFERENCES test_results(id),
    key TEXT NOT NULL,
    value TEXT,
    captured_at TEXT,
    UNIQUE(test_result_id, key)
);

-- Indexes for common queries
CREATE INDEX IF NOT EXISTS idx_test_results_run ON test_results(run_id);
CREATE INDEX IF NOT EXISTS idx_test_results_status ON test_results(status);
CREATE INDEX IF NOT EXISTS idx_step_results_test ON step_results(test_result_id);
CREATE INDEX IF NOT EXISTS idx_runs_status ON runs(status);
CREATE INDEX IF NOT EXISTS idx_runs_started ON runs(started_at DESC);
CREATE INDEX IF NOT EXISTS idx_suites_folder_path ON suites(folder_path);
`

// DefaultDBPath returns the default database path (~/.tsuite/results.db)
func DefaultDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".tsuite/results.db"
	}
	return filepath.Join(home, ".tsuite", "results.db")
}

// SetDBPath sets a custom database path (must be called before GetDB)
func SetDBPath(path string) {
	dbPath = path
}

// GetDB returns the singleton database connection
func GetDB() (*sql.DB, error) {
	var initErr error

	once.Do(func() {
		path := dbPath
		if path == "" {
			path = DefaultDBPath()
		}

		// Ensure directory exists
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			initErr = err
			return
		}

		// Open database with WAL mode and busy timeout
		// WAL mode allows concurrent reads with one writer
		// busy_timeout waits up to 30s for locks instead of failing immediately
		var err error
		db, err = sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(30000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
		if err != nil {
			initErr = err
			return
		}

		// Configure connection pool
		// Allow multiple connections for concurrent reads (WAL mode supports this)
		// Writes are serialized by SQLite, busy_timeout handles contention
		db.SetMaxOpenConns(10)       // Allow concurrent readers
		db.SetMaxIdleConns(5)        // Keep some connections ready
		db.SetConnMaxLifetime(0)     // Don't close idle connections

		// Test connection
		if err = db.Ping(); err != nil {
			initErr = err
			return
		}

		// Initialize schema
		if err = initSchema(db); err != nil {
			initErr = err
			return
		}
	})

	return db, initErr
}

// initSchema creates tables if they don't exist
func initSchema(db *sql.DB) error {
	_, err := db.Exec(schema)
	return err
}

// Close closes the database connection
func Close() error {
	if db != nil {
		return db.Close()
	}
	return nil
}
