# Spike: SAML Authentication for SAP S/4HANA Public Cloud

> **Date:** 2026-04-08
> **Status:** Analyzed -- recommendation ready, pending human decision
> **Priority:** High -- blocks Public Cloud connectivity for all users
> **Pipeline:** spike-6d2c6057
> **Cross-validation:** [C+O] Consensus -- Porfiry [Opus 4.6] + GPT-5.2-Pro + GPT-5.1-Codex

---

## Problem

VSP `--browser-auth` does not work with SAP S/4HANA Public Cloud systems that use SAML SSO via SAP IAS (Identity Authentication Service).

**Current behavior:** VSP opens a clean Edge instance (no user profile). The IAS login form appears, but:
1. No existing IAS session is available (clean profile)
2. User must manually enter credentials in the VSP-spawned Edge window
3. Even if user enters credentials, VSP polls for `SAP_SESSIONID_*` cookie but the SAML redirect chain may not complete within the timeout

**Expected behavior:** VSP should authenticate to SAP Public Cloud and obtain a working ADT session.

## Root Cause Analysis

S/4HANA Public Cloud uses a multi-step SAML flow:

```
GET /sap/bc/adt/discovery
  -> SAML form (SAMLRequest + IAS redirect)
    -> IAS login form (j_username + j_password)
      -> SAMLResponse
        -> SAP ACS (/sap/saml2/sp/acs/<client>)
          -> redirect chain
            -> SAP_SESSIONID_<SID>_<CLIENT> cookie
```

Key differences from on-premise:
1. **No Basic Auth** -- disabled for business users
2. **SAML is mandatory** -- no fallback
3. **IAS as IdP** -- not the SAP system itself
4. **Email-based login** -- IAS uses email, not SAP user ID
5. **Reentrance ticket flow** -- Eclipse uses `/sap/bc/adt/core/http/reentranceticket` with localhost redirect

### Authentication Architecture

| Component | URL | Purpose |
|-----------|-----|---------|
| Non-API hostname | `my413862.s4hana.cloud.sap` | SAML-protected, UI + initial auth |
| API hostname | `my413862-api.s4hana.cloud.sap` | Basic Auth (comm users only) |
| IAS tenant | `ahzoesedf.accounts.cloud.sap` | SAML IdP, login form |

### Working Python PoC

A proven Python script performs the full SAML dance (`GitLab/scripts/saml-login.py`):
1. GET `/sap/bc/adt/discovery` -> parse HTML form (SAMLRequest + action URL)
2. POST to IAS -> parse IAS login form
3. POST credentials (`j_username`, `j_password`) to IAS -> get SAMLResponse
4. Loop up to 10x following SAML form chain (action URL + hidden inputs)
5. Verify with GET `/sap/bc/adt/discovery` -> check `sap-authenticated` header
6. Save cookies in Netscape format

---

## Option Analysis

### Option A: Built-in SAML Flow (`--saml-auth`)

Go HTTP client performs SAML dance internally, mirroring the Python PoC.

**New flags:** `--saml-auth`, `--saml-user` / `SAP_SAML_USER`, `--saml-password` / `SAP_SAML_PASSWORD`
**New file:** `pkg/adt/saml_auth.go`
**New dep:** `golang.org/x/net/html` (for robust form parsing instead of regex)

| Aspect | Assessment |
|--------|-----------|
| Effort | 2-3 days |
| CI/CD friendly | Yes -- primary value |
| MFA support | No -- password-only |
| Upstream PR readiness | High -- new file + flags, self-contained |
| Fragility | Medium -- IAS form parsing (mitigated by SAML-field targeting, not layout) |
| New dependencies | `golang.org/x/net/html` (stdlib-adjacent) |
| Session refresh | Re-run SAML flow automatically on 401 |

### Option B: Browser Auth with User Profile (`--browser-profile`)

Reuse existing Edge profile with active IAS session via `--user-data-dir`.

**New flag:** `--browser-profile` / `SAP_BROWSER_PROFILE`
**Workaround exists:** `edge-with-profile.cmd` in repo root

