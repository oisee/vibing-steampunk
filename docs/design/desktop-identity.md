# Desktop identity: Entra ID on macOS, Windows and WSL, and how it reaches SAP

*Design study, 2026-08-21. Companion to [`fork-survey.md`](fork-survey.md) (what the fork
network already built) and a separate study of that system, kept outside this repository (see the note below) (how another system
does Entra for dependent MCP servers). Slots into **Sprint 3 — Authentication** in
[`../AGENDA.md`](../AGENDA.md).*

Scope: how a developer signs in on a desktop, where the resulting secret lives, and how
that identity becomes an SAP principal — for vsp over ADT/HTTP, and for the RFC leg in
`open-rfc-go`.

---

## 0. The one-paragraph answer

**Do not put Entra in the client.** For the overwhelming majority of corporate SAP
landscapes the developer's Entra identity already reaches SAP — through the browser, via
SAML2 to Entra directly or (far more commonly) to SAP Cloud Identity Services acting as a
proxy IdP. The correct desktop sign-in is therefore the **system browser plus a loopback
redirect, redeeming an SAP reentrance ticket** — Eclipse ADT's own mechanism, MFA and
conditional access included, with zero Entra code in the binary. Where a real OAuth token
is needed (BTP/Steampunk, XSUAA), the authorization server we talk to is **XSUAA or IAS,
not Entra**, and the flow is authorization-code + PKCE on the same loopback listener, with
device code as the headless fallback. Entra tokens as such are useful to vsp in exactly
one place — nowhere on the SAP wire — and pretending otherwise produces a stack that
cannot be made to work without SAP-side configuration nobody will grant.

---

## 1. Desktop sign-in: the four candidates, honestly compared

### 1.1 System browser + loopback redirect (PKCE) — **the default**

Bind `127.0.0.1:0`, launch the user's real browser at an authorize URL whose
`redirect_uri` is that loopback port, serve one request, take the code (or the SAP
reentrance ticket) off it, shut the listener down.

Two variants share the same 120 lines of transport:

| Variant | Authorize URL | What comes back |
|---|---|---|
| **Reentrance ticket** (SAP-native) | `/sap/bc/adt/core/http/reentranceticket?redirect-url=http://127.0.0.1:PORT/adt/redirect` | `MYSAPSSO2` ticket → exchanged at `/sap/bc/adt/core/http/sessions` for a cookie jar |
| **OAuth2 code + PKCE** | XSUAA/IAS/ABAP-AS `/oauth/authorize?...&code_challenge=…&code_challenge_method=S256` | `code` → token endpoint → bearer + refresh token |

**Cost in Go: low, and mostly already paid.** `BurnerPat/vsp-enterprise:develop` has the
reentrance variant working with 15 `httptest` tests; the fork survey already recommends
taking it. For the OAuth variant, `golang.org/x/oauth2` gives you
`oauth2.GenerateVerifier()` / `S256ChallengeOption` / `VerifierOption` in the standard
config — no MSAL needed, because **the authorization server is SAP's, not Entra's**. If
you ever do need to talk to Entra directly,
`github.com/AzureAD/microsoft-authentication-library-for-go/apps/public` has
`AcquireTokenInteractive` with `WithRedirectURI` (loopback port) and `WithOpenURL`
(custom browser launcher), and `AcquireTokenByDeviceCode`; it is maintained, v1.x, and
pure Go.

**Why it beats everything else:** it uses the browser profile the developer is already
signed into. Kerberos/SPNEGO, an existing Entra web session, conditional access, device
compliance, FIDO2, a corporate TLS-inspecting proxy — all of it is the browser's problem,
not ours. This is precisely the argument the survey makes for replacing the chromedp path,
which spawns a throwaway Chromium profile that has none of that.

**Failure modes:**
- The port is in use or blocked by a local firewall → bind `:0` and never a fixed port,
  except where Entra-style redirect-URI registration demands one (Entra permits any port
  on `http://localhost` for public clients; XSUAA generally does not, and wants the exact
  redirect URI registered — so the OAuth variant may need a **fixed** port, e.g. `35729`,
  registered by the Basis/BTP admin. Say so in the config docs).
- The user closes the tab → the listener must have a timeout (60–120 s) and a
  cancellable context, and must print the URL to stderr so it can be pasted manually.
- Two vsp processes race for the same fixed port → single-flight the login through a
  lockfile, or fail with a clear message rather than a bind error.

### 1.2 Device code — **the fallback**

Poll `/devicecode`, show a `https://microsoft.com/devicelogin` + `ABCD-EFGH` pair, wait.
Works with no browser on the machine and no loopback listener at all. `MSAL Go` has
`AcquireTokenByDeviceCode`; `x/oauth2` has `DeviceAuth`/`DeviceAccessToken` for a generic
provider. Keycloak and XSUAA both expose a device endpoint; **the SAP reentrance-ticket
service does not** — there is no device-code equivalent for `MYSAPSSO2`, so on a headless
box the reentrance path is simply unavailable and the fallback has to be OAuth or a
password/credential command.

Trap worth stating loudly: **an MCP server on stdio must never print a device code to
stdout.** It would corrupt the JSON-RPC frame. All interactive prompting goes to stderr,
and the real answer is that the MCP server should never authenticate interactively at
all — see `vsp auth login` in §4.4.

### 1.3 OS brokers (WAM, ASWebAuthenticationSession) — **not available in Go**

Worth checking so nobody spends a week on it:

