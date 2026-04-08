# Plan: SAML Authentication for SAP S/4HANA Public Cloud

**Session:** saml
**Created:** 2026-04-08
**Status:** APPROVED
**Audit round 1:** 3 MEDIUM fixed (argv exec, credential zeroing, test split)
**Audit round 2:** 1 HIGH + 2 MEDIUM fixed (401 re-auth gap, shlex dep, credential lifecycle)
**Final verdict:** APPROVE [C+O] — zero MEDIUM+ remaining
**Spike:** [docs/spikes/2026-04-08-saml-auth-public-cloud.md](spikes/2026-04-08-saml-auth-public-cloud.md)
**Cross-validation:** [C+O] Porfiry [Opus 4.6] + GPT-5.2-Pro + GPT-5.1-Codex — 8/10 confidence

---

## Context

VSP `--browser-auth` cannot authenticate to SAP S/4HANA Public Cloud systems using SAML SSO via SAP IAS. Basic Auth is disabled for business users. This blocks Public Cloud connectivity for all users.

The spike analyzed 4 options and recommends: C (fix browser-auth) -> A (programmatic SAML) -> credential-cmd. Each phase produces a standalone upstream PR.

### ADR-SAML-001: Implementation Sequence C -> A -> credential-cmd

**Status:** Accepted [C+O]

**Decision:** Fix existing `--browser-auth` first (Option C), then add programmatic `--saml-auth` (Option A), then add `--credential-cmd` helper.

**Rationale:** C is the smallest diff (1 file), fixes the most users immediately (including MFA), and builds upstream trust for the larger PR #2. A enables CI/CD automation. credential-cmd decouples secret storage.

### ADR-SAML-002: Package Placement in pkg/adt

**Status:** Accepted [C+O]

**Decision:** Keep SAML auth code in `pkg/adt` alongside existing `browser_auth.go`, not in a new `pkg/auth` package.

**Rationale:** Matches upstream codebase pattern. A structural refactor bundled with new auth behavior would reduce PR acceptance probability. Future refactor to `pkg/auth` can be proposed separately once both flows stabilize.

### ADR-SAML-003: golang.org/x/net/html for Form Parsing

**Status:** Accepted [C+O]

**Decision:** Use `golang.org/x/net/html` tokenizer for SAML/IAS form parsing instead of regex.

**Rationale:** Regex parsing of HTML is brittle (attribute ordering, escaping, malformed markup). `x/net/html` is stdlib-adjacent, stable, and handles edge cases. Both models strongly agree.

### ADR-SAML-004: --credential-cmd for Credential Providers

**Status:** Accepted [C+O]

**Decision:** Use git-credential-helper pattern (`--credential-cmd`) for upstream. Fork-only KeePass support via `gokeepasslib/v3` behind build tag is optional future work.

**Rationale:** Generic pattern supports any credential manager (KeePass, 1Password, Vault). Verified via context7: `gokeepasslib/v3` has benchmark 95.3, supports KDBX 3.1/4.0, API confirmed (`GetTitle()`, `GetPassword()`, `GetContent("UserName")`).

---

## Implementation Phases

### Phase SAML.1: Fix Browser Auth for SAML (upstream PR #1)

**Goal:** Fix existing `--browser-auth` to work with SAML/IAS SSO flows on S/4HANA Public Cloud.
**Files:** `pkg/adt/browser_auth.go`, `pkg/adt/browser_auth_test.go`
**New deps:** None
**Estimated effort:** 1-2 days

- [ ] T1.1: Fix `extractSAPCookies` cookie URL filtering — pass multiple URLs to `WithURLs()` (base URL + `/sap/bc/adt/` path) to handle SAML cookie path scoping
- [ ] T1.2: Improve `pollForSAPCookies` timing — add SAML-aware patience (wait longer for multi-hop redirect chain, log each poll cycle in verbose mode)
- [ ] T1.3: Add verbose SAML redirect logging (URL + host tracking, cookie names only — never values or SAML assertion bodies)
- [ ] T1.4: Write unit tests for cookie filtering logic — extract cookie name matching from `extractSAPCookies` into a testable helper that accepts `[]*network.Cookie` input. Test cases: SAML domain cookies, path-scoped cookies, weak vs strong cookie distinction, empty cookie jar
- [ ] T1.5: Write integration test under `//go:build integration` tag — `httptest.Server` with SAML-like redirect chain + headless chromedp, verify cookie extraction after multi-hop redirect
- [ ] T1.6: Manual test against K0B DEV (`vsp --browser-auth --url https://my413862.s4hana.cloud.sap -v`) — primary validation for cookie polling timing fix
- [ ] GATE: `go test ./pkg/adt/...` passes (verify T1.4 unit tests + T1.5 integration test exist) + `mcp__pal__codereview` + `mcp__pal__thinkdeep` — zero MEDIUM+ before next phase

**Rollback:**
1. `git revert <commit>` — single file change, clean revert
2. Existing `--browser-auth` behavior restored (was already broken for SAML, so no regression)

### Phase SAML.2: Programmatic SAML Flow (upstream PR #2)

