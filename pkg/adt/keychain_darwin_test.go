//go:build darwin

package adt

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"math/big"
	"testing"

	"github.com/ebitengine/purego"
)

// ephemeralKey makes a throwaway key inside Security.framework. It is never
// stored in a keychain, so the signer can be exercised on any Mac without a
// keychain item and without an access prompt. Returns the retained SecKeyRef
// and the framework's external representation of the public half.
func ephemeralKey(t *testing.T, sec *security, keyType string, bits int32) (secRef, []byte) {
	t.Helper()
	var createRandomKey func(params secRef, err *secRef) secRef
	var copyPublicKey func(key secRef) secRef
	var copyExternal func(key secRef, err *secRef) secRef
	var numberCreate func(alloc secRef, typ int, val *int32) secRef
	purego.RegisterLibFunc(&createRandomKey, sec.secLib, "SecKeyCreateRandomKey")
	purego.RegisterLibFunc(&copyPublicKey, sec.secLib, "SecKeyCopyPublicKey")
	purego.RegisterLibFunc(&copyExternal, sec.secLib, "SecKeyCopyExternalRepresentation")
	purego.RegisterLibFunc(&numberCreate, sec.cfLib, "CFNumberCreate")

	num := numberCreate(0, kCFNumberSInt32Type, &bits)
	defer sec.release(num)
	attrs := sec.dictionary(
		[]secRef{sec.constant(sec.secLib, "kSecAttrKeyType"), sec.constant(sec.secLib, "kSecAttrKeySizeInBits")},
		[]secRef{sec.constant(sec.secLib, keyType), num},
	)
	defer sec.release(attrs)

	var cfErr secRef
	key := createRandomKey(attrs, &cfErr)
	if key == 0 {
		t.Fatalf("SecKeyCreateRandomKey: %s", sec.errorString(cfErr))
	}
	pub := copyPublicKey(key)
	defer sec.release(pub)
	rep := copyExternal(pub, &cfErr)
	if rep == 0 {
		t.Fatalf("SecKeyCopyExternalRepresentation: %s", sec.errorString(cfErr))
	}
	defer sec.release(rep)
	return key, sec.dataBytes(rep)
}

// ephemeralECKey: the external representation of an EC key is X9.63,
// 04 || X || Y.
func ephemeralECKey(t *testing.T, sec *security) (secRef, *ecdsa.PublicKey) {
	t.Helper()
	key, raw := ephemeralKey(t, sec, "kSecAttrKeyTypeECSECPrimeRandom", 256)
	return key, &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(raw[1:33]),
		Y:     new(big.Int).SetBytes(raw[33:65]),
	}
}

// ephemeralRSAKey: the external representation of an RSA key is PKCS#1.
func ephemeralRSAKey(t *testing.T, sec *security) (secRef, *rsa.PublicKey) {
	t.Helper()
	key, raw := ephemeralKey(t, sec, "kSecAttrKeyTypeRSA", 2048)
	pub, err := x509.ParsePKCS1PublicKey(raw)
	if err != nil {
		t.Fatalf("parse RSA public key: %v", err)
	}
	return key, pub
}

func loadSecurityT(t *testing.T) *security {
	t.Helper()
	sec, err := loadSecurity()
	if err != nil {
		t.Fatal(err)
	}
	return sec
}

func TestKeychainSigner_SignsECDSADigestInSecurityFramework(t *testing.T) {
	sec, err := loadSecurity()
	if err != nil {
		t.Fatal(err)
	}
	key, pub := ephemeralECKey(t, sec)
	signer := newKeychainSigner(sec, pub, 0, key)

	digest := sha256.Sum256([]byte("signed by Security.framework, verified by Go"))
	sig, err := signer.Sign(rand.Reader, digest[:], crypto.SHA256)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if !ecdsa.VerifyASN1(pub, digest[:], sig) {
		t.Fatal("signature made by Security.framework does not verify against its own public key")
	}
	if signer.Public() != pub {
		t.Fatal("Public() must return the key the signer was built with")
	}
}