- **Windows WAM.** Broker support shipped in the Azure Identity libraries for .NET, Java,
  JavaScript and Python. There is **no Go binding**:
  `github.com/Azure/azure-sdk-for-go/sdk/azidentity/broker` does not exist (404 on
  pkg.go.dev), and MSAL Go's public client has no broker option. Building one means cgo
  against the MSAL native runtime DLL, which breaks the `CGO_ENABLED=0` six-platform
  release build outright. **Verdict: cannot do, do not try.**
- **macOS `ASWebAuthenticationSession`.** Objective-C/Swift API; from Go it is cgo plus an
  app bundle with a registered URL scheme. A CLI binary is not an app bundle. It buys
  nothing over the system browser, because Safari/Chrome already hold the session.
  **Verdict: no.**
- **What macOS *does* usefully offer** is the Keychain — for the secret, not the
  interaction (§2), and for client certificates via
  `github.com/github/smimesign/certstore`, where the private key stays in the keychain
  and you get a `crypto.Signer`. Edgars' patch already does this and is the single best
  piece of auth code in the fork network.

An oblique alternative that gets most of the broker's benefit for none of the cost:
**shell out to a broker-capable CLI the developer already has.** This is exactly what the
agency system does — its desktop path is not MSAL at all, it is
`azureauth aad --client … --tenant … --scope … --output token`, pinned to version 0.9.5
because 0.9.6 regressed. `az account get-access-token --resource … -o json` is the same
idea with a tool most developers already have. vsp already has the right hook for this:
`pkg/adt/credential_cmd.go` runs an argv-based (no shell) external command and parses
JSON. Generalise it from `{username,password}` to `{token,expires_on}` and you inherit
WAM, conditional access and the corporate cache for free, in about 40 lines. **Take this.**

### 1.4 WSL — the case everyone gets wrong

Three separate problems, and they have different answers.

**(a) Opening the browser.** `xdg-open` usually is not installed in a WSL rootfs. The
launcher must try, in order: `$BROWSER`, `wslview` (from `wslu`, present on most WSL
distros), `powershell.exe -NoProfile -Command Start-Process <url>`,
`cmd.exe /c start "" <url>`, `explorer.exe <url>`, then `xdg-open`. Detect WSL by reading
`/proc/sys/kernel/osrelease` for `microsoft` / `WSL` — the `WSL_DISTRO_NAME` env var is
more convenient but is absent under `systemd`-spawned services and some SSH sessions.
Note `cmd.exe`/`explorer.exe` mangle URLs containing `&` unless quoted, and
`explorer.exe` always returns exit code 1 even on success — do not treat that as failure.

**(b) Loopback back into WSL.** WSL2 is NAT'd, but `localhostForwarding=true` (the default
in `%UserProfile%\.wslconfig`) relays Windows `localhost:PORT` to a WSL2 listener, and
Windows 11's mirrored networking mode shares the stack outright. In practice
**the loopback redirect works on WSL2**, and always works on WSL1. It is not reliable
enough to assume: the relay is known to go stale after the host sleeps, and some corporate
images disable it. So bind `127.0.0.1` *and* `0.0.0.0`? No — binding `0.0.0.0` in WSL2
exposes the listener to the LAN. Bind `127.0.0.1`, and on timeout emit a message that
names the WSL case explicitly and offers `--device-code`.

**(c) No secret store.** Covered in §2.3; this is the real WSL problem, not the browser.

### 1.5 Headless SSH

No browser, no keyring, no D-Bus. Detect it — and detect it better than the agency code
does, which only recognises ADO and GitHub Actions and will happily try to spawn a browser
on an SSH session. The test should be: `SSH_CONNECTION`/`SSH_TTY` set, or no `DISPLAY`
and no `WAYLAND_DISPLAY` on Linux with no WSL interop, or `CI` set. In that case:

1. `--device-code` if the SAP-side AS supports it,
2. otherwise `credential_cmd` / env-var password,
3. otherwise `ssh -L 35729:127.0.0.1:35729` and the normal loopback flow, documented as a
   recipe rather than automated.

Never silently fall back to a password prompt on a channel that might be an MCP stdio pipe.

### 1.6 Recommendation

| | Default | Fallback |
|---|---|---|
| macOS / Windows / WSL (interactive) | **System browser + loopback**, reentrance ticket for on-prem, code+PKCE for BTP | `--device-code`, then `credential_cmd` |
| Headless / SSH / CI | `credential_cmd` (or a token command, §1.3) | `--device-code` where the AS supports it |
| Anywhere, once a cert exists | **mTLS from the OS keystore** — no interaction at all | as above |

---

## 2. Where the secret lives

Today the weakest part of the story: `.vsp.json` carries a plaintext `password`, and
`VSP_<SYSTEM>_PASSWORD` is the documented alternative. Three things must improve, in this
order: refuse a group/world-readable config, promote `credential_cmd` to the default, add
native stores.

### 2.1 macOS

Keychain. Two libraries, and the choice matters:

- `github.com/zalando/go-keyring` — no cgo, but it implements `Set` by shelling
  `security add-generic-password -w <password>`, which **puts the secret in `argv` where
  any user's `ps` can read it.** That is the same defect the fork survey flags in the JCo
  sidecar. Reads (`find-generic-password -w`) are fine.
- `github.com/keybase/go-keychain` — cgo against Security.framework, no argv exposure.
  cgo on darwin only is already forced on us if we take Edgars' `certstore` path, and
  `.goreleaser.yml`'s global `CGO_ENABLED=0` already has to be fixed for that (the survey
  notes this).