**Goal:** Add `--saml-auth` flag that performs the full SAML dance via HTTP client without browser.
**Files new:** `pkg/adt/saml_auth.go`, `pkg/adt/saml_auth_test.go`
**Files modified:** `cmd/vsp/main.go`, `go.mod`, `pkg/adt/http.go`, `pkg/adt/config.go`
**New deps:** `golang.org/x/net` (for `html` package — form parsing)
**Estimated effort:** 2-3 days

- [ ] T2.1: Create `pkg/adt/saml_auth.go` with `SAMLLogin(ctx, sapURL, user, password, insecure, verbose)` — 4-step SAML dance:
  - Step 1: GET target URL → detect SAML form, extract SAMLRequest + action URL using `x/net/html` tokenizer
  - Step 2: POST SAMLRequest to IAS → parse IAS login form (j_username, j_password fields)
  - Step 3: POST credentials to IAS → extract SAMLResponse
  - Step 4: Follow SAMLResponse chain (loop up to 10 form POSTs) → extract SAP session cookies
  - **Credential lifecycle:** Use `CredentialProvider func(ctx) (user, pass []byte, err error)` callback pattern. The provider re-reads credentials from env/credential-cmd on each invocation — no long-term credential retention in memory. This resolves the zeroing vs re-auth conflict: credentials are obtained fresh for each SAML dance (initial + re-auth on 401). Zero `[]byte` buffers after each use.
  - **InsecureSkipVerify:** When `--insecure` is set, TLS verification is also skipped for the IAS endpoint. Document in `--saml-auth` help text with warning.
- [ ] T2.2: Implement HTML form parser helper using `golang.org/x/net/html` — extract `<form action>` and `<input name value>` from HTML response body. Target SAML-standard field names (SAMLRequest, SAMLResponse, RelayState), not layout.
- [ ] T2.3: Add CLI flags to `cmd/vsp/main.go`: `--saml-auth`, `--saml-user` / `SAP_SAML_USER`, `--saml-password` / `SAP_SAML_PASSWORD`. Add `processSAMLAuth()` function.
- [ ] T2.4: Add `processSAMLAuth(cmd)` function called between `processBrowserAuth` and `processCookieAuth` in the PersistentPreRunE chain. `processSAMLAuth` performs the SAML dance and populates `cfg.Cookies`, identical to how `processBrowserAuth` works. The existing `authMethods` counter in `processCookieAuth` already handles mutual exclusivity.
- [ ] T2.5: Wire 401 re-auth into Transport layer:
  - Add `ReauthFunc func(ctx context.Context) (map[string]string, error)` field to `Config` (config.go)
  - Modify 401 handler in `http.go` `retryRequest()`: when `HasBasicAuth()` is false and `ReauthFunc` is set, call it to get fresh cookies instead of `fetchCSRFToken()` Basic Auth path
  - Use `sync.Once` or `singleflight` to prevent concurrent 401 stampede (multiple goroutines triggering simultaneous IAS logins)
  - `processSAMLAuth` in main.go sets `cfg.ReauthFunc` = closure calling `SAMLLogin` with the `CredentialProvider`
- [ ] T2.6: Write comprehensive unit tests with mock HTTP server:
  - `TestSAMLLogin_FullFlow` — 4-endpoint mock simulating SAP→IAS→SAP chain
  - `TestSAMLLogin_WrongPassword` — IAS returns error page
  - `TestSAMLLogin_IASUnavailable` — connection refused
  - `TestSAMLLogin_MalformedSAML` — missing SAMLResponse field
  - `TestSAMLLogin_RedirectLoop` — >10 hops protection
  - `TestSAMLAuth_VerboseNoSecrets` — capture stderr, verify no passwords/assertions logged
  - `TestSAMLLogin_ReauthOn401` — verify Transport calls ReauthFunc on 401, gets fresh cookies, retries request
  - `TestSAMLLogin_ReauthConcurrent` — verify singleflight prevents stampede on simultaneous 401s
- [ ] T2.7: Manual test against K0B DEV (`vsp --saml-auth --saml-user user@example.com --saml-password *** --url https://my413862.s4hana.cloud.sap`)
- [ ] GATE: `go test ./pkg/adt/...` + `go test ./cmd/vsp/...` passes (verify all 8 new test cases) + `mcp__pal__codereview` + `mcp__pal__thinkdeep` — zero MEDIUM+ before next phase

**Rollback:**
1. `git revert <commit>` — new files + minimal changes to main.go, http.go, config.go
2. Remove `golang.org/x/net` from go.mod via `go mod tidy`
3. `ReauthFunc` field in Config is nil by default — no impact on existing auth flows

### Phase SAML.3: Credential Helper (upstream PR #3)

**Goal:** Add `--credential-cmd` flag for generic credential provider integration.
**Files new:** `pkg/adt/credential_cmd.go`, `pkg/adt/credential_cmd_test.go`
**Files modified:** `cmd/vsp/main.go`
**New deps:** None
**Estimated effort:** 1-1.5 days