| Aspect | Assessment |
|--------|-----------|
| Effort | 1 day nominal, 2-3 days real (profile locking) |
| CI/CD friendly | No -- requires browser + active session |
| MFA support | Yes -- reuses existing authenticated session |
| Upstream PR readiness | Medium -- platform-specific edge cases |
| Fragility | Medium -- Edge locks profile when open, must copy to temp dir |
| New dependencies | None |
| Session refresh | Re-navigate to IAS (automatic via chromedp) |

### Option C: Browser Auth Cookie Detection Fix

Fix existing `--browser-auth` to handle SAML redirect chain properly.

**Files modified:** `pkg/adt/browser_auth.go` only
**No new dependencies**

| Aspect | Assessment |
|--------|-----------|
| Effort | 1-2 days |
| CI/CD friendly | No -- requires interactive browser |
| MFA support | Yes -- manual login in browser window |
| Upstream PR readiness | Highest -- smallest diff, fixes existing feature |
| Fragility | Low -- leverages existing chromedp infrastructure |
| New dependencies | None |
| Session refresh | Manual re-login |

The fix: ensure chromedp follows the SAML form auto-submits. The current code already navigates to `/sap/bc/adt/` which triggers the SAML redirect, and already checks for `SAP_SESSIONID_*` cookies. The issue is timing -- SAML chain has multiple redirects that need time to complete. Fix cookie detection to be more patient with SAML flows and add verbose logging for redirect tracking.

### Option D: OAuth2 ROPC via IAS

IAS supports `grant_type=password` via OAuth2 Resource Owner Password Credentials.

| Aspect | Assessment |
|--------|-----------|
| Effort | 3-4 days + IAS admin setup |
| CI/CD friendly | Yes -- token-based, supports refresh |
| MFA support | No -- ROPC bypasses MFA |
| Upstream PR readiness | Low -- requires admin docs, ROPC deprecated in OAuth 2.1 |
| Fragility | Low -- standards-based HTTP |
| New dependencies | `golang.org/x/oauth2` |
| Session refresh | Token refresh (automatic) |

