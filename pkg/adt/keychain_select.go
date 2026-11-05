package adt

import (
	"bytes"
	"crypto/x509"
	"time"
)

// issuerIn matches a leaf certificate whose issuer Common Name is one of cns.
// Org fleets often have more than one Secure Login Server or CA in play, so
// the shared config names all of them and each user's own certificate matches
// without a per-user Subject CN.
func issuerIn(cns []string) func(*x509.Certificate) bool {
	return func(c *x509.Certificate) bool {
		for _, cn := range cns {
			if c.Issuer.CommonName == cn {
				return true
			}
		}
		return false
	}
}

// freshestValid picks, among leaves, the one that matches pred and is valid at
// now, preferring the latest NotBefore: the most recently issued certificate is
// the one a daily re-login has just produced, and an older sibling that is
// still valid is the one about to be revoked. It returns -1 when nothing
// qualifies. nil entries are skipped, so callers can keep the slice aligned
// with the keystore identities the leaves came from.
func freshestValid(now time.Time, leaves []*x509.Certificate, pred func(*x509.Certificate) bool) int {
	best := -1
	for i, c := range leaves {
		if c == nil || !pred(c) || now.Before(c.NotBefore) || now.After(c.NotAfter) {
			continue
		}
		if best < 0 || c.NotBefore.After(leaves[best].NotBefore) {
			best = i
		}
	}
	return best
}

// chainFor returns leaf followed by its issuers found in pool, nearest first,
// stopping before a self-signed root, which the server holds anyway. The
// keychain keeps the SLS intermediates next to the identity, and sending
// them lets an ICM whose PSE trusts only the root still verify the client.
// A candidate counts only if it actually signed the previous link, so two
// CAs sharing a name cannot be confused.
func chainFor(leaf *x509.Certificate, pool []*x509.Certificate) []*x509.Certificate {
	chain := []*x509.Certificate{leaf}
	for cur := leaf; len(chain) < 8; {
		var issuer *x509.Certificate
		for _, c := range pool {
			if c != cur && bytes.Equal(c.RawSubject, cur.RawIssuer) && cur.CheckSignatureFrom(c) == nil {
				issuer = c
				break
			}
		}
		if issuer == nil || bytes.Equal(issuer.RawSubject, issuer.RawIssuer) {
			break
		}
		chain = append(chain, issuer)
		cur = issuer
	}
	return chain
}
