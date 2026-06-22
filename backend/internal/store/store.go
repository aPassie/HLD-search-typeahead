// Package store is the durable source of truth (SQLite). The in-memory Trie is
// rebuilt from here on startup.
package store

import (
	"bufio"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"searchtypeahead/internal/model"

	_ "modernc.org/sqlite" // pure-Go driver, registered as "sqlite" (no cgo)
)

// Store wraps the SQLite connection pool.
type Store struct {
	db     *sql.DB
	reads  atomic.Int64 // rows read at startup (building the Trie)
	writes atomic.Int64 // rows written (runtime UPSERTs)
}

const schema = `
CREATE TABLE IF NOT EXISTS queries (
    query        TEXT PRIMARY KEY,
    count        INTEGER NOT NULL,
    recent_value REAL    NOT NULL DEFAULT 0,
    recent_ts    INTEGER NOT NULL DEFAULT 0
) WITHOUT ROWID;`

// Open opens (creating if needed) the database and ensures the schema exists.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// WAL lets readers proceed while the (later) batch writer commits.
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL;`); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close releases the database.
func (s *Store) Close() error { return s.db.Close() }

// Count returns the number of distinct queries stored.
func (s *Store) Count() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM queries`).Scan(&n)
	return n, err
}

// ForEach streams every row to fn — used to build the Trie at startup.
func (s *Store) ForEach(fn func(model.Candidate)) error {
	rows, err := s.db.Query(`SELECT query, count, recent_value, recent_ts FROM queries`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var c model.Candidate
		if err := rows.Scan(&c.Query, &c.Count, &c.RecentValue, &c.RecentTS); err != nil {
			return err
		}
		s.reads.Add(1)
		fn(c)
	}
	return rows.Err()
}

// BulkLoadCSV ingests a `query,count` CSV in a single transaction. A header row
// or any row with a non-positive / non-numeric count is skipped; duplicate
// (normalized) queries are summed via UPSERT. Returns the number of rows applied.
func (s *Store) BulkLoadCSV(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	r := csv.NewReader(bufio.NewReaderSize(f, 1<<20))
	r.FieldsPerRecord = -1   // tolerate ragged rows
	r.ReuseRecord = true
	r.LazyQuotes = true      // tolerate stray quotes in real titles (e.g. Wikipedia)

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	stmt, err := tx.Prepare(`
INSERT INTO queries(query, count, recent_value, recent_ts)
VALUES (?, ?, 0, ?)
ON CONFLICT(query) DO UPDATE SET count = count + excluded.count;`)
	if err != nil {
		tx.Rollback()
		return 0, err
	}

	now := time.Now().Unix()
	n := 0
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			tx.Rollback()
			return 0, fmt.Errorf("csv read: %w", err)
		}
		if len(rec) < 2 {
			continue
		}
		q := model.Normalize(rec[0])
		if q == "" {
			continue
		}
		cnt, err := strconv.ParseInt(strings.TrimSpace(rec[1]), 10, 64)
		if err != nil || cnt <= 0 {
			continue // header row or bad count
		}
		if _, err := stmt.Exec(q, cnt, now); err != nil {
			tx.Rollback()
			return 0, err
		}
		n++
	}
	stmt.Close()
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return n, nil
}

// PersistBatch writes the absolute count and recency for each changed query in
// one transaction. Absolute values (not deltas) mean a failed flush needs no
// retry: the Trie stays authoritative and the next flush rewrites the row.
func (s *Store) PersistBatch(rows map[string]model.Candidate) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	stmt, err := tx.Prepare(`
INSERT INTO queries(query, count, recent_value, recent_ts)
VALUES (?, ?, ?, ?)
ON CONFLICT(query) DO UPDATE SET
    count        = excluded.count,
    recent_value = excluded.recent_value,
    recent_ts    = excluded.recent_ts;`)
	if err != nil {
		tx.Rollback()
		return 0, err
	}
	for q, r := range rows {
		if _, err := stmt.Exec(q, r.Count, r.RecentValue, r.RecentTS); err != nil {
			stmt.Close()
			tx.Rollback()
			return 0, err
		}
	}
	stmt.Close()
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	s.writes.Add(int64(len(rows)))
	return len(rows), nil
}

// DBStats returns rows read at startup and rows written (runtime UPSERTs).
func (s *Store) DBStats() (reads, writes int64) {
	return s.reads.Load(), s.writes.Load()
}
