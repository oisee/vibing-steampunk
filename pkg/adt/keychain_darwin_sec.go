//go:build darwin

package adt

// Security.framework and CoreFoundation are reached through purego, i.e.
// dlopen + dlsym at run time with no cgo. The released binaries are
// cross-compiled with CGO_ENABLED=0, and this is what lets them still use the
// keychain. Every function here is public Apple API that has existed since
// macOS 10.7 or earlier.

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// secRef is any CoreFoundation or Security object reference (CFTypeRef and
// its subtypes). Ownership follows Apple's Create/Copy rule: whatever comes
// back from a function with Create or Copy in its name belongs to the caller
// and must be released; everything else is borrowed.
type secRef = uintptr

const (
	errSecItemNotFound    = -25300
	kCFStringEncodingUTF8 = 0x08000100
	kCFNumberSInt32Type   = 3
)

const (
	coreFoundationPath = "/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation"
	securityPath       = "/System/Library/Frameworks/Security.framework/Security"
)

// security is the loaded pair of frameworks plus the handful of entry points
// the keychain loader and signer need.
type security struct {
	cfLib, secLib uintptr

	cfDictionaryCreate     func(alloc secRef, keys, vals *secRef, n int, keyCB, valueCB uintptr) secRef
	cfArrayGetCount        func(arr secRef) int
	cfArrayGetValueAtIndex func(arr secRef, i int) secRef
	cfDataCreate           func(alloc secRef, p *byte, n int) secRef
	cfDataGetLength        func(d secRef) int
	cfDataGetBytePtr       func(d secRef) *byte
	cfStringGetCString     func(s secRef, buf *byte, n int, encoding uint32) bool
	cfErrorCopyDescription func(e secRef) secRef
	cfRetain               func(r secRef) secRef
	cfRelease              func(r secRef)

	secItemCopyMatching        func(query secRef, result *secRef) int32
	secIdentityCopyCertificate func(ident secRef, cert *secRef) int32
	secIdentityCopyPrivateKey  func(ident secRef, key *secRef) int32
	secCertificateCopyData     func(cert secRef) secRef
	secKeyCreateSignature      func(key, algorithm, data secRef, err *secRef) secRef
	secCopyErrorMessageString  func(status int32, reserved uintptr) secRef

	dictKeyCallBacks, dictValueCallBacks uintptr
}

var (
	securityOnce sync.Once
	securityInst *security
	securityErr  error
)

// loadSecurity opens the frameworks once per process.
func loadSecurity() (*security, error) {
	securityOnce.Do(func() { securityInst, securityErr = openSecurity() })
	return securityInst, securityErr
}

func openSecurity() (s *security, err error) {
	cf, err := purego.Dlopen(coreFoundationPath, purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, fmt.Errorf("load CoreFoundation: %w", err)
	}
	sec, err := purego.Dlopen(securityPath, purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, fmt.Errorf("load Security.framework: %w", err)
	}
	// RegisterLibFunc panics on a missing symbol; turn that into an error so
	// a broken framework surfaces as "keychain unavailable", not a crash.
	defer func() {
		if r := recover(); r != nil {
			s, err = nil, fmt.Errorf("bind Security.framework: %v", r)
		}
	}()
	s = &security{cfLib: cf, secLib: sec}
	purego.RegisterLibFunc(&s.cfDictionaryCreate, cf, "CFDictionaryCreate")
	purego.RegisterLibFunc(&s.cfArrayGetCount, cf, "CFArrayGetCount")
	purego.RegisterLibFunc(&s.cfArrayGetValueAtIndex, cf, "CFArrayGetValueAtIndex")
	purego.RegisterLibFunc(&s.cfDataCreate, cf, "CFDataCreate")
	purego.RegisterLibFunc(&s.cfDataGetLength, cf, "CFDataGetLength")
	purego.RegisterLibFunc(&s.cfDataGetBytePtr, cf, "CFDataGetBytePtr")
	purego.RegisterLibFunc(&s.cfStringGetCString, cf, "CFStringGetCString")
	purego.RegisterLibFunc(&s.cfErrorCopyDescription, cf, "CFErrorCopyDescription")
	purego.RegisterLibFunc(&s.cfRetain, cf, "CFRetain")
	purego.RegisterLibFunc(&s.cfRelease, cf, "CFRelease")
	purego.RegisterLibFunc(&s.secItemCopyMatching, sec, "SecItemCopyMatching")
	purego.RegisterLibFunc(&s.secIdentityCopyCertificate, sec, "SecIdentityCopyCertificate")
	purego.RegisterLibFunc(&s.secIdentityCopyPrivateKey, sec, "SecIdentityCopyPrivateKey")
	purego.RegisterLibFunc(&s.secCertificateCopyData, sec, "SecCertificateCopyData")
	purego.RegisterLibFunc(&s.secKeyCreateSignature, sec, "SecKeyCreateSignature")
	purego.RegisterLibFunc(&s.secCopyErrorMessageString, sec, "SecCopyErrorMessageString")
	s.dictKeyCallBacks = s.symbol(cf, "kCFTypeDictionaryKeyCallBacks")
	s.dictValueCallBacks = s.symbol(cf, "kCFTypeDictionaryValueCallBacks")
	return s, nil
}

