"""
SQLite database connection and schema management for test reporting.
"""

import sqlite3
import os
from pathlib import Path
from contextlib import contextmanager
from typing import Optional
import threading

# Default database location
DEFAULT_DB_PATH = Path.home() / ".tsuite" / "results.db"

# Thread-local storage for connections
_local = threading.local()

# Global database path (can be overridden)
_db_path: Optional[Path] = None


SCHEMA = """
-- Schema version tracking
CREATE TABLE IF NOT EXISTS schema_version (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    version INTEGER NOT NULL DEFAULT 1
);
INSERT OR IGNORE INTO schema_version (id, version) VALUES (1, 3);

-- Each test run session
CREATE TABLE IF NOT EXISTS runs (
    run_id TEXT PRIMARY KEY,
    suite_id INTEGER REFERENCES suites(id),
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

-- Indexes for common queries
CREATE INDEX IF NOT EXISTS idx_test_results_run ON test_results(run_id);
CREATE INDEX IF NOT EXISTS idx_test_results_status ON test_results(status);
CREATE INDEX IF NOT EXISTS idx_step_results_test ON step_results(test_result_id);
CREATE INDEX IF NOT EXISTS idx_runs_status ON runs(status);
CREATE INDEX IF NOT EXISTS idx_runs_started ON runs(started_at DESC);
CREATE INDEX IF NOT EXISTS idx_suites_folder_path ON suites(folder_path);
"""


def set_db_path(path: Optional[Path]) -> None:
    """Set the database path. Call before any database operations."""
    global _db_path
    _db_path = path


def get_db_path() -> Path:
    """Get the current database path."""
    return _db_path or DEFAULT_DB_PATH


def init_db(path: Optional[Path] = None) -> None:
    """Initialize the database schema."""
    db_path = path or get_db_path()

    # Ensure directory exists
    db_path.parent.mkdir(parents=True, exist_ok=True)

    conn = sqlite3.connect(str(db_path))
    try:
        conn.executescript(SCHEMA)
        conn.commit()
    finally:
        conn.close()


def get_connection() -> sqlite3.Connection:
    """Get a thread-local database connection."""
    if not hasattr(_local, 'connection') or _local.connection is None:
        db_path = get_db_path()

        # Ensure database is initialized
        if not db_path.exists():
            init_db(db_path)

        _local.connection = sqlite3.connect(str(db_path))
        _local.connection.row_factory = sqlite3.Row
        # Enable foreign keys
        _local.connection.execute("PRAGMA foreign_keys = ON")
        # Wait up to 30s for locks instead of failing immediately
        _local.connection.execute("PRAGMA busy_timeout = 30000")
        # Enable WAL mode for better concurrency (allows concurrent reads during writes)
        _local.connection.execute("PRAGMA journal_mode = WAL")

    return _local.connection


def close_connection() -> None:
    """Close the thread-local connection."""
    if hasattr(_local, 'connection') and _local.connection is not None:
        _local.connection.close()
        _local.connection = None


@contextmanager
def transaction():
    """Context manager for database transactions."""
    conn = get_connection()
    try:
        yield conn
        conn.commit()
    except Exception:
        conn.rollback()
        raise


def execute(sql: str, params: tuple = ()) -> sqlite3.Cursor:
    """Execute a SQL statement and return the cursor."""
    conn = get_connection()
    return conn.execute(sql, params)


def executemany(sql: str, params_list: list) -> sqlite3.Cursor:
    """Execute a SQL statement with multiple parameter sets."""
    conn = get_connection()
    return conn.executemany(sql, params_list)


def fetchone(sql: str, params: tuple = ()) -> Optional[sqlite3.Row]:
    """Execute a query and fetch one result."""
    cursor = execute(sql, params)
    return cursor.fetchone()


def fetchall(sql: str, params: tuple = ()) -> list:
    """Execute a query and fetch all results."""
    cursor = execute(sql, params)
    return cursor.fetchall()


def commit() -> None:
    """Commit the current transaction."""
    conn = get_connection()
    conn.commit()


def migrate_db() -> None:
    """Run database migrations to update schema."""
    # No migrations needed - schema is created fresh with all columns
    pass