**Blocker:** Requires IAS OAuth `client_id` registration (admin access we don't control).
**Warning:** ROPC is deprecated in OAuth 2.1 [C+O consensus].

---

## Recommendation

### Implementation Sequence [C+O majority]

```
Phase 1: Option C -- Fix browser-auth for SAML (upstream PR #1)
  |  Smallest PR, fixes existing broken feature
  |  Unblocks ALL users including MFA immediately
  v
Phase 2: Option A -- Programmatic SAML flow (upstream PR #2)
  |  Enables CI/CD and scripted authentication
  |  Proven by Python PoC
  v
Phase 3: --credential-cmd helper + KeePass (upstream PR #3 + fork-only)
  |  Generic credential provider pattern
  v
Deferred: Option B (if demand), Option D (if IAS standardizes OAuth for ADT)
```

### Dissenting View [O: GPT-5.1-Codex, adversarial]

Argued for B first, D second, A as last resort. Reasoning: HTML parsing is inherently fragile; IAS could add JS challenges/CAPTCHA; enterprise CLIs (AWS, Azure) use browser-based or OAuth flows.

**Why we disagree:** (1) IAS login is standard HTML with stable SAML fields, not a JS SPA. The Python PoC has worked reliably. (2) Option D requires IAS admin provisioning we don't control. (3) Option B has real profile-locking complexity. (4) Option A directly replaces the working Python PoC with zero external dependencies.

---

## KeePass Credential Integration

### For Upstream: `--credential-cmd` (PR #3)

Git-credential-helper pattern -- generic, supports any credential manager:

```bash
vsp --saml-auth --credential-cmd "keepassxc-cli show -s -a Username -a Password db.kdbx 'SAP/K0B'"
```

The command outputs JSON to stdout:
```json
{"username": "user@example.com", "password": "secret"}
```

**New flag:** `--credential-cmd` / `SAP_CREDENTIAL_CMD`
**New file:** `pkg/adt/credential_cmd.go`

This keeps upstream code free of KeePass-specific dependencies while enabling any credential manager (KeePass, 1Password, Vault, pass, etc.).

### For Fork: Go-native KeePass

**Recommended:** `gokeepasslib/v3` in Go, behind build tag `//go:build keepass`.

| Approach | Pros | Cons |
|----------|------|------|
| Shell out to Python `sap-credentials.py` | Reuses existing script | Requires Python + pykeepass installed; breaks single-binary promise |
| `gokeepasslib/v3` (Go native) | Single binary, cross-platform | Extra dependency in fork build |
| `keepassxc-cli` via `--credential-cmd` | No Go deps, works now | Requires KeePassXC installed |

**Decision:** Use `--credential-cmd` with `keepassxc-cli` for immediate use (Phase 3), plan `gokeepasslib` integration for later if zero-dependency UX is needed.

### Master Password Handling

- TTY-only prompt; never accept as env var or CLI flag
- Cache in memory for session duration only
- Mirrors pattern from dashboard project's `credential-manager.ts`

### Dashboard KeePass Pattern (reference)

The `claude-team-control` dashboard uses:
- `scripts/sap-credentials.py` -- Python + `pykeepass`, searches by SID title
- `credential-manager.ts` -- TypeScript, master password caching via VSCode SecretStorage (DPAPI)
- `sync.ps1` -- PowerShell, calls Python script, sets env vars

For VSP, the `--credential-cmd` pattern is the Go equivalent of this pipeline.

---

## Upstream PR Strategy

### PR #1: Enhanced Browser Auth for SAML (Option C)

| Aspect | Detail |
|--------|--------|
| Files modified | `pkg/adt/browser_auth.go`, `pkg/adt/browser_auth_test.go` |
| Files new | None |
| Dependencies | None |
| Risk | Minimal -- improves existing feature |
| PR description | "Fix browser-auth for SAML-based SSO flows (S/4HANA Public Cloud)" |

### PR #2: Programmatic SAML Auth (Option A)

| Aspect | Detail |
|--------|--------|
| Files new | `pkg/adt/saml_auth.go`, `pkg/adt/saml_auth_test.go` |
| Files modified | `cmd/vsp/main.go` (flags + `processSAMLAuth()`), `go.mod` |
| Dependencies | `golang.org/x/net` (for `html` package) |
| Risk | Medium -- new feature, but self-contained |
| PR description | "Add --saml-auth for programmatic SAML SSO to S/4HANA Public Cloud" |

**Why `pkg/adt` not `pkg/auth`:** For upstream PRs, keeping `saml_auth.go` alongside existing `browser_auth.go` is the path of least resistance. The upstream owner already has this pattern. A `pkg/auth` refactor is a separate discussion.

### PR #3: Credential Helper (future)

| Aspect | Detail |
|--------|--------|
| Files new | `pkg/adt/credential_cmd.go` |
| Files modified | `cmd/vsp/main.go` (flag) |
| Dependencies | None |
| Risk | Low -- generic, extensible |

### Fork-Only (never in any PR)

- `pkg/auth/keepass.go` (build tag `keepass`) -- if Go-native KeePass is built
- `internal/mcp/tools_register_fork.go` -- extension hook
- `.claude/` directory -- team config

---

## Security Considerations

| Concern | Mitigation |
|---------|-----------|
| IAS password in env vars | Support but prefer `--credential-cmd` or TTY prompt as primary. Document CI log exposure risk. |
| SAML assertions in memory | Never log SAMLRequest/SAMLResponse bodies, even in verbose mode. Log only action URLs and status codes. |
| Cookie file permissions | Already handled: `SaveCookiesToFile` uses 0600 mode. |
| Browser profile copy (Option B) | Copy to temp dir with 0700; wipe after cookie extraction. |
| KeePass master password | TTY-only prompt; never accept as env var or CLI flag. |
| Verbose logging | Audit all `fmt.Fprintf(os.Stderr, ...)` paths for credential/token leaks. |
| MFA bypass | Never implement. Option A explicitly does not support MFA -- documented limitation. |

### Session Management

- SAP session cookies expire after ~30 minutes of idle
- Existing `--keepalive` mechanism (5-min ping) prevents idle expiry
- For SAML auth: transparent re-auth on 401 using cached IAS credentials (Option A) or error with re-login instructions (Options B/C)

---

## Implementation Phases

### Phase 1: Browser Auth SAML Fix (Option C) -- 1-2 days

**Gate:** `go test ./pkg/adt/...` passes, manual test against K0B DEV succeeds.

- [ ] T1.1: Improve `pollForSAPCookies` to handle SAML redirect chain timing
- [ ] T1.2: Add verbose logging for SAML redirect tracking
- [ ] T1.3: Consider increasing default `--browser-auth-timeout` for SAML flows
- [ ] T1.4: Add test cases simulating SAML cookie appearance pattern
- [ ] T1.5: Submit upstream PR #1

### Phase 2: Programmatic SAML Flow (Option A) -- 2-3 days

**Gate:** `go test ./pkg/adt/...` passes, `saml_auth_test.go` covers all 4 SAML steps, manual test succeeds.

- [ ] T2.1: Create `pkg/adt/saml_auth.go` with `SAMLLogin()` (4-step SAML dance)
- [ ] T2.2: Use `golang.org/x/net/html` for form parsing (not regex) [C+O]
- [ ] T2.3: Add `--saml-auth`, `--saml-user`, `--saml-password` flags to `main.go`
- [ ] T2.4: Add env vars: `SAP_SAML_AUTH`, `SAP_SAML_USER`, `SAP_SAML_PASSWORD`
- [ ] T2.5: Write comprehensive unit tests with mock HTTP server
- [ ] T2.6: Submit upstream PR #2

### Phase 3: Credential Helper + KeePass -- 1-2 days

- [ ] T3.1: Implement `--credential-cmd` in `pkg/adt/credential_cmd.go` (upstream PR #3)
- [ ] T3.2: Wire credential sources into SAML auth flow
- [ ] T3.3: Optional: KeePass Go-native provider (fork-only, build tag)

### Phase 4 (Future): Browser Profile Reuse (Option B)

- [ ] T4.1: Add `--browser-profile` flag
- [ ] T4.2: Handle profile locking (detect open browser, copy to temp)
- [ ] T4.3: Clean up temp profile after extraction

---

## Open Questions for Human Decision

1. **Phase ordering:** C first (fix browser), then A (SAML flow)? Or A first if CI/CD is the primary use case and MFA is not enforced on K0B?

2. **Package structure for upstream:** Keep in `pkg/adt` (matching `browser_auth.go` pattern) or propose `pkg/auth` with Provider interface?

3. **KeePass approach:** `--credential-cmd` with `keepassxc-cli` (immediate, no deps) vs `gokeepasslib` build tag (single binary, but fork-only complexity)?

---

## Test Systems

| System | Non-API URL | Client | IAS Tenant |
|--------|-------------|--------|------------|
| K0B CUST | my413856.s4hana.cloud.sap | 100 | ahzoesedf.accounts.cloud.sap |
| K0B DEV | my413862.s4hana.cloud.sap | 080 | ahzoesedf.accounts.cloud.sap |

## References

- Research report: `tech-research/docs/2026-04-08-s4hana-public-cloud-saml-auth.md`
- Working Python PoC: `GitLab/scripts/saml-login.py`
- IAS OIDC config: `https://ahzoesedf.accounts.cloud.sap/.well-known/openid-configuration`
- SAP Note 3259196 (ADT requires SAP_BR_DEVELOPER)
- Dashboard KeePass integration: `claude-team-control/scripts/sap-credentials.py`
- Dashboard credential manager: `claude-team-control/vscode-dashboard/src/services/credential-manager.ts`

## Cross-Validation Record

| Aspect | Porfiry [Opus 4.6] | GPT-5.2-Pro (neutral) | GPT-5.1-Codex (adversarial) | Tag |
|--------|--------------------|-----------------------|----------------------------|-----|
| Phase ordering | A first, then C | C first, then A (8/10) | B first, then D (7/10) | [C+O] C->A |
| Option A fragility | Mitigated by field targeting | Accepts with caveats | Strongly opposes | [C+O] accept |
| `--credential-cmd` upstream | Agree | Agree | Agree | [C+O] |
| `pkg/auth` abstraction | Agree (future) | Agree (future) | Agree | [C+O] |
| KeePass approach | Go native (`gokeepasslib`) | Open | Python shell-out | Resolved: `--credential-cmd` |
| Option D timing | Deferred | Deferred | Second priority | [C+O] deferred |
| ROPC deprecation | Yes (OAuth 2.1) | Yes | Noted | [C+O] |