- [ ] T3.1: Create `pkg/adt/credential_cmd.go`:
  - `RunCredentialCmd(ctx, args []string) (username, password string, err error)` — execute external command via `exec.Command(args[0], args[1:]...)` (argv-based, no shell). Parse JSON `{"username": "...", "password": "..."}` from stdout.
  - **Security:** argv-based execution by default (like git-credential-helper). No shell interpretation of the command string. This prevents shell injection when `SAP_CREDENTIAL_CMD` is set from config/env.
  - Context-aware timeout (default 30s)
  - Never log stdout/stderr content (contains secrets)
  - Read credential-cmd stdout into `[]byte`; zero buffer after JSON parsing
  - CLI flag accepts space-separated command: `--credential-cmd "keepassxc-cli show -s db.kdbx SAP/K0B"` — split by `strings.Fields()` (no shell quoting support; document limitation). For complex quoting, use repeated `--credential-cmd-arg` flags or wrapper script. Log warning when sourced from env var.
- [ ] T3.2: Add CLI flag `--credential-cmd` / `SAP_CREDENTIAL_CMD` to `cmd/vsp/main.go`
- [ ] T3.3: Wire credential-cmd as credential source for `--saml-auth`: priority order is credential-cmd > env vars > TTY prompt
- [ ] T3.4: Write unit tests:
  - `TestCredentialCmd_ValidJSON` — mock command returning valid JSON
  - `TestCredentialCmd_InvalidJSON` — malformed output
  - `TestCredentialCmd_Timeout` — command exceeds timeout
  - `TestCredentialCmd_NonZeroExit` — command fails
  - `TestCredentialCmd_MissingFields` — JSON missing username or password
- [ ] T3.5: Document usage in README or `--help` output
- [ ] GATE: `go test ./pkg/adt/...` + `go test ./cmd/vsp/...` passes (verify all 5 new test cases) + `mcp__pal__codereview` + `mcp__pal__thinkdeep` — zero MEDIUM+ before next phase

**Rollback:**
1. `git revert <commit>` — new files only, minimal main.go change
2. `--saml-auth` continues to work with env vars or TTY prompt

---

## Security Constraints (ALL phases)

| Constraint | Enforcement |
|------------|------------|
| Never log SAMLRequest/SAMLResponse bodies | Code review + test `TestSAMLAuth_VerboseNoSecrets` |
| Never log passwords or cookie values | Code review + verbose output tests |
| Cookie files use 0600 permissions | Already enforced in `SaveCookiesToFile` |
| KeePass master password: TTY-only | credential-cmd pattern — VSP never sees master password |
| MFA: Option A does not support it | Documented in `--saml-auth` help text + error message |
| Credential lifecycle | CredentialProvider callback re-reads on each auth; `[]byte` zeroed after each use. No long-term retention. |
| credential-cmd: no shell execution | Use `exec.Command(argv...)` not `sh -c`. Prevents shell injection from env/config |
| InsecureSkipVerify covers IAS too | Document in `--saml-auth` help that `--insecure` disables TLS for IAS endpoint |

## Risk Register

| ID | Risk | Severity | Mitigation |
|----|------|----------|------------|
| R1 | IAS form structure changes | MEDIUM | Target SAML-standard fields, golden tests with sanitized HTML fixtures |
| R2 | Cookie URL/path edge cases | LOW | Multiple URLs in `WithURLs()`, defensive fix |
| R3 | `golang.org/x/net` adds upstream friction | MEDIUM | stdlib-adjacent, widely accepted. Alternative: investigate if `cookiejar.New(nil)` sufficient |
| R4 | MFA enforced on test systems | LOW | Phase 1 handles MFA via browser. Phase 2 explicitly documented as no-MFA |
| R5 | credential-cmd injection via shell execution | HIGH | Use argv-based `exec.Command` (no shell). Split CLI flag value with shlex. Log warning when sourced from env/config. |
| R6 | Credential leak in verbose logs | HIGH | Hard rule: never log assertion/password bodies. Enforce via tests + code review |

## Test Systems

| System | Non-API URL | Client | IAS Tenant |
|--------|-------------|--------|------------|
| K0B CUST | my413856.s4hana.cloud.sap | 100 | ahzoesedf.accounts.cloud.sap |
| K0B DEV | my413862.s4hana.cloud.sap | 080 | ahzoesedf.accounts.cloud.sap |

---

## Next Plans

| Phase ID | Title | Status | Goal |
|----------|-------|--------|------|
| PLAN.md Phase 1.1-1.3 | Upstream Merge & Fork Modernization | 📋 PROPOSED | Merge upstream/main, re-add fork code, clean branches |
| PLAN.md Phase 1.4 | AnalyzeABAPCode v2 (abaplint-based) | 📋 PROPOSED | Rewrite code analysis on native Go abaplint parser |
| PLAN.md Phase 1.5 | Refactoring & QuickFix v2 | 📋 PROPOSED | Correct ADT API URLs for rename/quickfix |
| SAML.4 (deferred) | Browser profile reuse (Option B) | ⏸ DEFERRED | `--browser-profile` flag for MFA-heavy environments |
