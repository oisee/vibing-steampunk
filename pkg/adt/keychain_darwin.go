//go:build darwin

package adt

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/github/smimesign/certstore"
)

// keychainKeepAlive holds the certstore store and matched identity behind the
// certificate currently handed out. The in-place signer references live
// Security.framework handles; letting the store/identity be garbage-collected
// (or closed) would invalidate the signer, so they are kept for as long as
// that certificate is the one in use. A refresh (the SLC leaf rolls roughly
// daily) swaps in the new pair and closes the old one, instead of leaking a
// store handle per reload as the first version did.
var keychainKeepAlive struct {
	mu    sync.Mutex
	store certstore.Store
	ident certstore.Identity
}

// retainKeychainIdentity makes store/ident the pair kept alive and releases
// the previous pair. The swap happens before the release so there is never a
// moment with nothing retained.
func retainKeychainIdentity(store certstore.Store, ident certstore.Identity) {
	keychainKeepAlive.mu.Lock()
	prevStore, prevIdent := keychainKeepAlive.store, keychainKeepAlive.ident
	keychainKeepAlive.store, keychainKeepAlive.ident = store, ident
	keychainKeepAlive.mu.Unlock()
	if prevIdent != nil {
		prevIdent.Close()
	}
	if prevStore != nil {
		prevStore.Close()
	}
}

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
// matching pred, as a *tls.Certificate (full chain + in-place signer; the key
// never leaves the keychain). Caller pins TLS 1.2 (see Config.tlsClientConfig).
func loadKeychainIdentity(desc string, pred func(*x509.Certificate) bool) (*tls.Certificate, error) {
	store, err := certstore.Open()
	if err != nil {
		return nil, fmt.Errorf("open macOS keychain: %w", err)
	}
	idents, err := store.Identities()
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("list keychain identities: %w", err)
	}

	now := time.Now()
	leaves := make([]*x509.Certificate, len(idents))
	var seen []string // what IS in the keychain, for the no-match error
	for i, id := range idents {
		crt, err := id.Certificate()
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
	var best certstore.Identity
	var bestLeaf *x509.Certificate
	for i, id := range idents {
		if i == pick {
			best, bestLeaf = id, leaves[i]
			continue
		}
		id.Close()
	}

	if best == nil {
		store.Close()
		detail := "keychain has no identities at all"
		if len(seen) > 0 {
			detail = "keychain identities present: " + strings.Join(seen, "; ")
		}
		return nil, fmt.Errorf("no valid (unexpired) keychain identity matching %s found — open SLC and log in (wrong SLS profile? %s)", desc, detail)
	}

	chain, err := best.CertificateChain()
	if err != nil || len(chain) == 0 {
		chain = []*x509.Certificate{bestLeaf}
	}
	signer, err := best.Signer()
	if err != nil {
		best.Close()
		store.Close()
		return nil, fmt.Errorf("keychain signer for %s: %w (grant access when prompted)", desc, err)
	}

	cert := &tls.Certificate{PrivateKey: signer, Leaf: bestLeaf}
	for _, c := range chain {
		cert.Certificate = append(cert.Certificate, c.Raw)
	}

	// keep the store + matched identity alive while this cert is in use
	retainKeychainIdentity(store, best)
	return cert, nil
}