**Use `keybase/go-keychain` on darwin.** Item: service `vsp`, account `<system>`, with
`kSecAttrAccessibleWhenUnlocked` (not `…AfterFirstUnlock` — a dev laptop is either open or
it is not).

### 2.2 Windows

Credential Manager via `github.com/danieljoos/wincred` (pure Go, `syscall` only — keeps
`CGO_ENABLED=0`). **One hard limit: `CRED_MAX_CREDENTIAL_BLOB_SIZE` is 2560 bytes.** A
password fits. An OAuth token cache with a refresh token, an ID token and an account
record does not, and you will discover this as a confusing `ERROR_INVALID_PARAMETER` in
production.

So: **do not store the token cache in Credential Manager.** Store the cache as an
AES-256-GCM file under `%LOCALAPPDATA%\vsp\`, and protect it with **DPAPI**
(`CryptProtectData`, `CRYPTPROTECT_UI_FORBIDDEN`, current-user scope) — either directly on
the blob or on a 32-byte data-encryption key that the file's header carries. DPAPI is a
straight `syscall` (`github.com/billgraziano/dpapi` wraps it, or 60 lines of our own) and
is user- and machine-bound, which is exactly the property we want. Credential Manager is
still the right home for a *password*.

### 2.3 Linux and WSL

- **Linux desktop:** Secret Service over D-Bus. `zalando/go-keyring` (via
  `github.com/godbus/dbus/v5`) does this with no cgo and no libsecret linkage. Works with
  gnome-keyring and KWallet's Secret Service shim.
- **WSL and headless:** there is normally **no session D-Bus and no keyring daemon**, so
  Secret Service simply fails, and it fails at write time with a `org.freedesktop.DBus.Error`
  that means nothing to the user. Detect the absence up front
  (`DBUS_SESSION_BUS_ADDRESS` unset and no `/run/user/$UID/bus`) and do not attempt it.

The fallback has to be a file, and it has to be honest about what it is:

| Mode | What it is | When |
|---|---|---|
| `store: "file+passphrase"` | AES-256-GCM, key from argon2id over a passphrase prompted once per process | user opts in |
| `store: "file"` | mode-0600 file, **no encryption**, loud one-time warning | default fallback |
| `store: "none"` | in-memory only; re-login every process start | CI, containers, paranoid |

**Do not ship a "file" mode that derives its key from a machine ID or a constant and call
it encrypted.** A key stored beside its ciphertext is obfuscation. `99designs/keyring`
offers a `file` backend with a passphrase prompt and is a reasonable single dependency if
we want all four backends behind one interface; the trade is a heavier dep tree than
`wincred` + `go-keychain` + `go-keyring` chosen per platform.

For a long-lived MCP server, `store: "none"` is genuinely defensible: the process
authenticates once at start and holds the credential in memory for its lifetime. That is
what the agency system does — its `JwtCache` is `Mutex<Option<JwtToken>>`, in-memory,
process-scoped, with a 60-second refresh buffer and single-flight so concurrent callers
never trigger two interactive prompts. Copy that shape (§4.3).

### 2.4 Rules that apply to every store

- Nothing secret in `argv`, ever. `credential_cmd` already gets this right (argv-based
  `exec.Command`, stderr discarded, stdout zeroed).
- Nothing secret in `.vsp.json`. Refuse to load a config whose mode is group- or
  world-readable; warn whenever `password` is present at all.
- **Scrub before spawning.** When vsp execs a browser, a credential command or any child,
  strip `VSP_*_PASSWORD`, `SAP_PASSWORD` and any token env from the child environment
  unless that child is the credential command itself. The agency codebase does exactly
  this (`scrub_mcp_auth_env` removes `AGENCY_MCP_AUTH_*` before launching the agent) so
  the model it drives cannot replay the auth flow. Same reasoning applies here.
- Redact by construction: the redaction list lives next to the `Credential`
  implementations, so a new mechanism cannot forget to register its secret header.

---

## 3. How an Entra identity reaches SAP

Entra authenticates the *developer*. ADT wants an SAP principal — a `SY-UNAME` with
`S_DEVELOP`. Something must bridge, and there are only five bridges that actually exist.

### 3.1 Path A — browser SSO, no Entra tokens in vsp at all ✅ **recommended default**

vsp opens the system browser at the reentrance-ticket URL. The ICF logon procedure
redirects to the SAML2 IdP — Entra directly, or (the usual corporate shape) SAP Cloud
Identity Services / IAS federating upward to Entra. The user is already signed in, or does
MFA in a real browser. SAP issues `MYSAPSSO2`, which comes back on the loopback callback
and is exchanged for a session cookie jar.

vsp never sees an Entra token, never registers an app, never needs a client ID.

- **Where it works:** on-prem AS ABAP with SAML2 configured, and BTP ABAP
  environment/Steampunk (which is browser-SSO by design).
- **SAP-side prerequisites — the Basis ask:**
  - SAML2 SP configured (transaction `SAML2`), Entra or IAS as trusted IdP.
  - NameID → SAP user mapping (`SU01` alias / `USREXTID`) — the IdP must assert something
    that resolves to a SAP user name.
  - `login/create_sso2_ticket = 2` (or `3`), the system's own signing cert in `STRUST`.
  - `/sap/bc/adt/*` and `/sap/bc/adt/core/http/reentranceticket` active in `SICF`, with
    a logon procedure that includes SSO/SAML ahead of basic.
  - Developer authorizations: `S_DEVELOP`, `S_ADT_RES`, `S_RFC` for the ADT services.
- **Effort in vsp:** ~1 day. BurnerPat's patch, already scheduled in Sprint 3.
- **Limits:** the resulting session is a cookie session with a server-side lifetime
  (typically 8 h, or shorter under `icf/user_recheck`). Renewal means re-running the
  browser flow, which mints a **new** `SAP_SESSIONID` and therefore destroys
  `sap-contextid` continuity — see §4.5, this is the one refresh that can break a lock.

### 3.2 Path B — OAuth2 authorization code + PKCE against **XSUAA / IAS / the ABAP OAuth AS** ✅ **build this second**

For BTP and for an on-prem AS ABAP configured as an OAuth 2.0 authorization server
(`SOAUTH2`), the resource accepts `Authorization: Bearer`. The token comes from SAP's AS,
and Entra does the human login *inside* that AS's browser redirect. Nobody in the fork
network has built this — `Prolls`' work is `client_credentials` with a technical user,
which is the opposite of a developer identity.

Discovery is worth stealing from the agency design: probe the resource, take the 401, parse
`WWW-Authenticate: Bearer` per RFC 6750 §3, and follow `resource_metadata` per RFC 9728 to
find the authorization server and its scopes. That turns "which XSUAA am I talking to" from
a config field into a runtime fact. Where the header carries no `resource_metadata`, fall
back to configured values rather than guessing.

- **SAP-side prerequisites:** an OAuth client registered (`SOAUTH2` on-prem; an
  `xsuaa` service instance with `oauth2-configuration.redirect-uris` on BTP) with
  `http://localhost:35729/callback` — or whatever fixed port we settle on — in the allowed
  redirect URIs, PKCE permitted, and the ADT ICF node marked as an OAuth-protected
  resource with a scope.
- **Effort:** ~2 days on `golang.org/x/oauth2`, including refresh and proactive renewal.
- **Fixed-port caveat:** unlike Entra, XSUAA will not accept an arbitrary loopback port,
  so `:0` is not an option here. Document the port; make it configurable; fail clearly
  when it is taken.

### 3.3 Path C — SAML2 bearer assertion to the ABAP OAuth AS ❌ **cannot work from a Go desktop client**

AS ABAP genuinely supports `grant_type=urn:ietf:params:oauth:grant-type:saml2-bearer` at
`/sap/bc/sec/oauth2/token`, and it is the textbook answer to "turn an IdP identity into an
SAP OAuth token". It is unbuildable here for one reason: **Entra ID has no way to issue a
SAML assertion out of band.** Its SAML assertions exist only as the POST body of a browser
SSO response to a registered SP; there is no grant, no API, no `/saml/token` endpoint. The
only way to obtain one from a CLI is to drive the browser flow and scrape the
`SAMLResponse` out of the form — which is exactly `pkg/adt/saml_auth.go`, which the fork
survey already records as brittle and MFA-incapable.

(ADFS with WS-Trust could mint one non-interactively. It is being retired, it is not Entra,
and building for it would be building for the past.)

**Verdict: do not build it.** If a landscape needs SAML bearer, the assertion has to come
from IAS in a server-side deployment, not from a laptop.

### 3.4 Path D — principal propagation through the Cloud Connector ⚠️ **not a desktop story**

A BTP-hosted app forwards the user's JWT to a `PrincipalPropagation` destination; the
Connectivity service and Cloud Connector mint a short-lived X.509 with the user's identity,
and the on-prem ABAP maps it via `CERTRULE`. This is the correct architecture — for vsp
**deployed into BTP as a shared MCP server**. On a developer desktop there is no Cloud
Connector in the path and no destination service to call, so it does not apply. Keep it in
the design for the hosted deployment, and say plainly in the docs that it is not what
`vsp` on a laptop does.

`Prolls`' `Proxy-Authorization`-as-request-header bug must be fixed
(`Transport.GetProxyConnectHeader` for the `CONNECT` leg) before any of this ships, and the
407 retry must rewind the body via `GetBody`.

### 3.5 Path E — short-lived X.509 ✅ **(variant i)** / ❌ **(variant ii)**

**(i) Certificates issued by SAP Secure Login Service, consumed from the OS keystore.**
The user authenticates to IAS (federated to Entra), SAP Secure Login Client drops a
short-lived client certificate into the login keychain / CryptoAPI `MY` store, and vsp does
mTLS with it. The private key never leaves the store. **This is Entra SSO, arriving as a
certificate**, and it is the single highest-value Entra-derived path because the code
mostly exists: Edgars' `pkg/adt/keychain_darwin.go` resolves the certificate *per
handshake* via `tls.Config.GetClientCertificate`, selects the freshest valid cert by
issuer CN, and pins `MaxVersion: TLS1.2` because NetWeaver 7.50's ICM drops TLS 1.3
client-cert handshakes. `smimesign/certstore` supports Windows CryptoAPI as well as macOS,
so the Windows half is a build-tag away. Linux/WSL gets a PKCS#12 file instead.
- **Basis ask:** the issuing CA in `STRUST` (SSL server PSE certificate list),
  `icm/HTTPS/verify_client = 1`, a `CERTRULE` mapping from Subject/Issuer to the SAP user,
  and `SICF` logon procedure including "SSL client certificate".
- **Fix first:** `keychain_other.go` is missing `LoadKeychainClientCertByIssuers`, so the
  patch does not compile off macOS. Five minutes.

**(ii) vsp mints its own ephemeral certificate from a local CA.** `marianfoo`'s design:
`CN=<sap-user>`, 5-minute lifetime, signed by a CA that SAP trusts via `STRUST` +
`CERTRULE`. The design is right *for a server-side deployment where the CA key is guarded*.
**On a developer desktop it is a disaster**: the CA key sits on the laptop, and anyone who
takes it can impersonate any SAP user in the landscape, silently, with a clean audit trail
pointing at the victim. **Do not ship this as a desktop mode.** Its request path also needs
rebuilding regardless — the current code makes a fresh RSA key, `http.Client` and cookie
jar per request, which destroys `sap-contextid` and makes stateful locks impossible.

### 3.6 Summary table

| Path | Entra token in vsp? | Works on desktop | SAP-side config needed | Verdict |
|---|---|---|---|---|
| A — browser + reentrance ticket | no | ✅ all three OSes | SAML2 trust, SSO2 ticket, SICF | **build first** |
| B — code+PKCE vs XSUAA/IAS/ABAP-AS | no (SAP token) | ✅ (fixed port) | OAuth client + redirect URI + scope | **build second** |
| C — SAML2 bearer grant | would need one | ❌ Entra cannot issue | SAML2 trust + SOAUTH2 | **do not build** |
| D — principal propagation via CC | yes (server-side) | ❌ no CC on a laptop | destination + CC + CERTRULE | hosted only |
| E(i) — SLS/SLC cert via OS keystore | no (cert) | ✅ macOS/Win; file on Linux | STRUST + CERTRULE + verify_client | **build third** |
| E(ii) — local CA mints ephemeral cert | yes | ⚠️ works, unsafe | STRUST + CERTRULE | hosted only, never desktop |

---

## 4. A Go-shaped architecture for vsp

### 4.1 The interface

Building on the `Credential` sketch in `fork-survey.md`, with the identity/session split
the RFC leg needs:

```go
// package pkg/adt/auth

// Credential decorates one transport. Basic, cookie, bearer, mTLS and
// proxy-token all implement it; chain composes them.
type Credential interface {
    Apply(*http.Request) error          // header/cookie work; no-op for mTLS
    TLS(*tls.Config)                    // contributes GetClientCertificate, root pool, version pin
    Dial(*websocket.Dialer, http.Header) // the WS bridge is not an http.Request
    Refresh(context.Context) error       // idempotent, single-flight
    NotAfter() time.Time                 // zero == unknown/eternal
    Disruptive() bool                    // true if Refresh invalidates the SAP session
    Name() string                        // diagnostics; never renders a secret
}

// Provider performs the sign-in for one system, once.
type Provider interface {
    Name() string
    Login(ctx context.Context, sys SystemRef, ui Prompter) (*Identity, error)
    Restore(ctx context.Context, sys SystemRef, store Store) (*Identity, error) // cached
}

// Identity is what a sign-in produces, independent of transport. This is the
// object the RFC side consumes; it is deliberately not an *http.Client.
type Identity struct {
    System   string
    SAPUser  string        // resolved principal, if the flow tells us
    NotAfter time.Time
    HTTP     Credential    // ADT + WebSocket
    RFC      *RFCLogon     // nil when nothing the RFC leg can use exists
}

type RFCLogon struct {
    User, Password string           // today: the only thing that works
    Ticket         []byte           // MYSAPSSO2 — see §5, no wire slot yet
    Cert           *tls.Certificate // WebSocket RFC only — see §5
}
```

Three deliberate choices:

- **`Dial` is on the interface.** The survey's version has only `Apply` and `TLS`, and the
  WebSocket bridge (`pkg/adt/websocket_base.go`) is basic-auth-only precisely because it
  was never part of the auth abstraction. Every SSO, cookie and certificate user is
  currently locked out of the debugger, AMDP and report paths. Fix it in the interface, not
  afterwards.
- **`Disruptive()`** is the flag that makes refresh safe (§4.5).
- **`Identity.RFC` is a value, not a client.** open-rfc-go authenticates at dial time and
  cannot re-logon a live connection; handing it a live credential object would imply a
  capability it does not have.

`Config.ReauthFunc` becomes `Credential.Refresh` and stops being special-cased to
`!HasBasicAuth()`. The `authMethods`-counting branch in `processCookieAuth` — which errors
whenever the count is not exactly 1 — is deleted; composition becomes `chain{mtls, bearer}`
rather than a validation failure. That is a real scenario (SLC certificate *and* a Cloud
Connector proxy token).

### 4.2 One TLS constructor

`saml_auth.go`, `browser_auth.go`, `websocket_base.go` and `config.go` each build their own
`tls.Config` today, which is exactly how the TLS-1.2 pin gets forgotten on one path. All
four call one `func (c *Config) tlsConfig() *tls.Config`, which asks the `Credential` to
contribute. Non-negotiable prerequisite for the certificate work.

### 4.3 Token cache: single-flight, proactive, in memory by default

Copy the agency shape, which is small and correct: one mutex-guarded slot per system,
refreshed when expiry falls inside a **60-second buffer**, with concurrent callers
serialising on the mutex so two MCP tool calls never open two browser windows. Persist to
the OS store (§2) only for the refresh token / cookie jar, never for the access token.

```go
type cache struct {
    mu  sync.Mutex
    cur *Identity
}
func (c *cache) Get(ctx context.Context, p Provider) (*Identity, error) // single-flight
```

### 4.4 Configuration in `.vsp.json`

`SystemConfig` gains one nested object. Everything existing keeps working: absent `auth`
means "basic auth from `user`/`password`/`VSP_<SYS>_PASSWORD`", exactly as today.

```jsonc
{
  "systems": {
    "prod": {
      "url": "https://prod.example.com:44300",
      "client": "100",
      "auth": {
        "method": "reentrance",              // basic | cookie | reentrance | mtls | oauth | chain
        "store":  "keychain",                // keychain | wincred | secretservice | file | file+passphrase | none
        "browser": "system",                 // system | exec:/path/to/chrome | none
        "redirect_port": 0,                  // 0 = ephemeral; required fixed for oauth
        "timeout": "120s"
      }
    },
    "steampunk": {
      "url": "https://abc.abap.eu10.hana.ondemand.com",
      "auth": {
        "method": "oauth",
        "oauth": {
          "discover": true,                  // RFC 9728 from the 401 challenge
          "issuer": "https://tenant.authentication.eu10.hana.ondemand.com",
          "client_id": "sb-vsp!t1234",
          "scopes": ["uaa.user"],
          "flow": "authorization_code_pkce", // or device_code
          "redirect_port": 35729
        },
        "store": "keychain"
      }
    },
    "onprem-mtls": {
      "url": "https://sap.example.com:44300",
      "auth": {
        "method": "mtls",
        "cert": { "source": "keystore", "issuer_cn": ["SAP SSO CA", "Corp Issuing CA 2"] },
        "tls_max_version": "1.2"
      }
    },
    "ci": {
      "url": "https://ci.example.com:44300",
      "auth": {
        "method": "basic",
        "credential_cmd": ["op", "item", "get", "SAP-CI", "--format", "json"],
        "store": "none"
      }
    }
  }
}
```

- `cert.source: "keystore"` means Keychain on darwin, CryptoAPI on Windows;
  `"file"` adds `pkcs12`/`cert`/`key` paths for Linux/WSL and CI.
- Every field is overridable by `VSP_<SYSTEM>_AUTH_<FIELD>` for containers.
- **No secret is ever a field.** `credential_cmd` (and its new sibling `token_cmd`) is how
  a secret gets in.

### 4.5 Refresh without breaking a lock or a pinned session

This is the part that gets skipped and then hurts. Three rules:

1. **Refresh proactively, never reactively.** Renew at `NotAfter - 60s`, single-flight. A
   401 arriving mid-`PUT` is a bug in our scheduling, not a normal event. Keep the 401
   handler as a last resort, and make it *retry the request*, not just the credential.
2. **Classify the refresh.** A bearer renewal or an mTLS re-handshake changes only the
   header or the handshake: the `SAP_SESSIONID` and `sap-contextid` survive, and a held
   lock survives with them — `Disruptive() == false`. A browser re-login mints a **new**
   `SAP_SESSIONID`, which retires the ICM context and invalidates every lock —
   `Disruptive() == true`.
3. **Refuse a disruptive refresh while state is held.** The transport already knows when a
   stateful session is open (`SessionStateful`, `sap-contextid` present). If a disruptive
   refresh is required in that window, return a typed `ErrSessionWouldBreak` rather than
   silently re-authenticating and handing back a `423 invalid lock handle` three calls
   later. The caller — `workflows_deploy.go`, or the MCP handler — unlocks, refreshes, and
   re-locks. This is also the right instrument for the known "stateless hop between LOCK
   and PUT kills the lock" bug class the fork survey documents.

For the RFC leg the situation is simpler and stricter: **there is no re-logon on a live
CPIC connection.** `Identity.NotAfter` therefore bounds a pinned session. `vsp-debugd`
(Sprint 4) must read it at start, refuse to pin a session it cannot outlive, and warn the
operator rather than dying mid-debug. A 5-minute SLC certificate is unusable for a pinned
debugger; a 12-hour password is fine. Say this in the daemon's docs.

### 4.6 The MCP boundary

- **Never authenticate interactively from inside the MCP server.** Sign-in is
  `vsp auth login <system>` / `vsp auth status` / `vsp auth logout`, run by a human in a
  terminal; the server calls `Provider.Restore` and fails with a clear "run `vsp auth
  login prod`" if the store is empty. This also stops a browser window opening because a
  model called a tool.
- **Keep inbound and outbound auth separate.** `VSP_HTTP_API_KEY` authenticates the caller
  to vsp's Streamable HTTP transport; the `Credential` authenticates vsp to SAP. Do not
  let the second be derived from the first. (Later: RFC 9728 protected-resource metadata on
  the inbound side, which `marianfoo` already has.)
- One `Identity` per (system, user) per process, shared by ADT, WebSocket and RFC — not one
  per request.

---

## 5. The RFC leg: what an Entra identity can and cannot do

`open-rfc-go` speaks classic RFC over CPIC. The logon frame is
`internal/cpic/logon.go`: `TagUser` (`0x0111`), `TagPassword` (`0x0117`), `TagClient`,
`TagLanguage`. The password goes through `internal/scramble`, which is a **64-byte
substitution table — obfuscation, not encryption** — and is capped at **40 bytes of
ASCII**. There is no SNC (`docs/roadmap.md` P2), no WebSocket RFC, no X.509, no SSO
tickets; `docs/about.md` and `docs/live-test-plan.md` both say so explicitly.

### What works today

A user name and a password, ≤ 40 ASCII bytes, recoverable by anyone on the wire.

### What an Entra identity can do today

**Nothing.** Concretely:

- An Entra access token is a JWT of 1–2 KB. It does not fit in a 40-byte ASCII field, and
  there is no other field.
- A `MYSAPSSO2` reentrance ticket is several hundred bytes of base64. Same answer.
- There is no channel encryption, so even if a ticket fitted, sending a bearer credential
  over an unauthenticated, unencrypted CPIC channel would be worse than the password.
- SAP's own documentation is unambiguous on the underlying constraint: **X.509
  certificate-based authentication is supported for WebSocket RFC and is *not* supported
  for RFC via CPIC** (SAP KBA 3152253).

So: for the RFC leg, an Entra-authenticated developer today still supplies a SAP password.
The honest mitigation is *where the password lives* — move it out of `.vsp.json` and
`SAP_PASSWORD` into the OS store or `credential_cmd` (§2) — not *how it is authenticated*.

### What would be needed, ranked

**1. WebSocket RFC (`wshost`/`wsport`) — the right answer, and the strategic one.**
WebSocket RFC runs through the ICM as an ICF service, so it inherits ICF logon procedures:
X.509 client certificates (explicitly supported, per the KBA above), and — subject to
confirmation on a live system — the SSO2 cookie and SAML paths that every other ICF service
accepts. That means **the entire HTTP credential stack from §4 applies unchanged to RFC**:
Path A's cookie jar, Path B's bearer, Path E(i)'s keystore certificate. No CommonCryptoLib,
no cgo, no native dependency, no compromise of the SDK-free goal. The payload layer
(classic and fast serialisation, metadata, structures, tables) is already built and is
shared; what is missing is the transport and the logon frame. `docs/cheatsheet.md` already
reserves `wshost` in the destination shape, and the upstream `open-rfc` project's password
field is documented as "CPIC/WebSocket", which suggests the format is at least partly
known. **This should replace SNC on the roadmap as the RFC authentication project.**

**2. A ticket-carrying CPIC logon field — worth one week of reconnaissance, no more.**
JCo exposes `jco.client.mysapsso2` and `jco.client.x509cert` as *logon parameters*, which
implies a CPIC tag we have not identified — though the KBA's flat statement that CPIC does
not do X.509 suggests `x509cert` may be WebSocket-only. This project is good at exactly
this kind of question: `internal/sniffer` plus a JCo or NW RFC SDK client as an oracle
would settle it in days. If a ticket tag exists, the payoff is large — the ADT leg's
browser reentrance ticket would log the RFC leg on as the same human. Prerequisites on the
SAP side would be `login/accept_sso2_ticket = 1` and the issuing system in the ticket ACL
(`STRUSTSSO2`). Risks: the field may be gated on a protocol version we do not negotiate,
and sending a bearer ticket over cleartext CPIC still wants TLS or SNC underneath.
**Time-box it. Do not let it become the plan.**

**3. SNC — defer, probably permanently.** A cgo GSS-API binding to CommonCryptoLib, then
wrapping frames in `gss_wrap`/`gss_unwrap`. It reintroduces the native dependency the
project exists to avoid, requires per-platform CommonCryptoLib discovery (cwbr's
`findSNCLibrary()` is the reconnaissance half and is worth taking regardless), and even
then an Entra identity only reaches it via Kerberos or an SLC-issued credential — i.e.
via an out-of-band agent, not via anything we build. WebSocket RFC gets the same
authentication with a fraction of the effort and no cgo.

### What to take from the fork survey into open-rfc-go regardless

`pkg/adt/landscape.go` (SAPUILandscape.xml → hosts, message servers, SAProuter strings, SNC
partner names) transfers nearly unchanged and feeds the existing `saprouter.Route`
machinery. It is a usability win independent of any authentication work. Licence direction:
MIT → Apache-2.0 is permitted with the notice carried; the reverse is not, and open-rfc-go
requires a DCO sign-off that a `Co-authored-by` trailer does not supply.

---

## 6. Staged plan

### Stage 0 — the shape (2 days, no new auth methods)

- `pkg/adt/auth` with `Credential` / `Provider` / `Identity`, and `chain`.
- One `tlsConfig()` constructor; delete the other three.
- `Credential.Dial` wired into `websocket_base.go`, so the debugger stops being
  basic-auth-only.
- `vsp auth login|status|logout`, and the store abstraction (§2) with `none` + `file` +
  `keychain` backends.
- Refuse a world-readable `.vsp.json`; warn on a plaintext `password`.
- Generalise `credential_cmd` to also return `{token, expires_on}` (`token_cmd`), which
  buys WAM/broker/conditional-access via `az`/`azureauth` for ~40 lines (§1.3).

*No Basis prerequisite. Nothing user-visible breaks. Everything below depends on it.*

### Stage 1 — browser SSO ✅ **build this first** (1–2 days)

BurnerPat's reentrance-ticket flow: `resolveSystemURLs`, loopback listener,
`openSystemBrowser` (with the WSL launcher chain from §1.4), `exchangeReentranceTicket`,
`parseSystemInformationLink`. Persist the cookie jar in the OS store. Port the 15 tests.
Retire the chromedp path to `--browser-exec` legacy.

**This is where Entra starts working for most corporate developers, with zero Entra code
in the binary.** Basis prerequisite is SAML2 trust, which in an Entra shop almost always
already exists because Fiori and the Launchpad need it.

### Stage 2 — certificates (2–3 days)

Edgars' keychain path, non-darwin build fixed first, then Windows CryptoAPI via the same
`certstore` dependency, then file-based PKCS#12 + custom CA for Linux/WSL/CI (marianfoo's
`WithClientCert`/`WithCACert`, with the swallowed errors made fatal). Keep the deliberate
`MaxVersion: TLS1.2` pin and its comment. Fix `.goreleaser.yml`'s global `CGO_ENABLED=0`.

**Basis prerequisite:** CA in `STRUST`, `CERTRULE` mapping, `icm/HTTPS/verify_client`.

### Stage 3 — OAuth2 code + PKCE (2–3 days)

Against XSUAA / IAS / the ABAP OAuth AS. RFC 6750 + RFC 9728 challenge discovery, fixed
redirect port, refresh tokens, proactive renewal, device-code fallback. This is the
Steampunk/BTP developer story and it is unbuilt anywhere in the fork network.

**Basis/BTP prerequisite:** an OAuth client with our redirect URI registered.

### Stage 4 — hosted deployments (needs a real landscape; not desktop)

Destination service + Cloud Connector with the `CONNECT` bug fixed and the 407 retry
rewinding via `GetBody`; `OAuth2SAMLBearerAssertion` / `PrincipalPropagation` destination
types; principal propagation with a per-user certificate and client cache. Only relevant
once vsp runs as a shared BTP-hosted MCP server.

### Deferred, with reasons

| Item | Why not now |
|---|---|
| WAM / macOS broker | No Go binding exists; cgo would break the six-platform static build |
| SAML2 bearer grant from the desktop | Entra cannot issue an assertion out of band (§3.3) |
| Desktop-minted ephemeral certs | Puts an impersonation-capable CA key on a laptop (§3.5) |
| RFC ticket tag reverse engineering | Time-boxed research; WebSocket RFC is the better bet |
| SNC | cgo + CommonCryptoLib; WebSocket RFC gets the same result cheaper |

### Research items (separate track, not in Sprint 3)

1. **WebSocket RFC in open-rfc-go.** The single highest-value RFC item. It makes every
   credential in §4 usable by the RFC leg.
2. **Confirm what ICF logon procedures WebSocket RFC actually honours** on a live system —
   X.509 is documented; SSO2 cookie and SAML need proving.

---

## 7. Testing without a corporate tenant

Everything above is testable on a laptop. Nothing here needs an Entra tenant or a Basis
team.

**The IdP.** Run **Keycloak** in Docker. It stands in for Entra *and* for IAS, and it
speaks every protocol we need: SAML2 IdP (for Path A), OIDC with authorization-code + PKCE
(Path B), the device authorization grant (the fallback), and it can issue client
certificates through an external CA. If you specifically want Entra semantics — the
`http://localhost:<any-port>` public-client redirect rule, `.default` scopes, conditional
access — a **free Microsoft Entra ID tenant** (or a Microsoft 365 developer tenant) is
enough: register a public client, tick "Allow public client flows" for device code, add
`http://localhost` as a redirect URI. No paid tier, no corporate approval.

**The SAP system.** The **ABAP Platform Trial (A4H) Docker image** you already run for the
RFC work is a full AS ABAP: it has `SAML2`, `SICF`, `STRUST`, `CERTRULE`, `SOAUTH2`,
`RZ11`, `SU01`. Configure it as a SAML2 SP against Keycloak, map the NameID to `DEVELOPER`
via `SU01` → Aliases, set `login/create_sso2_ticket = 2`, and the whole of Path A runs end
to end on one machine. Path E(i) is testable by generating a CA and a client cert with
`openssl` or `step-cli`, importing the CA into `STRUST`, adding a `CERTRULE` entry, and
importing the P12 into the login keychain — that proves the certificate resolution and the
TLS 1.2 pin without SAP Secure Login Client, which the trial does not include.

**What cannot be tested locally, and must be flagged as untested until someone runs it:**
- SAP Secure Login Service issuing the certificate (needs SAP Cloud Identity Services).
- XSUAA specifically, as opposed to Keycloak-as-OAuth-AS (needs a BTP subaccount; the
  free tier does provide one, and an `xsuaa` service instance is within the free plan).
- Cloud Connector principal propagation (needs a CC and a real on-prem, i.e. Stage 4).
- Conditional access and device-compliance policies, which by definition only exist in a
  managed tenant.

**Per-platform smoke tests worth writing as a checklist, not as CI:** browser launch and
loopback callback on macOS, on Windows, on WSL2 with `localhostForwarding` on and off, and
device-code fallback over SSH with no `DISPLAY`. These are the four cases that actually
break, and none of them can be caught by `go test`.


---

## Why one reference is missing

The comparison system studied while writing this document is internal third-party
material: its documentation carries real tenant and application identifiers and a
vulnerability write-up. The study of it is deliberately **not** in this repository —
not even ignored, since an ignore rule is one `git add -f` away from publishing it.
It lives next to its source, outside the tree. What is reproduced here are only
design ideas, in our own words and with no identifiers: terminate credentials in a
local proxy rather than in the agent, bind a plugin's credential to a hash of the
config that declared it, keep one mutex-guarded token slot per system, scrub
`*_AUTH_*` from a child process's environment, and discover the authorization server
from a 401 rather than configuring it.
