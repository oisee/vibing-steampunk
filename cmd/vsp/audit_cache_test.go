package main

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/graph"
)

// newTempCache opens a fresh auditCache rooted in the test's temp dir.
// Using t.TempDir() guarantees the file is cleaned up after the test and
// never collides with a developer's real /tmp/vsp-audit-cache-*.db.
func newTempCache(t *testing.T) *auditCache {
	t.Helper()
	path := filepath.Join(t.TempDir(), "audit_cache_test.db")
	c, err := openAuditCache("testsys", path)
	if err != nil {
		t.Fatalf("openAuditCache failed: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})
	return c
}

// TestAuditCache_PutGetRoundtrip checks the most basic contract: what
// you put in is exactly what you get out (via the JSON helpers) and a
// miss returns false.
func TestAuditCache_PutGetRoundtrip(t *testing.T) {
	c := newTempCache(t)

	// Miss on empty cache.
	var dst []string
	if c.getJSON("unused", &dst) {
		t.Fatal("empty cache should miss")
	}

	// Put and read back.
	payload := []string{"alpha", "beta", "gamma"}
	c.putJSON("k1", payload)
	var got []string
	if !c.getJSON("k1", &got) {
		t.Fatal("fresh cache entry should hit")
	}
	if len(got) != len(payload) || got[0] != "alpha" || got[2] != "gamma" {
		t.Errorf("roundtrip mismatch: got %+v, want %+v", got, payload)
	}

	// Different system id stays isolated.
	c.sys = "othersys"
	if c.getJSON("k1", &got) {
		t.Error("cross-system read should miss — keys are scoped per system")
	}
}

// TestAuditCache_Persistence verifies the on-disk store survives a
// handle close+reopen. This guards against "we cached to memory only"
// regressions and makes sure consecutive vsp invocations warm-hit.
func TestAuditCache_Persistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "persist.db")

	c1, err := openAuditCache("sys", path)
	if err != nil {
		t.Fatalf("open #1: %v", err)
	}
	c1.putJSON("k", map[string]int{"x": 42})
	if err := c1.Close(); err != nil {
		t.Fatalf("close #1: %v", err)
	}

	c2, err := openAuditCache("sys", path)
	if err != nil {
		t.Fatalf("open #2: %v", err)
	}
	defer c2.Close()
	var out map[string]int
	if !c2.getJSON("k", &out) {
		t.Fatal("entry should survive close/reopen")
	}
	if out["x"] != 42 {
		t.Errorf("got %d, want 42", out["x"])
	}
}

// TestAuditCache_TTLExpiry ensures expired entries are treated as misses
// even though they still sit in the sqlite file. This is important when
// cached DD02L snapshots outlive their freshness window but before the
// file is manually cleared.
func TestAuditCache_TTLExpiry(t *testing.T) {
	c := newTempCache(t)
	c.putJSON("k", "value")

	// Fresh read with the default TTL must hit.
	var v string
	if !c.getJSON("k", &v) {
		t.Fatal("fresh entry should hit")
	}

	// Zero TTL means "expires immediately" — the TTL form should miss
	// even though the entry was written milliseconds ago.
	if _, ok := c.getTTL("k", 0); ok {
		t.Error("zero-TTL read should miss")
	}

	// Very small TTL and a real sleep: the entry is older than the
	// window → miss. 20ms is long enough for the clock to tick past 1ms.
	time.Sleep(20 * time.Millisecond)
	if _, ok := c.getTTL("k", 1*time.Millisecond); ok {
		t.Error("entry older than TTL should miss")
	}
}

// TestAuditCache_NilSafe covers the "no cache" path — a nil receiver
// must never panic and must always report a miss. This is the
// by-construction proof that --no-cache does not crash the audit.
func TestAuditCache_NilSafe(t *testing.T) {
	var c *auditCache
	if _, ok := c.get("k"); ok {
		t.Error("nil cache get should miss")
	}
	if _, ok := c.getTTL("k", time.Hour); ok {
		t.Error("nil cache getTTL should miss")
	}
	var v string
	if c.getJSON("k", &v) {
		t.Error("nil cache getJSON should miss")
	}
	c.putJSON("k", "value") // must not panic
	if err := c.Close(); err != nil {
		t.Errorf("nil Close returned error: %v", err)
	}
}

