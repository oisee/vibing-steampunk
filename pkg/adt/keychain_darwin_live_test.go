//go:build darwin

package adt

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"os"
	"testing"
)

// TestLoadKeychainClientCertByIssuer_Live runs only against a real keychain:
//
//	VSP_TEST_KEYCHAIN_ISSUER="SAP PKI Certificate Service Client CA" go test ./pkg/adt -run Live -v
//
// It loads the identity but never signs with it, so it does not trigger the
// keychain access prompt.
func TestLoadKeychainClientCertByIssuer_Live(t *testing.T) {
	issuer := os.Getenv("VSP_TEST_KEYCHAIN_ISSUER")
	if issuer == "" {
		t.Skip("set VSP_TEST_KEYCHAIN_ISSUER to run against the login keychain")
	}

	cert, err := LoadKeychainClientCertByIssuer(issuer)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cert.Leaf == nil {
		t.Fatal("Leaf must be parsed")
	}
	if cert.Leaf.Issuer.CommonName != issuer {
		t.Errorf("leaf issuer %q, want %q", cert.Leaf.Issuer.CommonName, issuer)
	}
	if len(cert.Certificate) == 0 || !bytes.Equal(cert.Certificate[0], cert.Leaf.Raw) {
		t.Error("Certificate[0] must be the leaf's DER")
	}
	signer, ok := cert.PrivateKey.(crypto.Signer)
	if !ok {
		t.Fatalf("PrivateKey is %T, want a crypto.Signer", cert.PrivateKey)
	}
	if eq, ok := signer.Public().(interface{ Equal(crypto.PublicKey) bool }); !ok || !eq.Equal(cert.Leaf.PublicKey) {
		t.Error("signer's public key must be the leaf's")
	}
	// The chain must be the leaf's real issuers, nearest first, and stop
	// before the root: the server holds its own trust anchors.
	chain := make([]*x509.Certificate, len(cert.Certificate))
	for i, der := range cert.Certificate {
		c, err := x509.ParseCertificate(der)
		if err != nil {
			t.Fatalf("chain[%d]: %v", i, err)
		}
		chain[i] = c
		t.Logf("chain[%d]: CN=%q", i, c.Subject.CommonName)
	}
	if len(chain) < 2 {
		t.Errorf("expected the SLS intermediate(s) after the leaf, got %d certificate(s)", len(chain))
	}
	for i := 1; i < len(chain); i++ {
		if err := chain[i-1].CheckSignatureFrom(chain[i]); err != nil {
			t.Errorf("chain[%d] did not sign chain[%d]: %v", i, i-1, err)
		}
	}
	if last := chain[len(chain)-1]; bytes.Equal(last.RawSubject, last.RawIssuer) {
		t.Errorf("chain ends in the self-signed root %q; roots are not sent", last.Subject.CommonName)
	}
}
