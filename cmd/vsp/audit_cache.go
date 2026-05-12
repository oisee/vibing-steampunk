package main

import (
	"crypto/sha1"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// auditCache is a tiny on-disk key/value cache for cr-config-audit.
// Only static / slowly-changing DDIC system tables go here — DD02L,
// DD03L, DD04L, DD01L, DD07L, TADIR. Transport content (E070, E071,
// E071K, E070A) must NEVER be cached because its whole purpose in the
// audit is to reflect the current CR state.
//
// The store lives under /tmp by default so it is guaranteed to be
// outside any git working tree. If the user explicitly passes a path
// via the --cache flag we honour it but still warn if the path is
// inside a git-tracked directory.
type auditCache struct {
	db  *sql.DB
	mu  sync.Mutex
	sys string // system identifier — scopes every key so multiple SAP systems coexist
}

// cacheTTL is the default validity window for cached DDIC entries. Schemas
// do change in practice (customer field additions, new data elements), so
// we refresh daily. Users who need aggressive cache invalidation can just
// delete the file at any time — it is a cache, not a source of truth.
const cacheTTL = 24 * time.Hour

// shortCacheTTL applies to entries that can change more often than DDIC:
// per-object CROSS/WBCROSSGT symbol scans depend on ABAP source, which an
// active developer may edit multiple times a day. One hour is short
// enough to stay fresh through most dev cycles while still collapsing
// the "same CR, three re-runs in a minute" loop to a single SAP pass.
const shortCacheTTL = 1 * time.Hour

// openAuditCache opens (creating if needed) a sqlite cache file for the
// given SAP system identifier. Path resolution:
//
//  1. If override != "" → use that exact path.
//  2. Else default to /tmp/vsp-audit-cache-<system>.db.
//
// The caller is responsible for calling Close when done. A nil cache is
// returned only if sqlite is unusable; in that case filterDDICTables and
// walkDDICMetadata fall back to direct queries (no cache).
func openAuditCache(systemName, override string) (*auditCache, error) {
	if systemName == "" {
		systemName = "default"
	}
	path := override
	if path == "" {
		// /tmp is outside any repo, survives between runs, cleaned by the OS.
		path = filepath.Join(os.TempDir(), "vsp-audit-cache-"+sanitizeSystemName(systemName)+".db")
	}
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("opening audit cache %s: %w", path, err)
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS audit_cache (
			sys         TEXT    NOT NULL,
			key         TEXT    NOT NULL,
			payload     BLOB    NOT NULL,
			created_at  INTEGER NOT NULL,
			PRIMARY KEY (sys, key)
		) WITHOUT ROWID
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialising audit cache schema: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Audit cache: %s\n", path)
	return &auditCache{db: db, sys: systemName}, nil
}

// sanitizeSystemName strips anything that would be awkward in a filename —
// we only keep alphanumeric, dash and underscore so the cache path stays
// portable across platforms.
func sanitizeSystemName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	if out == "" {
		return "default"
	}
	return out
}

// get looks up a fresh (not expired) payload using the default cacheTTL.
// Returns (payload, true) on hit, (nil, false) on miss or expiry. A miss
// means the caller should fetch from SAP and put the result back.
func (c *auditCache) get(key string) ([]byte, bool) {
	return c.getTTL(key, cacheTTL)
}

// getTTL is the TTL-parameterised form of get. Call it directly when a
// cached entry has a freshness window different from the default — e.g.
// shortCacheTTL for CROSS/WBCROSSGT per-object scans.
func (c *auditCache) getTTL(key string, ttl time.Duration) ([]byte, bool) {
	if c == nil || c.db == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var payload []byte
	var createdAt int64
	err := c.db.QueryRow(
		`SELECT payload, created_at FROM audit_cache WHERE sys = ? AND key = ?`,
		c.sys, key).Scan(&payload, &createdAt)
	if err != nil {
		return nil, false
	}
	if time.Since(time.Unix(createdAt, 0)) > ttl {
		return nil, false
	}
	return payload, true
}

// put stores or replaces a payload under the given key, timestamped now.
// On error we log to stderr and swallow — cache failures must never break
// the audit itself.
func (c *auditCache) put(key string, payload []byte) {
	if c == nil || c.db == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.db.Exec(
		`INSERT OR REPLACE INTO audit_cache (sys, key, payload, created_at) VALUES (?, ?, ?, ?)`,
		c.sys, key, payload, time.Now().Unix())
	if err != nil {
		fmt.Fprintf(os.Stderr, "  [cache warn] put %s failed: %v\n", key, err)
	}
}

// getJSON is a convenience wrapper around get that unmarshals into v and
// returns whether the cache had a hit. Uses the default cacheTTL.
func (c *auditCache) getJSON(key string, v any) bool {
	return c.getJSONTTL(key, cacheTTL, v)
}

// getJSONTTL is the TTL-parameterised form of getJSON.
func (c *auditCache) getJSONTTL(key string, ttl time.Duration, v any) bool {
	data, ok := c.getTTL(key, ttl)
	if !ok {
		return false
	}
	if err := json.Unmarshal(data, v); err != nil {
		fmt.Fprintf(os.Stderr, "  [cache warn] corrupt entry %s: %v — refetching\n", key, err)
		return false
	}
	return true
}

// putJSON marshals v as JSON and stores it. Swallows marshal errors the
// same way put swallows sqlite errors — cache is best-effort.
func (c *auditCache) putJSON(key string, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  [cache warn] marshal %s failed: %v\n", key, err)
		return
	}
	c.put(key, data)
}

// Close releases the underlying sqlite handle. Safe to call on nil.
func (c *auditCache) Close() error {
	if c == nil || c.db == nil {
		return nil
	}
	return c.db.Close()
}

// hashStringList returns a stable hex digest of a sorted string slice —
// used as a cache key so a re-run of the same CR (same scope set) hits
// the cache regardless of map iteration order.
func hashStringList(items []string) string {
	h := sha1.New()
	for _, s := range items {
		h.Write([]byte(s))
		h.Write([]byte{0})
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