func TestKeychainSigner_SignsRSAPKCS1v15Digest(t *testing.T) {
	sec := loadSecurityT(t)
	key, pub := ephemeralRSAKey(t, sec)
	signer := newKeychainSigner(sec, pub, 0, key)

	digest := sha256.Sum256([]byte("PKCS#1 v1.5, the TLS 1.2 default"))
	sig, err := signer.Sign(rand.Reader, digest[:], crypto.SHA256)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig); err != nil {
		t.Fatalf("PKCS#1 v1.5 signature does not verify: %v", err)
	}
}

func TestKeychainSigner_SignsRSAPSSDigest(t *testing.T) {
	sec := loadSecurityT(t)
	key, pub := ephemeralRSAKey(t, sec)
	signer := newKeychainSigner(sec, pub, 0, key)

	// Exactly the options crypto/tls passes for rsa_pss_rsae_sha256.
	opts := &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash, Hash: crypto.SHA256}
	digest := sha256.Sum256([]byte("RSA-PSS, which TLS 1.2 may negotiate"))
	sig, err := signer.Sign(rand.Reader, digest[:], opts)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if err := rsa.VerifyPSS(pub, crypto.SHA256, digest[:], sig, opts); err != nil {
		t.Fatalf("PSS signature does not verify: %v", err)
	}
}

// Every algorithm name the mapping can produce must resolve in the framework,
// or a typo would only surface at a real handshake with that hash.
func TestSecSignatureAlgorithm_NamesResolveInFramework(t *testing.T) {
	sec := loadSecurityT(t)
	pss := func(h crypto.Hash) crypto.SignerOpts { return &rsa.PSSOptions{Hash: h} }
	cases := []struct {
		pub  crypto.PublicKey
		opts crypto.SignerOpts
		want string
	}{
		{&ecdsa.PublicKey{}, crypto.SHA1, "kSecKeyAlgorithmECDSASignatureDigestX962SHA1"},
		{&ecdsa.PublicKey{}, crypto.SHA256, "kSecKeyAlgorithmECDSASignatureDigestX962SHA256"},
		{&ecdsa.PublicKey{}, crypto.SHA384, "kSecKeyAlgorithmECDSASignatureDigestX962SHA384"},
		{&ecdsa.PublicKey{}, crypto.SHA512, "kSecKeyAlgorithmECDSASignatureDigestX962SHA512"},
		{&rsa.PublicKey{}, crypto.SHA1, "kSecKeyAlgorithmRSASignatureDigestPKCS1v15SHA1"},
		{&rsa.PublicKey{}, crypto.SHA256, "kSecKeyAlgorithmRSASignatureDigestPKCS1v15SHA256"},
		{&rsa.PublicKey{}, crypto.SHA384, "kSecKeyAlgorithmRSASignatureDigestPKCS1v15SHA384"},
		{&rsa.PublicKey{}, crypto.SHA512, "kSecKeyAlgorithmRSASignatureDigestPKCS1v15SHA512"},
		{&rsa.PublicKey{}, pss(crypto.SHA256), "kSecKeyAlgorithmRSASignatureDigestPSSSHA256"},
		{&rsa.PublicKey{}, pss(crypto.SHA384), "kSecKeyAlgorithmRSASignatureDigestPSSSHA384"},
		{&rsa.PublicKey{}, pss(crypto.SHA512), "kSecKeyAlgorithmRSASignatureDigestPSSSHA512"},
	}
	for _, c := range cases {
		got, err := secSignatureAlgorithm(c.pub, c.opts)
		if err != nil {
			t.Errorf("%T/%v: %v", c.pub, c.opts.HashFunc(), err)
			continue
		}
		if got != c.want {
			t.Errorf("%T/%v: got %s, want %s", c.pub, c.opts.HashFunc(), got, c.want)
			continue
		}
		if sec.constant(sec.secLib, got) == 0 {
			t.Errorf("%s does not resolve in Security.framework", got)
		}
	}
}

func TestSecSignatureAlgorithm_RejectsWhatTLSNeverAsksFor(t *testing.T) {
	if _, err := secSignatureAlgorithm(&rsa.PublicKey{}, crypto.MD5); err == nil {
		t.Error("MD5 must be rejected")
	}
	if _, err := secSignatureAlgorithm(ed25519.PublicKey{}, crypto.SHA256); err == nil {
		t.Error("Ed25519 keys must be rejected: the framework has no digest-signing algorithm for them")
	}
}
