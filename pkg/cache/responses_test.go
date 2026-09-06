package cache

import (
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

func TestResponseStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "responses.db")
	s, err := NewResponseStore(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Put("k1", &adt.CachedResponse{StatusCode: 200, Headers: http.Header{"Content-Type": {"text/plain"}}, Body: []byte("hello"), Expires: time.Now().Add(time.Minute)})
	s.Put("k2", &adt.CachedResponse{StatusCode: 200, Body: []byte("gone"), Expires: time.Now().Add(-time.Minute)})
	if r, ok := s.Get("k1"); !ok || string(r.Body) != "hello" || r.Headers.Get("Content-Type") != "text/plain" {
		t.Errorf("k1: %+v %v", r, ok)
	}
	if _, ok := s.Get("k2"); ok {
		t.Error("expired entry returned")
	}
	if s.Len() != 1 {
		t.Errorf("len %d", s.Len())
	}
	s.Close()

	// The next process finds what the last one stored, and not what expired.
	s2, err := NewResponseStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if r, ok := s2.Get("k1"); !ok || string(r.Body) != "hello" {
		t.Error("entry did not survive reopening")
	}
	s2.Clear()
	if s2.Len() != 0 {
		t.Error("Clear left entries")
	}
}