// TestQueryCodeRefsCached_EquivalenceAndMemoization is the main
// cache-on vs cache-off equivalence test: run the cached wrapper
// against a deterministic fake fetcher with caching on, with caching
// off, and over two calls in a row, then assert identical results and
// that the second warm call did not re-invoke the fetcher.
//
// This is the test the user explicitly asked for: prove the cache
// does not change observable behaviour, only timing.
func TestQueryCodeRefsCached_EquivalenceAndMemoization(t *testing.T) {
	// Restore the real fetcher after the test so later tests aren't
	// affected by our swap.
	orig := queryCodeRefsFunc
	t.Cleanup(func() { queryCodeRefsFunc = orig })

	var fakeCalls int64
	fakeRefs := []graph.TableCodeRef{
		{Table: "ZEXAMPLE_CONFIG", FromInclude: "ZCL_TEST_MAIN==CP", RefKind: "WBGT_TY", Source: "WBCROSSGT"},
		{Table: "ZEXAMPLE_STATUS", FromInclude: "ZCL_TEST_MAIN==CI", RefKind: "WBGT_TY", Source: "WBCROSSGT"},
	}
	queryCodeRefsFunc = func(_ context.Context, _ *adt.Client, name, objType string) []graph.TableCodeRef {
		atomic.AddInt64(&fakeCalls, 1)
		// Return deep copies so the test cannot accidentally observe
		// the callee mutating its input — same discipline the real
		// fetcher maintains.
		out := make([]graph.TableCodeRef, len(fakeRefs))
		copy(out, fakeRefs)
		return out
	}

	ctx := context.Background()

	// --- Cache off path ---
	off1 := queryCodeRefsCached(ctx, nil, "ZCL_TEST_MAIN", "CLAS", nil)
	off2 := queryCodeRefsCached(ctx, nil, "ZCL_TEST_MAIN", "CLAS", nil)
	if !refsEqual(off1, off2) {
		t.Errorf("cache-off runs diverged: %+v vs %+v", off1, off2)
	}
	if atomic.LoadInt64(&fakeCalls) != 2 {
		t.Errorf("cache-off expected 2 fetcher calls, got %d", fakeCalls)
	}

	// --- Cache on path ---
	cache := newTempCache(t)
	atomic.StoreInt64(&fakeCalls, 0)

	on1 := queryCodeRefsCached(ctx, nil, "ZCL_TEST_MAIN", "CLAS", cache)
	if atomic.LoadInt64(&fakeCalls) != 1 {
		t.Errorf("first cached call: expected 1 fetcher call, got %d", fakeCalls)
	}
	if !refsEqual(on1, off1) {
		t.Errorf("cache-on first result differs from cache-off: %+v vs %+v", on1, off1)
	}

	on2 := queryCodeRefsCached(ctx, nil, "ZCL_TEST_MAIN", "CLAS", cache)
	if atomic.LoadInt64(&fakeCalls) != 1 {
		t.Errorf("second cached call: fetcher should not be re-invoked, got %d total calls", fakeCalls)
	}
	if !refsEqual(on2, on1) {
		t.Errorf("cached re-read diverged from first: %+v vs %+v", on2, on1)
	}

	// --- Cache miss for a different object ---
	_ = queryCodeRefsCached(ctx, nil, "ZCL_OTHER", "CLAS", cache)
	if atomic.LoadInt64(&fakeCalls) != 2 {
		t.Errorf("different key should miss cache, expected 2 fetcher calls total, got %d", fakeCalls)
	}
}

// TestQueryCodeRefsCached_EmptyResultCached makes sure we also cache
// the empty-result case. Without this, objects that genuinely have no
// CROSS/WBCROSSGT hits would re-query on every call.
func TestQueryCodeRefsCached_EmptyResultCached(t *testing.T) {
	orig := queryCodeRefsFunc
	t.Cleanup(func() { queryCodeRefsFunc = orig })

	var calls int64
	queryCodeRefsFunc = func(_ context.Context, _ *adt.Client, name, objType string) []graph.TableCodeRef {
		atomic.AddInt64(&calls, 1)
		return nil
	}

	cache := newTempCache(t)
	ctx := context.Background()

	if r := queryCodeRefsCached(ctx, nil, "ZCL_EMPTY", "CLAS", cache); len(r) != 0 {
		t.Errorf("expected empty, got %+v", r)
	}
	if r := queryCodeRefsCached(ctx, nil, "ZCL_EMPTY", "CLAS", cache); len(r) != 0 {
		t.Errorf("expected empty on re-read, got %+v", r)
	}
	if atomic.LoadInt64(&calls) != 1 {
		t.Errorf("fetcher called %d times, want 1 — empty result must be cached", calls)
	}
}

// refsEqual is a stable deep-equality helper for TableCodeRef slices.
// The normal reflect.DeepEqual would work but is noisier in failure
// output than a tuple comparison, and sorts guarantee order invariance
// (the real fetcher does not guarantee a deterministic order yet).
func refsEqual(a, b []graph.TableCodeRef) bool {
	if len(a) != len(b) {
		return false
	}
	copyA := append([]graph.TableCodeRef(nil), a...)
	copyB := append([]graph.TableCodeRef(nil), b...)
	sortRefs := func(s []graph.TableCodeRef) {
		sort.Slice(s, func(i, j int) bool {
			if s[i].Table != s[j].Table {
				return s[i].Table < s[j].Table
			}
			return s[i].FromInclude < s[j].FromInclude
		})
	}
	sortRefs(copyA)
	sortRefs(copyB)
	for i := range copyA {
		if copyA[i] != copyB[i] {
			return false
		}
	}
	return true
}

// TestAuditCache_FilesystemIsolation verifies that test instances don't
// accidentally pollute /tmp — the path passed explicitly wins over the
// /tmp default. Regression guard: an earlier refactor once made the
// override silently fall back to /tmp.
func TestAuditCache_FilesystemIsolation(t *testing.T) {
	dir := t.TempDir()
	explicit := filepath.Join(dir, "isolation.db")
	c, err := openAuditCache("whatever", explicit)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer c.Close()
	if _, err := os.Stat(explicit); err != nil {
		t.Errorf("explicit path %s not created: %v", explicit, err)
	}
	// Make sure nothing in /tmp/vsp-audit-cache-whatever.db got created
	// as a side-effect of this call.
	sideEffect := filepath.Join(os.TempDir(), "vsp-audit-cache-whatever.db")
	if _, err := os.Stat(sideEffect); err == nil && sideEffect != explicit {
		// Only fail if the file exists AND isn't the one we asked for.
		// On systems where TempDir() happens to resolve to the test dir
		// parent this check is a no-op, which is fine.
		t.Errorf("openAuditCache wrote to default %s despite explicit override", sideEffect)
	}
}
