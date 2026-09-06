package adt

import (
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// A response cache for reads. An MCP session asks for the same class, the
// same structure, the same where-used list again and again — every dependency
// resolution reads the includes of the objects it just read — and each of
// those is a round trip to the system for bytes that have not changed. The
// transport keeps successful GET responses for a while and hands them back;
// any request that modifies something empties the cache, because after a
// write the cheapest correct assumption is that anything may have moved.
//
// Data preview queries are POSTs and are kept only when every table they
// read is one that changes with development, not with business — DD03L,
// TADIR, CROSS, T100. A query on the logs, the spool or the jobs is never
// kept: those change under the reader, and a stale row there is a wrong
// answer rather than a slow one.

// CachedResponse is one stored response.
type CachedResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
	Expires    time.Time
}

// ResponseStore keeps cached responses. The in-memory store lives as long as
// the process; pkg/cache has one on SQLite that outlives it.
type ResponseStore interface {
	Get(key string) (*CachedResponse, bool)
	Put(key string, r *CachedResponse)
	Clear()
	Len() int
}

// CacheStats counts what the cache did.
type CacheStats struct {
	Enabled       bool          `json:"enabled"`
	TTL           time.Duration `json:"ttl,omitempty"`
	Hits          int64         `json:"hits"`
	Misses        int64         `json:"misses"`
	Invalidations int64         `json:"invalidations"`
	Entries       int           `json:"entries"`
}

// MemoryResponseStore is the default store: a map, gone with the process.
type MemoryResponseStore struct {
	mu      sync.RWMutex
	entries map[string]*CachedResponse
}

// NewMemoryResponseStore makes an empty in-memory store.
func NewMemoryResponseStore() *MemoryResponseStore {
	return &MemoryResponseStore{entries: map[string]*CachedResponse{}}
}

func (m *MemoryResponseStore) Get(key string) (*CachedResponse, bool) {
	m.mu.RLock()
	r, ok := m.entries[key]
	m.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Now().After(r.Expires) {
		m.mu.Lock()
		delete(m.entries, key)
		m.mu.Unlock()
		return nil, false
	}
	return r, true
}

func (m *MemoryResponseStore) Put(key string, r *CachedResponse) {
	m.mu.Lock()
	m.entries[key] = r
	m.mu.Unlock()
}

func (m *MemoryResponseStore) Clear() {
	m.mu.Lock()
	m.entries = map[string]*CachedResponse{}
	m.mu.Unlock()
}

func (m *MemoryResponseStore) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.entries)
}

// responseCache is the transport's view: a store, a TTL, and counters.
type responseCache struct {
	store         ResponseStore
	ttl           time.Duration
	hits, misses  atomic.Int64
	invalidations atomic.Int64
}

// DefaultCacheTTL is how long a read is trusted when nothing says otherwise.
const DefaultCacheTTL = 10 * time.Minute

func newResponseCache(store ResponseStore, ttl time.Duration) *responseCache {
	if store == nil {
		store = NewMemoryResponseStore()
	}
	if ttl <= 0 {
		ttl = DefaultCacheTTL
	}
	return &responseCache{store: store, ttl: ttl}
}

func (c *responseCache) get(key string) (*Response, bool) {
	r, ok := c.store.Get(key)
	if !ok {
		c.misses.Add(1)
		return nil, false
	}
	c.hits.Add(1)
	body := make([]byte, len(r.Body))
	copy(body, r.Body)
	return &Response{StatusCode: r.StatusCode, Headers: r.Headers.Clone(), Body: body}, true
}

func (c *responseCache) put(key string, resp *Response) {
	body := make([]byte, len(resp.Body))
	copy(body, resp.Body)
	c.store.Put(key, &CachedResponse{StatusCode: resp.StatusCode, Headers: resp.Headers.Clone(), Body: body, Expires: time.Now().Add(c.ttl)})
}

func (c *responseCache) invalidate() {
	c.invalidations.Add(1)
	c.store.Clear()
}

func (c *responseCache) stats() CacheStats {
	return CacheStats{Enabled: true, TTL: c.ttl, Hits: c.hits.Load(), Misses: c.misses.Load(), Invalidations: c.invalidations.Load(), Entries: c.store.Len()}
}

// cacheable says whether a request's answer may be kept: a plain GET on a
// stateless session, or a data preview query that reads only tables which
// change with development activity rather than with business — DDIC, the
// repository index, the cross-reference tables, message and documentation
// texts. A query on BALDAT or TSP01 is never kept: those change under the
// reader. A stateful request belongs to a session whose state the answer
// may depend on.
func cacheable(path string, opts *RequestOptions) bool {
	if opts.Stateful {
		return false
	}
	if opts.Method == http.MethodGet {
		return true
	}
	return opts.Method == http.MethodPost && strings.HasPrefix(path, "/sap/bc/adt/datapreview/freestyle") && stableQuery(string(opts.Body))
}

// stableTables are the tables a data preview query may be cached on. They
// describe the system's code and dictionary, and any write through this
// client empties the cache anyway.
var stableTables = map[string]bool{
	"DD02L": true, "DD02T": true, "DD03L": true, "DD04L": true, "DD04T": true, "DD40L": true, "DD01L": true, "DD07L": true, "DD07T": true,
	"TADIR": true, "TDEVC": true, "TDEVCT": true, "TRDIR": true, "TRDIRT": true, "D010INC": true, "D010TAB": true,
	"CROSS": true, "WBCROSSGT": true, "WBCROSSGTX": true, "WBCROSSI": true,
	"TFDIR": true, "TFTIT": true, "ENLFDIR": true, "SEOCLASS": true, "SEOCLASSDF": true, "SEOCLASSTX": true, "SEOMETAREL": true, "SEOCOMPO": true, "SEOCOMPODF": true,
	"T100": true, "T100T": true, "DOKIL": true, "DOKTL": true, "DOKHL": true,
	"TSTC": true, "TSTCT": true, "CUS_IMGACH": true, "CUS_IMGACT": true, "TNODEIMG": true, "TNODEIMGR": true, "TNODEIMGT": true,
	"E070": true, "E071": true, "E07T": true,
}

var fromTable = regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+([A-Za-z0-9_/]+)`)

// stableQuery reports whether every table a statement reads is stable.
func stableQuery(sql string) bool {
	tables := fromTable.FindAllStringSubmatch(sql, -1)
	if len(tables) == 0 {
		return false
	}
	for _, m := range tables {
		if !stableTables[strings.ToUpper(m[1])] {
			return false
		}
	}
	return true
}

// CacheStats reports the response cache's counters; Enabled is false when
// no cache was configured.
func (c *Client) CacheStats() CacheStats {
	if c.transport == nil || c.transport.cache == nil {
		return CacheStats{}
	}
	return c.transport.cache.stats()
}

// InvalidateCache empties the response cache, for a caller that knows the
// system changed under it.
func (c *Client) InvalidateCache() {
	if c.transport != nil && c.transport.cache != nil {
		c.transport.cache.invalidate()
	}
}
