package adt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// A server that counts what reaches it, so the test can say how many
// requests the cache turned into none.
func cachingTestServer(t *testing.T) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("X-CSRF-Token", "token")
		if r.Method != http.MethodGet || r.URL.Path == "/sap/bc/adt/discovery" {
			w.WriteHeader(http.StatusOK)
			return
		}
		// Only reads of content count; the CSRF fetch and the writes do not.
		hits.Add(1)
		_, _ = w.Write([]byte("source of " + r.URL.Path + " " + r.Header.Get("Accept")))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestResponseCache(t *testing.T) {
	srv, upstream := cachingTestServer(t)
	c := NewClient(srv.URL, "u", "p", WithCache(time.Minute))
	ctx := context.Background()

	get := func(path, accept string) string {
		t.Helper()
		resp, err := c.transport.Request(ctx, path, &RequestOptions{Accept: accept})
		if err != nil {
			t.Fatal(err)
		}
		return string(resp.Body)
	}
	if got := get("/sap/bc/adt/oo/classes/zcl_a/source/main", "text/plain"); got == "" {
		t.Fatal("empty first answer")
	}
	get("/sap/bc/adt/oo/classes/zcl_a/source/main", "text/plain")
	get("/sap/bc/adt/oo/classes/zcl_a/source/main", "text/plain")
	if n := upstream.Load(); n != 1 {
		t.Errorf("three identical GETs reached the server %d times, want 1", n)
	}
	// A different Accept is a different answer.
	get("/sap/bc/adt/oo/classes/zcl_a/source/main", "application/xml")
	if n := upstream.Load(); n != 2 {
		t.Errorf("a different Accept did not reach the server (%d)", n)
	}
	// A stateful request is never served from the cache.
	if _, err := c.transport.Request(ctx, "/sap/bc/adt/oo/classes/zcl_a/source/main", &RequestOptions{Accept: "text/plain", Stateful: true}); err != nil {
		t.Fatal(err)
	}
	if n := upstream.Load(); n != 3 {
		t.Errorf("a stateful GET was served from the cache (%d)", n)
	}
	stats := c.CacheStats()
	if !stats.Enabled || stats.Hits != 2 || stats.Misses != 2 || stats.Entries != 2 {
		t.Errorf("stats: %+v", stats)
	}

	// A write empties the cache; the next read goes to the server again.
	if _, err := c.transport.Request(ctx, "/sap/bc/adt/oo/classes/zcl_a/source/main", &RequestOptions{Method: http.MethodPut, Body: []byte("x")}); err != nil {
		t.Fatal(err)
	}
	get("/sap/bc/adt/oo/classes/zcl_a/source/main", "text/plain")
	if n := upstream.Load(); n != 4 {
		t.Errorf("after a write: %d reads reached the server, want 4", n)
	}
	if s := c.CacheStats(); s.Invalidations != 1 || s.Entries != 1 {
		t.Errorf("after a write: %+v", s)
	}

	// A data preview POST on a table that changes under the reader is a
	// read every time, and invalidates nothing.
	for i := 0; i < 2; i++ {
		if _, err := c.transport.Request(ctx, "/sap/bc/adt/datapreview/freestyle", &RequestOptions{Method: http.MethodPost, Body: []byte("SELECT rqident FROM tsp01")}); err != nil {
			t.Fatal(err)
		}
	}
	get("/sap/bc/adt/oo/classes/zcl_a/source/main", "text/plain")
	if s := c.CacheStats(); s.Invalidations != 1 || s.Hits != 3 {
		t.Errorf("after a data preview: %+v", s)
	}
	// One on the dictionary is kept.
	before := c.CacheStats().Hits
	for i := 0; i < 3; i++ {
		if _, err := c.transport.Request(ctx, "/sap/bc/adt/datapreview/freestyle", &RequestOptions{Method: http.MethodPost, Body: []byte("SELECT fieldname FROM dd03l WHERE tabname = 'BALDAT'")}); err != nil {
			t.Fatal(err)
		}
	}
	if s := c.CacheStats(); s.Hits != before+2 {
		t.Errorf("a DD03L query was not cached: %+v", s)
	}
}

func TestStableQuery(t *testing.T) {
	for sql, want := range map[string]bool{
		"SELECT a FROM dd03l WHERE x = 1":                                      true,
		"SELECT n~a, t~b FROM tnodeimg AS n LEFT OUTER JOIN tnodeimgt AS t ON": true,
		"SELECT * FROM baldat":                                                 false,
		"SELECT a FROM dd03l AS d INNER JOIN tsp01 AS s ON d~x = s~y":          false,
		"not sql at all": false,
	} {
		if got := stableQuery(sql); got != want {
			t.Errorf("%q: %v", sql, got)
		}
	}
}

func TestResponseCacheExpires(t *testing.T) {
	srv, upstream := cachingTestServer(t)
	c := NewClient(srv.URL, "u", "p", WithCache(20*time.Millisecond))
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if _, err := c.transport.Request(ctx, "/x", &RequestOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(40 * time.Millisecond)
	if _, err := c.transport.Request(ctx, "/x", &RequestOptions{}); err != nil {
		t.Fatal(err)
	}
	if n := upstream.Load(); n != 2 {
		t.Errorf("expired entry not refetched: %d requests", n)
	}
}

func TestNoCacheByDefault(t *testing.T) {
	srv, upstream := cachingTestServer(t)
	c := NewClient(srv.URL, "u", "p")
	for i := 0; i < 2; i++ {
		if _, err := c.transport.Request(context.Background(), "/x", &RequestOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	if n := upstream.Load(); n != 2 {
		t.Errorf("caching without WithCache: %d requests", n)
	}
	if c.CacheStats().Enabled {
		t.Error("stats say enabled")
	}
}
