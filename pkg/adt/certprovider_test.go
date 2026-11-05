package adt

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"testing"
	"time"
)

func fakeCert(raw string, notAfter time.Time) *tls.Certificate {
	return &tls.Certificate{Leaf: &x509.Certificate{Raw: []byte(raw), NotAfter: notAfter}}
}

// loaderOf hands out the given certs in order and counts the loads.
func loaderOf(loads *int, certs ...*tls.Certificate) func() (*tls.Certificate, error) {
	return func() (*tls.Certificate, error) {
		c := certs[*loads]
		*loads++
		return c, nil
	}
}

func TestCachingCertProvider_ReusesWhileKeychainLeafIsUnchanged(t *testing.T) {
	later := time.Now().Add(time.Hour)
	loads := 0
	p := NewCachingCertProvider(loaderOf(&loads, fakeCert("A", later)),
		func() ([]byte, error) { return []byte("A"), nil })

	for i := 0; i < 3; i++ {
		if _, err := p(); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if loads != 1 {
		t.Fatalf("loads = %d, want 1 — an unchanged leaf must not reload", loads)
	}
}

// The 2026-09-13 failure: SLC minted a new keypair while the server held a
// cert loaded at startup. The cached cert was hours from expiry, so the old
// provider kept handing out a signer for a key that no longer existed.
func TestCachingCertProvider_ReloadsWhenSLCRollsTheLeafBeforeExpiry(t *testing.T) {
	later := time.Now().Add(time.Hour)
	loads := 0
	keychainLeaf := "OLD"
	p := NewCachingCertProvider(loaderOf(&loads, fakeCert("OLD", later), fakeCert("NEW", later)),
		func() ([]byte, error) { return []byte(keychainLeaf), nil })

	if c, _ := p(); string(c.Leaf.Raw) != "OLD" {
		t.Fatalf("first cert = %q, want OLD", c.Leaf.Raw)
	}
	keychainLeaf = "NEW" // SLC re-enrolled
	c, err := p()
	if err != nil {
		t.Fatal(err)
	}
	if string(c.Leaf.Raw) != "NEW" || loads != 2 {
		t.Fatalf("got %q after %d loads, want NEW after 2", c.Leaf.Raw, loads)
	}
}

func TestCachingCertProvider_ProbeFailureNeverFallsBackToTheCachedCert(t *testing.T) {
	later := time.Now().Add(time.Hour)
	loads := 0
	gone := errors.New("no valid keychain identity")
	probeErr := error(nil)
	p := NewCachingCertProvider(func() (*tls.Certificate, error) {
		loads++
		if loads == 1 {
			return fakeCert("A", later), nil
		}
		return nil, gone
	}, func() ([]byte, error) { return []byte("A"), probeErr })

	if _, err := p(); err != nil {
		t.Fatal(err)
	}
	probeErr = gone // SLC logged out: the identity is gone
	if c, err := p(); !errors.Is(err, gone) || c != nil {
		t.Fatalf("got cert=%v err=%v, want the load error and no cert", c, err)
	}
}

func TestCachingCertProvider_ReloadsNearExpiry(t *testing.T) {
	loads := 0
	p := NewCachingCertProvider(loaderOf(&loads,
		fakeCert("A", time.Now().Add(time.Minute)), fakeCert("B", time.Now().Add(time.Hour))),
		func() ([]byte, error) { return []byte("A"), nil })

	p()
	if c, _ := p(); string(c.Leaf.Raw) != "B" || loads != 2 {
		t.Fatalf("got %q after %d loads, want B after 2 — a leaf inside the 5 min margin reloads", c.Leaf.Raw, loads)
	}
}

func TestCachingCertProvider_NilProbeKeepsTheTimeOnlyCache(t *testing.T) {
	loads := 0
	p := NewCachingCertProvider(loaderOf(&loads, fakeCert("A", time.Now().Add(time.Hour))), nil)
	p()
	p()
	if loads != 1 {
		t.Fatalf("loads = %d, want 1", loads)
	}
}
