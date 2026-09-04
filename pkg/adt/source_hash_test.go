package adt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSourceHashNormalizesADTLineEndingsAndTrailingBlankLines(t *testing.T) {
	lf := "REPORT zdemo.\nWRITE 'x'.\n"
	adtMaterialized := "REPORT zdemo.\r\nWRITE 'x'.\r\n\r\n"
	if got, want := SourceHash(adtMaterialized), SourceHash(lf); got != want {
		t.Fatalf("SourceHash must ignore ADT line-ending/trailing-newline normalization: got %s, want %s", got, want)
	}
}

func TestUpdateSourceRejectsDriftBeforePut(t *testing.T) {
	const sourceURL = "/sap/bc/adt/programs/programs/ZDEMO/source/main"
	putCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-CSRF-Token", "test-token")
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte("REPORT zdemo.\nWRITE 'changed elsewhere'.\n"))
		case http.MethodPut:
			putCalls++
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TESTUSER", "pw")
	ctx := withExpectedSourceHash(context.Background(), SourceHash("REPORT zdemo.\nWRITE 'original'.\n"))
	err := c.UpdateSource(ctx, sourceURL, "REPORT zdemo.\nWRITE 'replacement'.\n", "LOCK", "")
	if err == nil {
		t.Fatal("UpdateSource must reject a changed source")
	}
	if _, ok := err.(*SourceDriftError); !ok {
		t.Fatalf("UpdateSource error = %T (%v), want *SourceDriftError", err, err)
	}
	if putCalls != 0 {
		t.Fatalf("drift must be rejected before PUT; got %d PUT calls", putCalls)
	}
}

func TestUpdateSourceAcceptsMatchingVersionAndWrites(t *testing.T) {
	const sourceURL = "/sap/bc/adt/programs/programs/ZDEMO/source/main"
	const current = "REPORT zdemo.\nWRITE 'original'.\n"
	putCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-CSRF-Token", "test-token")
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(current))
		case http.MethodPut:
			putCalls++
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "TESTUSER", "pw")
	ctx := withExpectedSourceHash(context.Background(), SourceHash(current))
	if err := c.UpdateSource(ctx, sourceURL, "REPORT zdemo.\nWRITE 'replacement'.\n", "LOCK", ""); err != nil {
		t.Fatalf("UpdateSource returned an error for a matching version: %v", err)
	}
	if putCalls != 1 {
		t.Fatalf("matching version should write once; got %d PUT calls", putCalls)
	}
}
