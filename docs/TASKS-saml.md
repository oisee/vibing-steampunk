# Tasks: SAML Authentication

**Session:** saml
**Plan:** [docs/PLAN-saml.md](PLAN-saml.md)

## Phase SAML.1: Fix Browser Auth for SAML

| Task | Description | Status | Assignee |
|------|-------------|--------|----------|
| T1.1 | Fix `extractSAPCookies` cookie URL filtering | DONE | backend-dev |
| T1.2 | Improve `pollForSAPCookies` SAML timing | DONE | backend-dev |
| T1.3 | Add verbose SAML redirect logging | DONE | backend-dev |
| T1.4 | Write unit tests for cookie filtering logic | DONE | test-engineer |
| T1.5 | Write integration test (chromedp + httptest, build tag) | DONE | test-engineer |
| T1.6 | Manual test against K0B DEV | PENDING | backend-dev |
| GATE | Tests pass + PAL codereview + thinkdeep + /check audit | DONE | orchestrator |

## Phase SAML.2: Programmatic SAML Flow

| Task | Description | Status | Assignee |
|------|-------------|--------|----------|
| T2.1 | Create `saml_auth.go` with `SAMLLogin()` | TODO | backend-dev |
| T2.2 | HTML form parser using `x/net/html` | TODO | backend-dev |
| T2.3 | Add CLI flags to `main.go` | TODO | backend-dev |
| T2.4 | Add `processSAMLAuth()` between browser and cookie auth | TODO | backend-dev |
| T2.5 | Wire 401 re-auth into Transport (ReauthFunc + singleflight) | TODO | backend-dev |
| T2.6 | Write comprehensive unit tests (8 cases) | TODO | test-engineer |
| T2.7 | Manual test against K0B DEV | TODO | backend-dev |
| GATE | Tests pass + PAL codereview + thinkdeep | TODO | orchestrator |

## Phase SAML.3: Credential Helper

| Task | Description | Status | Assignee |
|------|-------------|--------|----------|
| T3.1 | Create `credential_cmd.go` (argv-based exec, []byte zeroing) | TODO | backend-dev |
| T3.2 | Add CLI flag `--credential-cmd` | TODO | backend-dev |
| T3.3 | Wire credential-cmd into SAML auth | TODO | backend-dev |
| T3.4 | Write unit tests (5 cases) | TODO | test-engineer |
| T3.5 | Document usage | TODO | doc-writer |
| GATE | Tests pass + PAL codereview + thinkdeep | TODO | orchestrator |
