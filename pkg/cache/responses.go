package cache

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // pure Go; see sqlite.go and issue #157

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// ResponseStore keeps the ADT client's cached responses in SQLite, so a
// CLI run can reuse what the previous one read. Entries carry their expiry;
// a write through the client clears the whole table, as it clears the
// in-memory store.
type ResponseStore struct {
	db *sql.DB
}

// NewResponseStore opens (creating as needed) the store at path.
func NewResponseStore(path string) (*ResponseStore, error) {
	if path == "" {
		return nil, fmt.Errorf("cache: a path is required")
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("cache: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("cache: opening %s: %w", path, err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS responses (
		key TEXT PRIMARY KEY,
		status INTEGER NOT NULL,
		headers TEXT NOT NULL,
		body BLOB NOT NULL,
		expires INTEGER NOT NULL
	)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("cache: creating the responses table: %w", err)
	}
	// Expired rows are worth nothing to anyone; drop them on open.
	_, _ = db.Exec(`DELETE FROM responses WHERE expires < ?`, time.Now().Unix())
	return &ResponseStore{db: db}, nil
}

var _ adt.ResponseStore = (*ResponseStore)(nil)

func (s *ResponseStore) Get(key string) (*adt.CachedResponse, bool) {
	var status int
	var headers string
	var body []byte
	var expires int64
	err := s.db.QueryRow(`SELECT status, headers, body, expires FROM responses WHERE key = ?`, key).Scan(&status, &headers, &body, &expires)
	if err != nil {
		return nil, false
	}
	exp := time.Unix(expires, 0)
	if time.Now().After(exp) {
		_, _ = s.db.Exec(`DELETE FROM responses WHERE key = ?`, key)
		return nil, false
	}
	var h http.Header
	_ = json.Unmarshal([]byte(headers), &h)
	return &adt.CachedResponse{StatusCode: status, Headers: h, Body: body, Expires: exp}, true
}

func (s *ResponseStore) Put(key string, r *adt.CachedResponse) {
	headers, _ := json.Marshal(r.Headers)
	_, _ = s.db.Exec(`INSERT OR REPLACE INTO responses (key, status, headers, body, expires) VALUES (?, ?, ?, ?, ?)`,
		key, r.StatusCode, string(headers), r.Body, r.Expires.Unix())
}

func (s *ResponseStore) Clear() {
	_, _ = s.db.Exec(`DELETE FROM responses`)
}

func (s *ResponseStore) Len() int {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM responses WHERE expires >= ?`, time.Now().Unix()).Scan(&n); err != nil {
		return 0
	}
	return n
}

// Close releases the database.
func (s *ResponseStore) Close() error { return s.db.Close() }
