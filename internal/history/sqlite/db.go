package sqlite

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const targetUserVersion = 1

// DB wraps a SQLite database handle for history recording and querying.
type DB struct {
	db *sql.DB
}

// Open creates or opens a SQLite database at the specified path and configures PRAGMAs.
func Open(path string) (*DB, error) {
	if path != ":memory:" {
		dir := filepath.Dir(path)
		if dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, fmt.Errorf("sqlite open: mkdir %s: %w", dir, err)
			}
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("sqlite open: %w", err)
	}

	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA busy_timeout=5000;",
		"PRAGMA synchronous=NORMAL;",
		"PRAGMA foreign_keys=ON;",
	}

	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("sqlite pragma %q: %w", p, err)
		}
	}

	s := &DB{db: db}
	if err := s.Migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite migrate: %w", err)
	}

	return s, nil
}

// Migrate checks table columns and applies missing schema migrations or column renames.
func (s *DB) Migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS requests (
  id                      TEXT PRIMARY KEY,
  timestamp               INTEGER NOT NULL,
  provider                TEXT,
  account                 TEXT,
  model_requested         TEXT NOT NULL,
  model_served            TEXT,
  key_id                  TEXT,
  session                 TEXT,
  endpoint                TEXT,
  stream                  INTEGER NOT NULL,
  outcome                 TEXT NOT NULL,
  input_tokens            INTEGER NOT NULL DEFAULT 0,
  output_tokens           INTEGER NOT NULL DEFAULT 0,
  cache_read_tokens       INTEGER NOT NULL DEFAULT 0,
  cache_creation_tokens   INTEGER NOT NULL DEFAULT 0,
  latency_ms              INTEGER,
  request_body            TEXT,
  response_body           TEXT,
  translated_request_body TEXT,
  raw_response_body       TEXT,
  attempts                TEXT NOT NULL DEFAULT '[]'
);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("create requests table: %w", err)
	}

	// Inspect existing columns
	rows, err := s.db.Query("PRAGMA table_info(requests);")
	if err != nil {
		return fmt.Errorf("inspect table info: %w", err)
	}

	cols := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, typeStr string
		var notNull, pk int
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &typeStr, &notNull, &dfltValue, &pk); err != nil {
			rows.Close()
			return fmt.Errorf("scan table info row: %w", err)
		}
		cols[name] = true
	}
	rows.Close()

	// Rename old abbreviated columns if present in an existing table
	renames := map[string]string{
		"ts":              "timestamp",
		"model_req":       "model_requested",
		"req_body":        "request_body",
		"resp_body":       "response_body",
		"xlated_req_body": "translated_request_body",
		"raw_resp_body":   "raw_response_body",
	}

	for oldCol, newCol := range renames {
		if cols[oldCol] && !cols[newCol] {
			alter := fmt.Sprintf("ALTER TABLE requests RENAME COLUMN %s TO %s;", oldCol, newCol)
			if _, err := s.db.Exec(alter); err != nil {
				return fmt.Errorf("rename column %s to %s: %w", oldCol, newCol, err)
			}
			cols[newCol] = true
			delete(cols, oldCol)
		}
	}

	// Add missing columns if they do not exist
	newCols := []string{"request_body", "response_body", "translated_request_body", "raw_response_body", "account"}
	for _, col := range newCols {
		if !cols[col] {
			alter := fmt.Sprintf("ALTER TABLE requests ADD COLUMN %s TEXT;", col)
			if _, err := s.db.Exec(alter); err != nil {
				return fmt.Errorf("add column %s: %w", col, err)
			}
		}
	}

	// Ensure indexes exist
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_requests_ts ON requests(timestamp DESC);",
		"CREATE INDEX IF NOT EXISTS idx_requests_provider ON requests(provider, timestamp DESC);",
		"CREATE INDEX IF NOT EXISTS idx_requests_session ON requests(session, timestamp DESC);",
	}
	for _, idx := range indexes {
		if _, err := s.db.Exec(idx); err != nil {
			return fmt.Errorf("create index: %w", err)
		}
	}

	return nil
}

// Close closes the underlying SQLite database handle.
func (s *DB) Close() error {
	return s.db.Close()
}