// symbol is the address of an exported variable.
func (s *security) symbol(lib uintptr, name string) uintptr {
	addr, err := purego.Dlsym(lib, name)
	if err != nil {
		panic(fmt.Sprintf("%s: %v", name, err))
	}
	return addr
}

// constant reads an exported `const CFStringRef kFoo`: dlsym yields the
// address of the variable, and the reference is the value stored there. The
// address is C memory, so the read goes through a pointer to the local
// variable holding it rather than a uintptr conversion vet would reject.
func (s *security) constant(lib uintptr, name string) secRef {
	addr := s.symbol(lib, name)
	return **(**secRef)(unsafe.Pointer(&addr))
}

func (s *security) dictionary(keys, vals []secRef) secRef {
	return s.cfDictionaryCreate(0, &keys[0], &vals[0], len(keys), s.dictKeyCallBacks, s.dictValueCallBacks)
}

func (s *security) retain(r secRef) secRef { return s.cfRetain(r) }

func (s *security) release(r secRef) {
	if r != 0 {
		s.cfRelease(r)
	}
}

// dataBytes copies a CFData's bytes into Go memory, so the CFData can be
// released straight after.
func (s *security) dataBytes(d secRef) []byte {
	n := s.cfDataGetLength(d)
	if n == 0 {
		return nil
	}
	return bytes.Clone(unsafe.Slice(s.cfDataGetBytePtr(d), n))
}

func (s *security) stringValue(str secRef) string {
	if str == 0 {
		return ""
	}
	buf := make([]byte, 1024)
	if !s.cfStringGetCString(str, &buf[0], len(buf), kCFStringEncodingUTF8) {
		return ""
	}
	if i := bytes.IndexByte(buf, 0); i >= 0 {
		buf = buf[:i]
	}
	return string(buf)
}

// errorString describes a CFErrorRef; it does not release it.
func (s *security) errorString(cfErr secRef) string {
	if cfErr == 0 {
		return "unknown Security.framework error"
	}
	desc := s.cfErrorCopyDescription(cfErr)
	defer s.release(desc)
	return s.stringValue(desc)
}

func (s *security) statusError(status int32) error {
	msg := s.secCopyErrorMessageString(status, 0)
	defer s.release(msg)
	return fmt.Errorf("%s (OSStatus %d)", s.stringValue(msg), status)
}

// sign asks the framework to sign an already-hashed digest with key, using
// the named kSecKeyAlgorithm constant. The key never leaves the keychain;
// this is where macOS shows its access prompt when the key's ACL asks for
// one.
func (s *security) sign(key secRef, algorithm string, digest []byte) ([]byte, error) {
	data := s.cfDataCreate(0, &digest[0], len(digest))
	defer s.release(data)
	var cfErr secRef
	sig := s.secKeyCreateSignature(key, s.constant(s.secLib, algorithm), data, &cfErr)
	if sig == 0 {
		defer s.release(cfErr)
		return nil, errors.New(s.errorString(cfErr))
	}
	defer s.release(sig)
	return s.dataBytes(sig), nil
}

// keychainSigner is the crypto.Signer behind a keychain-backed
// tls.Certificate. It owns the retained identity and key references, and
// they are released only once the signer itself is unreachable. There is no
// package-level state: a certificate still held by an in-flight handshake,
// or by another system's client in the same process, keeps its own key.
type keychainSigner struct {
	sec  *security
	pub  crypto.PublicKey
	refs *secRefs
}

type secRefs struct{ ident, key secRef }

func newKeychainSigner(sec *security, pub crypto.PublicKey, ident, key secRef) *keychainSigner {
	s := &keychainSigner{sec: sec, pub: pub, refs: &secRefs{ident: ident, key: key}}
	runtime.AddCleanup(s, func(r *secRefs) {
		sec.release(r.key)
		sec.release(r.ident)
	}, s.refs)
	return s
}

