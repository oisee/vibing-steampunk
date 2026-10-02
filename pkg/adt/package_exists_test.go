package adt

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// listenOn404Port listens on a loopback port whose number contains "404",
// the shape that once turned a timeout into "package does not exist".
func listenOn404Port(t *testing.T) net.Listener {
	t.Helper()
	for p := 40400; p < 40500; p++ {
		if l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p)); err == nil {
			return l
		}
	}
	t.Skip("no free loopback port in 40400-40499")
	return nil
}

// packageExists decides "missing" only from SAP's own answer: a 404, or "not
// found" in SAP's message. A failure that never got an answer is optimistic,
// whatever its text says.
func TestPackageExistsReadsSAPsAnswerNotTheErrorText(t *testing.T) {
	serve := func(t *testing.T, l net.Listener, h http.HandlerFunc) *Client {
		t.Helper()
		ts := httptest.NewUnstartedServer(h)
		if l != nil {
			_ = ts.Listener.Close()
			ts.Listener = l
		}
		ts.Start()
		t.Cleanup(ts.Close)
		return NewClient(ts.URL, "u", "p", WithClient("001"))
	}
	answer := func(status int, body string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-CSRF-Token", "t")
			if r.Method == http.MethodHead || r.Method == http.MethodGet {
				return
			}
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		}
	}

	t.Run("a timeout on a 404 port is not a missing package", func(t *testing.T) {
		c := serve(t, listenOn404Port(t), func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-time.After(5 * time.Second):
			case <-r.Context().Done():
			}
		})
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		if !c.packageExists(ctx, "$TMP") {
			t.Fatal("a call that timed out was read as \"package does not exist\"")
		}
	})
	t.Run("SAP 404 is a missing package", func(t *testing.T) {
		c := serve(t, nil, answer(http.StatusNotFound, ""))
		if c.packageExists(context.Background(), "$ZNOPE") {
			t.Fatal("SAP's 404 was not read as a missing package")
		}
	})
	t.Run("SAP saying not found is a missing package", func(t *testing.T) {
		c := serve(t, nil, answer(http.StatusBadRequest, "Package $ZNOPE not found"))
		if c.packageExists(context.Background(), "$ZNOPE") {
			t.Fatal("SAP's \"not found\" was not read as a missing package")
		}
	})
	t.Run("SAP 500 is not a missing package", func(t *testing.T) {
		c := serve(t, nil, answer(http.StatusInternalServerError, "dump"))
		if !c.packageExists(context.Background(), "$TMP") {
			t.Fatal("a 500 was read as a missing package")
		}
	})
}
