package adt

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"
	"time"
)

func leaf(issuer string, notBefore, notAfter time.Duration, now time.Time) *x509.Certificate {
	return &x509.Certificate{
		Issuer:    pkix.Name{CommonName: issuer},
		NotBefore: now.Add(notBefore),
		NotAfter:  now.Add(notAfter),
	}
}

func TestFreshestValid_PrefersLatestNotBefore(t *testing.T) {
	now := time.Now()
	leaves := []*x509.Certificate{
		leaf("SLS CA", -48*time.Hour, 24*time.Hour, now), // yesterday's login, still valid
		leaf("SLS CA", -1*time.Hour, 24*time.Hour, now),  // this morning's login
	}
	if got := freshestValid(now, leaves, issuerIn([]string{"SLS CA"})); got != 1 {
		t.Fatalf("expected the most recently issued certificate (index 1), got %d", got)
	}
}

func TestFreshestValid_SkipsExpiredNotYetValidNilAndOtherIssuers(t *testing.T) {
	now := time.Now()
	leaves := []*x509.Certificate{
		nil, // identity whose certificate could not be read
		leaf("SLS CA", -2*time.Hour, -1*time.Minute, now), // expired
		leaf("SLS CA", 1*time.Hour, 24*time.Hour, now),    // not yet valid
		leaf("Other CA", -1*time.Hour, 24*time.Hour, now), // wrong issuer
		leaf("SLS CA", -3*time.Hour, 24*time.Hour, now),   // the only one that qualifies
	}
	if got := freshestValid(now, leaves, issuerIn([]string{"SLS CA"})); got != 4 {
		t.Fatalf("expected index 4, got %d", got)
	}
}

func TestFreshestValid_NoneQualifies(t *testing.T) {
	now := time.Now()
	leaves := []*x509.Certificate{leaf("Other CA", -1*time.Hour, time.Hour, now)}
	if got := freshestValid(now, leaves, issuerIn([]string{"SLS CA", "Second CA"})); got != -1 {
		t.Fatalf("expected -1, got %d", got)
	}
	if got := freshestValid(now, nil, issuerIn(nil)); got != -1 {
		t.Fatalf("expected -1 for no candidates, got %d", got)
	}
}

func TestIssuerIn_MatchesAnyOfSeveral(t *testing.T) {
	now := time.Now()
	pred := issuerIn([]string{"SAP PKI Certificate Service Client CA", "Zalaris User CA"})
	if !pred(leaf("Zalaris User CA", -time.Hour, time.Hour, now)) {
		t.Fatal("second issuer should match")
	}
	if pred(leaf("Zalaris User CA ", -time.Hour, time.Hour, now)) {
		t.Fatal("issuer match must be exact")
	}
}
