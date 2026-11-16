package adt

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
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
	pred := issuerIn([]string{"SAP PKI Certificate Service Client CA", "Example Corp User CA"})
	if !pred(leaf("Example Corp User CA", -time.Hour, time.Hour, now)) {
		t.Fatal("second issuer should match")
	}
	if pred(leaf("Example Corp User CA ", -time.Hour, time.Hour, now)) {
		t.Fatal("issuer match must be exact")
	}
}

// mintChainCert signs a certificate for subject with parent's key (or its own
// when parent is nil, i.e. a root). Real signatures, so chainFor's signature
// check is exercised rather than mocked.
func mintChainCert(t *testing.T, subject string, isCA bool, parent *x509.Certificate, parentKey *ecdsa.PrivateKey) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: subject},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  isCA,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
	}
	if parent == nil {
		parent, parentKey = tmpl, key
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, &key.PublicKey, parentKey)
	if err != nil {
		t.Fatal(err)
	}
	crt, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return crt, key
}

func TestChainFor_AddsIntermediatesFromPoolAndOmitsRoot(t *testing.T) {
	root, rootKey := mintChainCert(t, "SAP Cloud Root CA", true, nil, nil)
	inter, interKey := mintChainCert(t, "SAP PKI Certificate Service Client CA", true, root, rootKey)
	leaf, _ := mintChainCert(t, "DEVELOPER", false, inter, interKey)
	other, _ := mintChainCert(t, "Unrelated CA", true, nil, nil)

	got := chainFor(leaf, []*x509.Certificate{other, root, inter})

	want := []*x509.Certificate{leaf, inter}
	if len(got) != len(want) {
		t.Fatalf("chain length %d, want %d (leaf + intermediate, root omitted)", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("chain[%d] = %q, want %q", i, got[i].Subject.CommonName, want[i].Subject.CommonName)
		}
	}
}

func TestChainFor_LeafAloneWhenIssuerNotInPool(t *testing.T) {
	inter, interKey := mintChainCert(t, "Intermediate", true, nil, nil)
	leaf, _ := mintChainCert(t, "DEVELOPER", false, inter, interKey)

	got := chainFor(leaf, nil)

	if len(got) != 1 || got[0] != leaf {
		t.Fatalf("expected just the leaf, got %d certificates", len(got))
	}
}

func TestChainFor_SkipsSameNameIssuerWithWrongKey(t *testing.T) {
	inter, interKey := mintChainCert(t, "Intermediate", true, nil, nil)
	impostor, _ := mintChainCert(t, "Intermediate", true, nil, nil) // same name, different key
	leaf, _ := mintChainCert(t, "DEVELOPER", false, inter, interKey)

	got := chainFor(leaf, []*x509.Certificate{impostor})

	if len(got) != 1 {
		t.Fatalf("an issuer that did not sign the leaf must not be chained; got %d certificates", len(got))
	}
}
