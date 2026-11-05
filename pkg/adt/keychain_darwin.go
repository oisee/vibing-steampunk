//go:build darwin

package adt

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"strings"
	"time"
)

// LoadKeychainClientCert finds a valid keychain identity whose leaf certificate
// Subject Common Name equals cn. Use this to pin a specific user's cert.
func LoadKeychainClientCert(cn string) (*tls.Certificate, error) {
	return loadKeychainIdentity(
		fmt.Sprintf("Subject CN=%q", cn),
		func(c *x509.Certificate) bool { return c.Subject.CommonName == cn },
	)
}

// LoadKeychainClientCertByIssuer finds a valid keychain identity whose leaf
// certificate was issued by an issuer with the given Common Name (the freshest,
// if several match). This lets a shared config select "the SLC/IAS login cert"
// generically — each user's own cert is picked without a per-user CN.
func LoadKeychainClientCertByIssuer(issuerCN string) (*tls.Certificate, error) {
	return LoadKeychainClientCertByIssuers([]string{issuerCN})
}

// LoadKeychainClientCertByIssuers is LoadKeychainClientCertByIssuer for a set
// of acceptable issuer CNs (org fleets often have more than one Secure Login
// Server / CA in play). The freshest valid identity across all of them wins.
func LoadKeychainClientCertByIssuers(issuerCNs []string) (*tls.Certificate, error) {
	return loadKeychainIdentity(
		fmt.Sprintf("Issuer CN in %q", issuerCNs),
		issuerIn(issuerCNs),
	)
}

// loadKeychainIdentity returns the freshest currently-valid keychain identity
// matching pred as a *tls.Certificate: the leaf, the intermediates the
// keychain holds for it, and a signer whose private key stays in the
// keychain. The returned certificate owns its keychain handles (see
// keychainSigner); nothing here outlives the call. Caller pins TLS 1.2 (see
// Config.tlsClientConfig).
func loadKeychainIdentity(desc string, pred func(*x509.Certificate) bool) (*tls.Certificate, error) {
	sec, err := loadSecurity()
	if err != nil {
		return nil, fmt.Errorf("open macOS keychain: %w", err)
	}
	idents, err := sec.identities()
	if err != nil {
		return nil, fmt.Errorf("list keychain identities: %w", err)
	}

	now := time.Now()
	leaves := make([]*x509.Certificate, len(idents))
	var seen []string // what IS in the keychain, for the no-match error
	for i, id := range idents {
		crt, err := sec.identityCertificate(id)
		if err != nil {
			continue // leaves[i] stays nil and freshestValid skips it
		}
		leaves[i] = crt
		if len(seen) < 10 {
			state := "valid"
			if now.After(crt.NotAfter) {
				state = "EXPIRED " + crt.NotAfter.Format("2006-01-02 15:04")
			}
			seen = append(seen, fmt.Sprintf("Subject CN=%q Issuer CN=%q (%s)",
				crt.Subject.CommonName, crt.Issuer.CommonName, state))
		}
	}

	// The rule itself lives in keychain_select.go, where it can be tested
	// without a keychain; this file only owns the Security.framework handles.
	pick := freshestValid(now, leaves, pred)
	for i, id := range idents {
		if i != pick {
			sec.release(id)
		}
	}
	if pick < 0 {
		detail := "keychain has no identities at all"
		if len(seen) > 0 {
			detail = "keychain identities present: " + strings.Join(seen, "; ")
		}
		return nil, fmt.Errorf("no valid (unexpired) keychain identity matching %s found — open SLC and log in (wrong SLS profile? %s)", desc, detail)
	}

	ident, leaf := idents[pick], leaves[pick]
	key, err := sec.identityPrivateKey(ident)
	if err != nil {
		sec.release(ident)
		return nil, fmt.Errorf("keychain private key for %s: %w (grant access when prompted)", desc, err)
	}

	// Intermediates are best-effort: a leaf alone still authenticates against
	// a PSE that holds the whole chain.
	pool, _ := sec.certificates()
	cert := &tls.Certificate{PrivateKey: newKeychainSigner(sec, leaf.PublicKey, ident, key), Leaf: leaf}
	for _, c := range chainFor(leaf, pool) {
		cert.Certificate = append(cert.Certificate, c.Raw)
	}
	return cert, nil
}