func (s *keychainSigner) Public() crypto.PublicKey { return s.pub }

func (s *keychainSigner) Sign(_ io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	alg, err := secSignatureAlgorithm(s.pub, opts)
	if err != nil {
		return nil, err
	}
	sig, err := s.sec.sign(s.refs.key, alg, digest)
	runtime.KeepAlive(s)
	return sig, err
}

// secSignatureAlgorithm names the kSecKeyAlgorithm constant for a key type,
// hash and padding. Go hands the signer an already-hashed digest, so only the
// "Digest" family applies.
func secSignatureAlgorithm(pub crypto.PublicKey, opts crypto.SignerOpts) (string, error) {
	var hash string
	switch opts.HashFunc() {
	case crypto.SHA1:
		hash = "SHA1"
	case crypto.SHA256:
		hash = "SHA256"
	case crypto.SHA384:
		hash = "SHA384"
	case crypto.SHA512:
		hash = "SHA512"
	default:
		return "", fmt.Errorf("keychain signer: unsupported hash %v", opts.HashFunc())
	}
	switch pub.(type) {
	case *ecdsa.PublicKey:
		return "kSecKeyAlgorithmECDSASignatureDigestX962" + hash, nil
	case *rsa.PublicKey:
		// crypto/tls signs with PSSOptions for rsa_pss_rsae_*, plain Hash
		// otherwise. The framework's PSS salt length equals the hash length,
		// which is what TLS requires.
		if _, pss := opts.(*rsa.PSSOptions); pss {
			return "kSecKeyAlgorithmRSASignatureDigestPSS" + hash, nil
		}
		return "kSecKeyAlgorithmRSASignatureDigestPKCS1v15" + hash, nil
	}
	return "", fmt.Errorf("keychain signer: unsupported key type %T", pub)
}

// identityCertificate parses the leaf certificate of a keychain identity.
func (s *security) identityCertificate(ident secRef) (*x509.Certificate, error) {
	var cert secRef
	if status := s.secIdentityCopyCertificate(ident, &cert); status != 0 {
		return nil, s.statusError(status)
	}
	defer s.release(cert)
	data := s.secCertificateCopyData(cert)
	defer s.release(data)
	return x509.ParseCertificate(s.dataBytes(data))
}

// items lists every keychain item of the given kSecClass, each retained for
// the caller.
func (s *security) items(class string) ([]secRef, error) {
	query := s.dictionary(
		[]secRef{s.constant(s.secLib, "kSecClass"), s.constant(s.secLib, "kSecReturnRef"), s.constant(s.secLib, "kSecMatchLimit")},
		[]secRef{s.constant(s.secLib, class), s.constant(s.cfLib, "kCFBooleanTrue"), s.constant(s.secLib, "kSecMatchLimitAll")},
	)
	defer s.release(query)
	var result secRef
	switch status := s.secItemCopyMatching(query, &result); status {
	case 0:
	case errSecItemNotFound:
		return nil, nil
	default:
		return nil, s.statusError(status)
	}
	defer s.release(result)
	n := s.cfArrayGetCount(result)
	refs := make([]secRef, 0, n)
	for i := 0; i < n; i++ {
		refs = append(refs, s.retain(s.cfArrayGetValueAtIndex(result, i)))
	}
	return refs, nil
}

// identities lists the keychain identities (certificate + private key pairs),
// each retained for the caller.
func (s *security) identities() ([]secRef, error) { return s.items("kSecClassIdentity") }

// certificates parses every certificate in the keychain search list. This is
// the pool chainFor draws intermediates from.
func (s *security) certificates() ([]*x509.Certificate, error) {
	refs, err := s.items("kSecClassCertificate")
	if err != nil {
		return nil, err
	}
	certs := make([]*x509.Certificate, 0, len(refs))
	for _, ref := range refs {
		data := s.secCertificateCopyData(ref)
		if c, err := x509.ParseCertificate(s.dataBytes(data)); err == nil {
			certs = append(certs, c)
		}
		s.release(data)
		s.release(ref)
	}
	return certs, nil
}

// identityPrivateKey returns the identity's private key, retained for the
// caller. This does not touch the key material and does not prompt; the
// access prompt, if the key's ACL wants one, comes at the first signature.
func (s *security) identityPrivateKey(ident secRef) (secRef, error) {
	var key secRef
	if status := s.secIdentityCopyPrivateKey(ident, &key); status != 0 {
		return 0, s.statusError(status)
	}
	return key, nil
}
