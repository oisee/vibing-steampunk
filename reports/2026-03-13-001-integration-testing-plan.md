# vsp — Integration Testing Overhaul & Feature Roadmap

**Date:** 2026-03-13
**Report ID:** 001
**Subject:** Real-system integration test infrastructure, gap analysis, TDD stubs, and high-priority missing features
**Status:** Active — ready for implementation

---

## 1. Executive Summary

**Decision:** Continue with vsp as the single MCP server (not mcp-abap-adt). vsp is ahead on features; mcp-abap-adt's test suite and edge-case handling are the things worth extracting.

**Goals:**
1. Overhaul integration test infrastructure — proper logging, cleanup helpers, no test debris left on SAP
2. Cover ~50 currently-untested client methods with new integration tests
3. Add TDD stubs (show as `SKIP [TODO]`, never `FAIL`) for planned-but-not-yet-implemented features
4. Fix known test bugs (B-001 through B-010)
5. Add 8 high-priority missing tools — kept minimal to avoid token bloat
6. Add JWT/XSUAA auth for ABAP Cloud/BTP support
7. Add SSE/HTTP transport (already in `mcp-go v0.17.0` dependency)

**Current state:** ~35 integration tests, ~22 methods covered
**Target state:** ~85 integration tests, ~55+ methods covered, 8 new tools

---

## 2. SAP ABAP Docker System: Prerequisites

### 2.1 Docker Image Status (March 2026)

> **IMPORTANT — Temporarily unavailable:** SAP removed the ABAP Cloud Developer Trial 2023 from Docker Hub in February 2026. SAP is working on **ABAP Cloud Developer Trial 2025** (ABAP Platform 2025 SP01). Follow [this SAP Community blog post](https://community.sap.com/t5/technology-blog-posts-by-sap/abap-cloud-developer-trial-2023-available-now/ba-p/14057183) for availability updates.

**Alternatives while waiting:**
- [SAP Cloud Appliance Library (CAL)](https://cal.sap.com/catalog#/applianceTemplates/4d7b4410-ee54-4691-9901-4828d05dfc29) — hosted appliance, same product, no local Docker needed
- Check Docker Hub tags: `https://hub.docker.com/r/sapse/abap-cloud-developer-trial/tags` — 2022 tag may still be pullable

### 2.2 System Requirements

| Resource | Minimum | Recommended |
|----------|---------|-------------|
| RAM | 16 GB | 32 GB |
| Disk | 100 GB free | 150 GB |
| CPU | 4 cores | 8 cores |
| OS | Linux / macOS / Windows WSL2 | Linux or macOS |

**macOS M-series (Apple Silicon):** Requires Rosetta emulation — performance is limited. See [M-series guide](https://community.sap.com/t5/technology-blog-posts-by-members/m-series-apple-chip-macbooks-and-abap-trial-containers-using-docker-and/ba-p/13593215).

### 2.3 First Pull & Run

```bash
# Pull (once image is available again)
docker pull sapse/abap-cloud-developer-trial:2023

# First run — creates container named "a4h"
# Takes 15-30 min on first start. Do NOT abort mid-start.
docker run --stop-timeout 7200 \
  -i --name a4h \
  -h vhcala4hci \
  -p 3200:3200 \
  -p 3300:3300 \
  -p 8443:8443 \
  -p 30213:30213 \
  -p 50000:50000 \
  -p 50001:50001 \
  sapse/abap-cloud-developer-trial:2023 \
  -skip-limits-check

# Wait until ADT responds (poll — do in a separate terminal)
until curl -s -u DEVELOPER:Down1oad \
  http://localhost:50000/sap/bc/adt/core/discovery \
  -o /dev/null -w "%{http_code}" | grep -q 200; do
  echo "Waiting for SAP ADT..."; sleep 15
done
echo "SAP is ready!"
```

### 2.4 Daily Start / Stop

```bash
# Start (after first run — much faster, ~2 min)
docker start -ai a4h

# Stop — ALWAYS use graceful stop. Abrupt kill corrupts the container permanently.
docker stop --time 7200 a4h

# Shell into container
docker exec -it a4h bash

# Check container status
docker ps -a | grep a4h
```

### 2.5 Default Credentials

| Parameter | Value |
|-----------|-------|
| User | `DEVELOPER` |
| Password | `Down1oad` |
| Client | `001` |
| System ID | `A4H` |
| ADT URL | `http://localhost:50000` |
| SAP GUI host | `localhost`, system number `00` |

### 2.6 License Renewal (3-month expiry)

The shipped license lasts only 3 months. Renew before expiry:

1. Log on with `SAP*` / `Down1oad` in client `000`
2. Run transaction `SLICENSE` — copy the hardware key
3. Go to [minisap](https://go.support.sap.com/minisap/#/minisap), choose system `A4H`, generate license
4. Back in SLICENSE: **Install** the downloaded file
5. Log off, log back on as `DEVELOPER` / client `001`
6. Delete old invalid licenses if any remain

Alternative via Docker (no SAP GUI needed):
```bash
docker cp A4H.txt a4h:/opt/sap/ASABAP_license
docker start -ai a4h   # applies license on start
```

### 2.7 Verify Connection

```bash
# Quick check
curl -s -u DEVELOPER:Down1oad \
  -H "X-CSRF-Token: Fetch" \
  "http://localhost:50000/sap/bc/adt/core/discovery" \
  -o /dev/null -w "HTTP %{http_code}\n"
# Expected: HTTP 200

# Full ADT header check
curl -v -u DEVELOPER:Down1oad \
  -H "X-CSRF-Token: Fetch" \
  "http://localhost:50000/sap/bc/adt/core/discovery" 2>&1 | \
  grep -E "< HTTP|x-csrf-token|sap-client"
```

### 2.8 Test Environment File

Create once, never commit (already in `.gitignore`):

```bash
cat > .env.test << 'EOF'
export SAP_URL=http://localhost:50000
export SAP_USER=DEVELOPER
export SAP_PASSWORD=Down1oad
export SAP_CLIENT=001
export SAP_LANGUAGE=EN
export SAP_INSECURE=false
export SAP_TEST_VERBOSE=false     # set true for full HTTP logging
export SAP_TEST_NO_CLEANUP=       # set true to keep test objects for inspection
EOF
```

---

## 3. Part A — Integration Test Infrastructure

**File:** `pkg/adt/integration_test.go`

### A1 — `testLogger` Struct

Add after imports. Provides timestamped, levelled output. `Debug` lines are suppressed unless `SAP_TEST_VERBOSE=true`.

```go
type testLogger struct {
    t       *testing.T
    verbose bool
    start   time.Time
}

func newTestLogger(t *testing.T) *testLogger {
    t.Helper()
    return &testLogger{
        t:       t,
        verbose: os.Getenv("SAP_TEST_VERBOSE") == "true",
        start:   time.Now(),
    }
}

func (l *testLogger) Info(format string, args ...any) {
    l.t.Helper()
    l.t.Logf("[INFO  +%6v] %s", time.Since(l.start).Round(time.Millisecond), fmt.Sprintf(format, args...))
}

func (l *testLogger) Debug(format string, args ...any) {
    if !l.verbose {
        return
    }
    l.t.Helper()
    l.t.Logf("[DEBUG +%6v] %s", time.Since(l.start).Round(time.Millisecond), fmt.Sprintf(format, args...))
}

func (l *testLogger) Warn(format string, args ...any) {
    l.t.Helper()
    l.t.Logf("[WARN  +%6v] %s", time.Since(l.start).Round(time.Millisecond), fmt.Sprintf(format, args...))
}

// Todo marks a test as intentionally unimplemented.
// Shows as SKIP in output, never FAIL — safe for CI.
func (l *testLogger) Todo(feature string) {
    l.t.Helper()
    l.t.Skipf("[TODO ] %s", feature)
}
```

### A2 — `requireIntegrationClient`

Replace `getIntegrationClient` with a version that logs connection details and uses a longer timeout. Keep the old name as an alias so existing tests don't need touching immediately.

```go
func requireIntegrationClient(t *testing.T) *Client {
    t.Helper()
    url  := os.Getenv("SAP_URL")
    user := os.Getenv("SAP_USER")
    pass := os.Getenv("SAP_PASSWORD")
    if url == "" || user == "" || pass == "" {
        t.Skip("Integration tests require SAP_URL, SAP_USER, SAP_PASSWORD — see reports/2026-03-13-001-integration-testing-plan.md")
    }
    client   := os.Getenv("SAP_CLIENT")
    if client == "" { client = "001" }
    lang := os.Getenv("SAP_LANGUAGE")
    if lang == "" { lang = "EN" }

    log := newTestLogger(t)
    log.Info("Connecting: url=%s user=%s client=%s", url, user, client)

    opts := []Option{
        WithClient(client),
        WithLanguage(lang),
        WithTimeout(60 * time.Second),   // 30s was too short for activate/ATC
    }
    if os.Getenv("SAP_INSECURE") == "true" {
        opts = append(opts, WithInsecureSkipVerify())
    }
    return NewClient(url, user, pass, opts...)
}

// Alias — lets existing tests continue to compile unchanged.
func getIntegrationClient(t *testing.T) *Client { return requireIntegrationClient(t) }
```

### A3 — Object Helpers

```go
// tempObjectName returns a name guaranteed not to collide across parallel runs.
// Example: "ZTEST_PROG_42317"
func tempObjectName(base string) string {
    return fmt.Sprintf("%s_%05d", base, time.Now().Unix()%100000)
}

// withTempProgram creates a report program in $TMP, calls fn with its ADT URL,
// then deletes it via t.Cleanup even if the test panics.
// Set SAP_TEST_NO_CLEANUP=true to keep objects for manual inspection.
func withTempProgram(t *testing.T, client *Client, name, source string, fn func(objectURL string)) {
    t.Helper()
    ctx := context.Background()
    log := newTestLogger(t)

    // Create
    result, err := client.CreateProgram(ctx, name, "Integration test", "$TMP", source, "")
    if err != nil {
        t.Fatalf("withTempProgram: create %s: %v", name, err)
    }
    log.Info("Created %s at %s", name, result.ObjectURL)

    // Cleanup
    t.Cleanup(func() {
        if os.Getenv("SAP_TEST_NO_CLEANUP") == "true" {
            log.Warn("Cleanup skipped (SAP_TEST_NO_CLEANUP=true) — delete %s manually", name)
            return
        }
        ctx2, cancel := context.WithTimeout(context.Background(), 15*time.Second)
        defer cancel()
        lock, err := client.LockObject(ctx2, result.ObjectURL, "MODIFY")
        if err != nil {
            log.Warn("Cleanup: lock %s failed: %v", name, err)
            return
        }
        if err := client.DeleteObject(ctx2, result.ObjectURL, lock.Handle, ""); err != nil {
            log.Warn("Cleanup: delete %s failed: %v", name, err)
        } else {
            log.Info("Cleanup: deleted %s", name)
        }
    })

    fn(result.ObjectURL)
}
```

Same pattern for `withTempClass`.

### A4 — Bug Fixes in Existing Tests

| Bug | Test | Fix |
|-----|------|-----|
| **B-004** | `TestIntegration_LockUnlock` | Create a `$TMP` program inside the test instead of locking `SAPMSSY0` (standard program — risky, may conflict with other users) |
| **B-008** | `TestIntegration_ActivateObject` | Parse activation result XML for `<msg:message severity="W">` — currently ignores warnings that indicate partial failure |
| **B-010** | `TestIntegration_GetClass` | Assert all 5 include keys present (`main`, `localDef`, `localImp`, `macros`, `testclasses`), log which are missing rather than silently passing |

### A5 — New Tests for Currently-Untested Methods

~50 new tests. All follow: `t.Parallel()` (where safe), `requireIntegrationClient`, `newTestLogger`, graceful `t.Skipf` on not-found, full field logging at DEBUG level.

**Stable SAP objects** used as read-only test fixtures (present in any ABAP system):

| Object | Type | Purpose |
|--------|------|---------|
| `IF_SERIALIZABLE_OBJECT` | Interface | Read interface source |
| `RFC_PING` | Function Module | Read FM source |
| `SYST` | Structure | Read DDIC structure |
| `DD02V` | View | Read DDIC view |
| `T000` | Table | Table contents/query tests |
| `CL_ABAP_TYPEDESCR` | Class | Class components, type hierarchy |
| `BASIS` | Function Group | Function group source |
| `SE38` | Transaction | Transaction source |

**New test list:**

| Test Function | Method Under Test | Fixture / Notes |
|---------------|-------------------|-----------------|
| `TestIntegration_GetInterface` | `GetInterface` | `IF_SERIALIZABLE_OBJECT` |
| `TestIntegration_GetFunctionGroup` | `GetFunctionGroup` | `BASIS` |
| `TestIntegration_GetFunction` | `GetFunction` | `RFC_PING` |
| `TestIntegration_GetView` | `GetView` | `DD02V` |
| `TestIntegration_GetStructure` | `GetStructure` | `SYST` |
| `TestIntegration_GetSystemInfo` | `GetSystemInfo` | Assert SystemID, Release non-empty |
| `TestIntegration_GetInstalledComponents` | `GetInstalledComponents` | Assert len > 0; log all names |
| `TestIntegration_GetClassComponents` | `GetClassComponents` | `CL_ABAP_TYPEDESCR` |
| `TestIntegration_GetClassInfo` | `GetClassInfo` | `CL_ABAP_TYPEDESCR` |
| `TestIntegration_GetTypeHierarchy` | `GetTypeHierarchy` | `CL_ABAP_TYPEDESCR` supertypes |
| `TestIntegration_FindDefinition` | `FindDefinition` | Known class method reference |
| `TestIntegration_FindReferences` | `FindReferences` | Known symbol in standard class |
| `TestIntegration_GetInactiveObjects` | `GetInactiveObjects` | Assert no error (may be empty list) |
| `TestIntegration_PrettyPrint` | `PrettyPrint` | Create temp program with bad indent; verify cleaned |
| `TestIntegration_RunATCCheck` | `RunATCCheck` + `GetATCWorklist` | Create program with known ATC finding (e.g. unused variable); assert result non-empty |
| `TestIntegration_GetCallGraph` | `GetCallGraph` | Known class URI; assert edges non-empty |
| `TestIntegration_ExecuteABAP` | `ExecuteABAP` | `DATA lv_x TYPE i. lv_x = 42.` — assert no error |
| `TestIntegration_GrepPackage` | `GrepPackage` | Create temp program containing `"UNIQUE_STRING_42317"`; grep for it; assert found |
| `TestIntegration_EditSource_HappyPath` | `EditSource` | Create temp program; replace known string; read back and verify |
| `TestIntegration_EditSource_PatternNotFound` | `EditSource` | **(B-001)** Expect explicit error on missing pattern, not silent no-op |
| `TestIntegration_GetTableContents_Empty` | `GetTableContents` | **(B-007)** `WHERE MANDT = 'IMPOSSIBLE'` — assert empty rows, no error |
| `TestIntegration_GetClassInclude_AllTypes` | `GetClassInclude` | **(B-010)** Create temp class; assert all 5 include types readable |
| `TestIntegration_UpdateClassInclude` | `UpdateClassInclude` | Write to `localDef` include of temp class; read back and verify |
| `TestIntegration_CompareSource` | `CompareSource` | Two standard programs with known differences |
| `TestIntegration_ListDumps` | `GetDumps` | Assert no error (result may be empty) |
| `TestIntegration_ListTraces` | `ListTraces` | Assert no error |
| `TestIntegration_GetSQLTraceState` | `GetSQLTraceState` | Assert no error |
| `TestIntegration_TransportLifecycle` | `GetUserTransports` + `CreateTransport` + `GetTransport` + `DeleteTransport` | Create and delete a transport; NO release (destructive) |
| `TestIntegration_GetTransportInfo` | `GetTransportInfo` | Pass a `$TMP` object URL; verify response fields |
| `TestIntegration_ListTransports` | `ListTransports` | Assert no error; log count |

### A6 — TDD Stubs for Planned Features

These compile and run but call `l.Todo(...)` which calls `t.Skip`. They appear as `SKIP [TODO]` — visible in logs, never failing, give Claude a direct checklist.

```go
func TestIntegration_GetDomain(t *testing.T) {
    newTestLogger(t).Todo("GetDomain not yet implemented — ADT: /sap/bc/adt/ddic/domains/{name}/source/main")
}
func TestIntegration_GetDataElement(t *testing.T) {
    newTestLogger(t).Todo("GetDataElement not yet implemented — ADT: /sap/bc/adt/ddic/dataelements/{name}/source/main")
}
func TestIntegration_GetWhereUsed(t *testing.T) {
    newTestLogger(t).Todo("GetWhereUsed not yet implemented — ADT: GET /sap/bc/adt/repository/informationsystem/usageReferences?objectName=&objectType=")
}
func TestIntegration_GetProgFullCode(t *testing.T) {
    newTestLogger(t).Todo("GetProgFullCode not yet implemented — composite: GetProgram + recursive INCLUDE resolution")
}
func TestIntegration_ListObjects(t *testing.T) {
    newTestLogger(t).Todo("ListObjects not yet implemented — extends nodestructure with parent_type param")
}
func TestIntegration_GetEnhancements(t *testing.T) {
    newTestLogger(t).Todo("GetEnhancements not yet implemented — ADT: GET /sap/bc/adt/{type}/{name}/source/main/enhancements/elements")
}
func TestIntegration_GetEnhancementSpot(t *testing.T) {
    newTestLogger(t).Todo("GetEnhancementSpot not yet implemented — ADT: GET /sap/bc/adt/enhancements/enhsxsb/{spot}")
}
func TestIntegration_CDSUnitTests(t *testing.T) {
    newTestLogger(t).Todo("CDS Unit Test runner not yet implemented — ADT: POST /sap/bc/adt/abapunit/results/testsuite (multi-step lifecycle)")
}
func TestIntegration_JWTAuth(t *testing.T) {
    newTestLogger(t).Todo("JWT/XSUAA auth not yet implemented — SAP_AUTH_TYPE=jwt + SAP_JWT_TOKEN")
}
```

---

## 4. Part B — Eight New ADT Features

Keeping tool count minimal (8 new tools). Each merges overlapping functionality where possible.

### B1 — `GetDomain` + `GetDataElement`

**File:** `pkg/adt/client.go`
**Pattern:** identical to existing `GetView` / `GetStructure` — 15 lines each.

```go
// GetDomain retrieves the source definition of a Data Domain.
// ADT endpoint: GET /sap/bc/adt/ddic/domains/{name}/source/main
func (c *Client) GetDomain(ctx context.Context, name string) (string, error) {
    path := fmt.Sprintf("/sap/bc/adt/ddic/domains/%s/source/main", strings.ToUpper(name))
    resp, err := c.transport.Request(ctx, path, &RequestOptions{
        Method: http.MethodGet,
        Accept: "text/plain",
    })
    if err != nil {
        return "", fmt.Errorf("GetDomain %s: %w", name, err)
    }
    return string(resp.Body), nil
}

// GetDataElement retrieves the source definition of a Data Element.
// ADT endpoint: GET /sap/bc/adt/ddic/dataelements/{name}/source/main
func (c *Client) GetDataElement(ctx context.Context, name string) (string, error) {
    path := fmt.Sprintf("/sap/bc/adt/ddic/dataelements/%s/source/main", strings.ToUpper(name))
    resp, err := c.transport.Request(ctx, path, &RequestOptions{
        Method: http.MethodGet,
        Accept: "text/plain",
    })
    if err != nil {
        return "", fmt.Errorf("GetDataElement %s: %w", name, err)
    }
    return string(resp.Body), nil
}
```

**MCP tools:** `GetDomain`, `GetDataElement` — both take `name` (required string). Add to **focused** mode.

---

### B2 — `GetWhereUsed`

**File:** `pkg/adt/codeintel.go`
**Note:** Different from `FindReferences` (which is a cursor-based POST for in-source navigation). `GetWhereUsed` is an object-level search — "which objects call/reference this object?" Very high value for impact analysis.

```go
// GetWhereUsed returns all objects that reference the given ADT object.
// Distinct from FindReferences: this is object-level (no source cursor needed).
// ADT endpoint: GET /sap/bc/adt/repository/informationsystem/usageReferences
//   ?objectName={name}&objectType={type}&includeSubclasses={bool}
func (c *Client) GetWhereUsed(ctx context.Context, objectName, objectType string, includeSubclasses bool) ([]UsageReference, error) {
    params := url.Values{}
    params.Set("objectName", strings.ToUpper(objectName))
    if objectType != "" {
        params.Set("objectType", strings.ToUpper(objectType))
    }
    if includeSubclasses {
        params.Set("includeSubclasses", "true")
    }
    resp, err := c.transport.Request(ctx, "/sap/bc/adt/repository/informationsystem/usageReferences", &RequestOptions{
        Method: http.MethodGet,
        Query:  params,
        Accept: "application/xml",
    })
    if err != nil {
        return nil, fmt.Errorf("GetWhereUsed %s: %w", objectName, err)
    }
    return parseUsageReferences(resp.Body)   // reuses existing parser
}
```

**MCP tool:** `GetWhereUsed` — params: `object_name` (required), `object_type` (optional, e.g. `"CLAS/OC"`), `include_subclasses` (bool). Add to **focused** mode.

---

### B3 — `GetProgFullCode`

**File:** `pkg/adt/client.go` (or `workflows.go`)
**Purpose:** Fetch a program's complete source including all INCLUDE programs, recursively. Avoids the LLM having to call `GetProgram` + multiple `GetSource` calls just to see a full program.

**Types:**
```go
type ProgCodeObject struct {
    Name   string
    Source string
}

type ProgFullCode struct {
    Name    string
    Objects []ProgCodeObject   // index 0 = main program, rest = includes in resolution order
}
```

**Algorithm:**
1. `GetProgram(ctx, name)` → main source
2. Scan source for INCLUDE statements using regex:
   - Single: `(?im)^\s*INCLUDE\s+(\w+)\s*\.`
   - List:   `(?im)^\s*INCLUDE\s*:\s*((?:\w+\s*,?\s*)+)\.`
3. For each found include name: `GetInclude(ctx, includeName)` (or `GetSource` with type `PROG/I`)
4. Recursively scan each include (same regex, deduplicate via `visited map[string]bool`)
5. Max depth: 10 (prevent infinite loops from rare circular INCLUDEs)
6. Return `ProgFullCode` with main + all includes in BFS order

**MCP tool:** `GetProgFullCode` — param: `program_name` (required). Add to **focused** mode.

---

### B4 — `ListObjects`

**File:** `pkg/adt/client.go`
**Purpose:** Merges three separate mcp-abap-adt tools (`GetObjectsByType`, `GetObjectsList`, `GetObjectInfo`) into **one** — minimizes tool count and token use. Extends the existing `GetPackage` infrastructure (`/sap/bc/adt/repository/nodestructure` POST).

**Types:**
```go
type ListObjectsOptions struct {
    ParentType string   // required: "DEVC/K", "CLAS/OC", "PROG/P", "FUGR/F", etc.
    ParentName string   // required: e.g. "$TMP", "ZCL_MY_CLASS"
    NodeID     string   // optional: specific sub-node ID from a previous call
    WithDesc   bool     // include short descriptions (slightly slower)
    MaxDepth   int      // 0 = flat listing only (default); 1+ = recursive sub-nodes
}

type ObjectNode struct {
    Name        string
    Type        string   // e.g. "CLAS/OC"
    TechName    string
    URI         string   // ADT URI for direct use in other tools
    Description string
    Package     string
    Children    []ObjectNode   // populated only if MaxDepth > 0
}
```

**Implementation:** Reuse `parsePackageNodeStructure` already used by `GetPackage`. Add recursion loop for `MaxDepth > 0`.

**MCP tool:** `ListObjects` — params: `parent_type` (required), `parent_name` (required), `node_id` (optional), `max_depth` (optional, default 0). Add to **focused** mode.

---

### B5 — `GetEnhancements` + `GetEnhancementSpot`

**File:** `pkg/adt/client.go`
**Purpose:** Critical for understanding SAP customizations. Enhancements (BAdI implementations, implicit/explicit enhancement spots) are how SAP systems are customized without modifying standard code.

```go
type Enhancement struct {
    Name        string
    SpotName    string
    Type        string   // "BADI_IMPL", "HOOK_IMPL", "WDCOMP_ENH", etc.
    ShortText   string
    Source      string
    Active      bool
}

type EnhancementSpot struct {
    Name        string
    Type        string
    ShortText   string
    BADIs       []BADIDefinition
}

type BADIDefinition struct {
    Name        string
    ShortText   string
    Interface   string
    IsFilter    bool
    IsMultiUse  bool
}

// GetEnhancements returns all enhancement implementations on a given object.
// ADT endpoint: GET /sap/bc/adt/{objectType}/{objectName}/source/main/enhancements/elements
func (c *Client) GetEnhancements(ctx context.Context, objectName, objectType string) ([]Enhancement, error)

// GetEnhancementSpot returns metadata and BAdI definitions for an enhancement spot.
// ADT endpoint: GET /sap/bc/adt/enhancements/enhsxsb/{spotName}
func (c *Client) GetEnhancementSpot(ctx context.Context, spotName string) (*EnhancementSpot, error)
```

Response format: XML with `<enh:source>` blocks. Source may be base64-encoded.

**MCP tools:** `GetEnhancements` (params: `object_name`, `object_type`), `GetEnhancementSpot` (param: `spot_name`). Add both to **focused** mode.

---

## 5. Part C — Infrastructure

### C1 — JWT / XSUAA Authentication

Needed for ABAP Cloud / BTP environments that use OAuth 2.0 token-based auth instead of Basic.

**`pkg/adt/config.go`** — extend `Config` struct:
```go
// AuthType controls which authentication scheme is used.
// "basic" (default): HTTP Basic Auth with Username/Password
// "jwt":             Bearer token (ABAP Cloud / BTP / XSUAA)
// "cookies":         Cookie-based auth (existing SAP_COOKIE_STRING/FILE)
AuthType  string
JWTToken  string
```

New options:
```go
func WithJWTToken(token string) Option {
    return func(c *Config) {
        c.AuthType  = "jwt"
        c.JWTToken  = token
    }
}
func WithAuthType(t string) Option {
    return func(c *Config) { c.AuthType = t }
}
```

**`pkg/adt/http.go`** — in the request builder where auth headers are added:
```go
switch c.config.AuthType {
case "jwt":
    req.Header.Set("Authorization", "Bearer "+c.config.JWTToken)
case "cookies":
    // existing cookie injection logic — unchanged
default:  // "basic" or ""
    req.SetBasicAuth(c.config.Username, c.config.Password)
}
```

**`cmd/vsp/main.go`** — new flags:
```
--auth-type   SAP_AUTH_TYPE   (default: "basic")
--jwt-token   SAP_JWT_TOKEN
```

**`internal/mcp/server.go`** — pass through to `adt.NewClient`:
```go
if cfg.AuthType == "jwt" {
    adtOpts = append(adtOpts, adt.WithJWTToken(cfg.JWTToken))
}
```

**CLAUDE.md update** — add to configuration table:
```
SAP_AUTH_TYPE / --auth-type     Auth scheme: basic (default), jwt, cookies
SAP_JWT_TOKEN / --jwt-token     Bearer token for JWT/XSUAA auth
```

---

### C2 — SSE / HTTP Transport

Allows vsp to run as a persistent HTTP server instead of a stdio subprocess. Useful for shared deployments (single vsp instance, multiple MCP clients). `mcp-go v0.17.0` (already in `go.mod`) ships `server.NewSSEServer()` — confirmed present at `server/sse.go`.

**`cmd/vsp/main.go`** — new flags + transport switch:
```go
// New flags
--transport    stdio | sse     (default: stdio)
--sse-host     SAP_SSE_HOST    (default: 127.0.0.1)
--sse-port     SAP_SSE_PORT    (default: 3001)

// In Run():
switch cfg.Transport {
case "sse":
    sseServer := mcpserver.NewSSEServer(
        s.mcpServer,
        mcpserver.WithPort(cfg.SSEPort),
        mcpserver.WithBaseURL(fmt.Sprintf("http://%s:%d", cfg.SSEHost, cfg.SSEPort)),
    )
    log.Printf("vsp SSE server listening on http://%s:%d", cfg.SSEHost, cfg.SSEPort)
    return sseServer.Start(ctx)
default: // "stdio"
    return s.mcpServer.Serve(ctx, os.Stdin, os.Stdout)
}
```

No new MCP tools — pure transport layer.

---

## 6. Known Bugs — Regression Test Coverage

| ID | Description | Where | Test |
|----|-------------|-------|------|
| B-001 | `EditSource`: behavior undefined when search pattern not found — may silently no-op | `crud.go` | `TestIntegration_EditSource_PatternNotFound` — assert explicit error |
| B-002 | `GetDomain` / `GetDataElement` not in `client.go` | `client.go` | Phase 5 TDD stub |
| B-003 | `CreateObject` only tested for PROG and CLAS types | `crud.go` | Phase 3 — add INTF, FUGR, TABL variants |
| B-004 | `TestIntegration_LockUnlock` locks `SAPMSSY0` (standard program) — conflicts risk | `integration_test.go` | Fix: use temp `$TMP` program |
| B-005 | `DebuggerListener` test always times out; no meaningful assertion | `integration_test.go` | Keep as skip with `t.Skipf("requires live debugger session")` |
| B-006 | `ListTransports` SQL fallback returns different structure than normal path | `client.go` | `TestIntegration_TransportLifecycle` — assert consistent field set |
| B-007 | `GetTableContents`: no test for 0-row result or impossible WHERE | `integration_test.go` | `TestIntegration_GetTableContents_Empty` |
| B-008 | `ActivateObject`: result XML warnings never checked | `devtools.go` | Fix test to parse and log `<msg:message severity="W">` |
| B-009 | `WriteSource`: CRLF/LF round-trip not tested | `crud.go` | `TestIntegration_WriteSource_CRLFRoundtrip` |
| B-010 | `GetClass`: not all 5 include types validated | `integration_test.go` | `TestIntegration_GetClassInclude_AllTypes` |

---

## 7. Coverage Summary

| Category | Current | Target | New Tests |
|----------|---------|--------|-----------|
| Basic read (PROG, CLAS, INTF, FUGR, FM) | 2/5 | 5/5 | +3 |
| DDIC (TABL, VIEW, STRU) | 1/3 | 3/3 | +2 |
| DDIC missing (DOMA, DTEL) | 0 — not impl. | stubs | spec only |
| RAP objects (DDLS, BDEF, SRVD, SRVB) | 3/4 | 4/4 | +1 |
| CRUD full lifecycle | PROG, CLAS | +INTF, FUGR, TABL | +3 |
| Class includes (all 5 types) | partial | all 5 | +3 |
| Code intelligence (def, refs, hierarchy) | 4/7 | 7/7 | +3 |
| Transport management | 0/8 | 5/8 (safe ops) | +5 |
| ATC / quality | 0/4 | 4/4 | +4 |
| Runtime diagnostics (dumps, traces) | 0/5 | 5/5 | +5 |
| ExecuteABAP | 0/1 | 1/1 | +1 |
| GrepPackage / GrepObjects | 0/4 | 4/4 | +4 |
| System / Features | 0/3 | 3/3 | +3 |
| EditSource edge cases | 0/2 | 2/2 | +2 |
| New tools (stubs) | 0 | 8 stubs | +8 |
| **TOTAL** | **~35** | **~85** | **+50** |

---

## 8. Files to Modify

| File | Change |
|------|--------|
| `pkg/adt/integration_test.go` | Full overhaul: `testLogger`, `requireIntegrationClient`, helpers, ~50 new tests, 9 TDD stubs, B-004/B-008/B-010 fixes |
| `pkg/adt/client.go` | Add `GetDomain`, `GetDataElement`, `GetProgFullCode`, `ListObjects`, `GetEnhancements`, `GetEnhancementSpot` |
| `pkg/adt/codeintel.go` | Add `GetWhereUsed` |
| `pkg/adt/config.go` | Add `AuthType`, `JWTToken` fields; add `WithJWTToken`, `WithAuthType` options |
| `pkg/adt/http.go` | Add JWT Bearer path in auth header logic |
| `internal/mcp/server.go` | Register 8 new tools; JWT auth config passthrough; update focused-mode whitelist |
| `cmd/vsp/main.go` | Add `--auth-type`, `--jwt-token`, `--transport`, `--sse-host`, `--sse-port` flags |
| `CLAUDE.md` | Update config table with new flags |

---

## 9. Implementation Order

1. **Part A** — Test infrastructure + all new tests + stubs in `integration_test.go`. Run: see all new tests either PASS (read-only), SKIP (TODO), or FAIL (bugs to fix). Stubs give a live checklist.
2. **Part B1** — `GetDomain` + `GetDataElement` (5 min each — copy pattern from `GetView`)
3. **Part B2** — `GetWhereUsed` (reuses existing `UsageReference` type + parser)
4. **Part B3** — `GetProgFullCode` (new regex + recursion logic)
5. **Part B4** — `ListObjects` (extends `GetPackage` nodestructure infrastructure)
6. **Part B5** — `GetEnhancements` + `GetEnhancementSpot` (new XML types needed)
7. **Part C1** — JWT auth (config + http.go + CLI flags)
8. **Part C2** — SSE transport (main.go switch + flags)

Each Part B step: convert the TDD stub from `l.Todo(...)` to a real test → run → confirm SKIP changes to FAIL → implement → confirm PASS.

---

## 10. Quick Start Guide

### One-Time Setup

```bash
# 1. Create .env.test (already in .gitignore)
cat > .env.test << 'EOF'
export SAP_URL=http://localhost:50000
export SAP_USER=DEVELOPER
export SAP_PASSWORD=Down1oad
export SAP_CLIENT=001
export SAP_LANGUAGE=EN
EOF

# 2. Load it
source .env.test

# 3. Verify SAP is up
curl -s -u "$SAP_USER:$SAP_PASSWORD" \
  "$SAP_URL/sap/bc/adt/core/discovery" -o /dev/null -w "HTTP %{http_code}\n"
# → HTTP 200
```

### Run Tests

```bash
# Fast — unit tests only, no SAP needed
go test ./...

# Read-only integration tests — safe, ~30s
source .env.test
go test -tags=integration -v -timeout 120s ./pkg/adt/ \
  -run "TestIntegration_(Search|GetProgram|GetClass|GetInterface|GetSystem|GetInstalled)" \
  2>&1 | tee reports/test-readonly-$(date +%Y%m%d-%H%M).log

# Full integration suite — ~5-10 min
go test -tags=integration -v -timeout 600s ./pkg/adt/ \
  2>&1 | tee reports/test-full-$(date +%Y%m%d-%H%M).log
```

### Debug a Failing Test

```bash
# Maximum log output for one test
SAP_TEST_VERBOSE=true \
go test -tags=integration -v -timeout 120s ./pkg/adt/ \
  -run TestIntegration_YourFailingTest \
  2>&1 | tee reports/debug-$(date +%Y%m%d-%H%M).log

# Keep test objects on SAP for manual inspection in Eclipse
SAP_TEST_NO_CLEANUP=true SAP_TEST_VERBOSE=true \
go test -tags=integration -v ./pkg/adt/ \
  -run TestIntegration_YourFailingTest
# Remember: delete test objects manually afterward (prefix ZTEST_*)
```

### Reading the Output

```
--- PASS:  TestIntegration_GetInterface (0.05s)       ← works
    [INFO    +45ms] Got 384 chars of interface source

--- SKIP:  TestIntegration_GetDomain (0.00s)          ← not implemented yet
    [TODO ] GetDomain not yet implemented — ADT: /sap/bc/adt/ddic/domains/...

--- FAIL:  TestIntegration_EditSource_PatternNotFound  ← bug to fix
    [WARN  +120ms] EditSource returned nil, expected error

--- SKIP:  TestIntegration_RunATCCheck (0.01s)        ← env var missing or ATC not configured
    integration_test.go:XXX: Integration tests require SAP_URL...
```

- `PASS` = working correctly
- `SKIP [TODO]` = planned feature, not yet implemented
- `SKIP` (no TODO) = env not set or fixture not found on this system
- `FAIL` = implemented but broken — needs a fix

### Summarise a Run for Claude

```bash
# Paste this output when reporting failures
{
  echo "=== vsp Integration Test Summary ==="
  echo "Date: $(date)"
  echo "SAP_URL: $SAP_URL"
  go test -tags=integration ./pkg/adt/ 2>&1 | grep -E "^(ok|FAIL|---)" | tail -20
  echo ""
  echo "=== FAILures ==="
  grep "^--- FAIL" reports/test-full-*.log 2>/dev/null | tail -20
  echo ""
  echo "=== TODO stubs (unimplemented) ==="
  grep "\[TODO \]" reports/test-full-*.log 2>/dev/null | sed 's/.*\[TODO \]//'
} 2>&1 | tee /tmp/vsp-summary.txt
cat /tmp/vsp-summary.txt
```

### Common Failure Diagnoses

| Symptom | Cause | Fix |
|---------|-------|-----|
| All SKIP — "requires SAP_URL" | Env not loaded | `source .env.test` |
| `connection refused` | SAP not started | `docker start -ai a4h`, wait 2 min |
| `HTTP 401` | Wrong credentials | Check `SAP_USER` / `SAP_PASSWORD` |
| `HTTP 403` CSRF | Token stale | Auto-handled by transport; retry once; if persists check session |
| `HTTP 404` on specific object | Object not on your system | Test skips gracefully — that's expected |
| Test leaves `ZTEST_*` objects | Cleanup failed | Run `SAP_TEST_VERBOSE=true` to see WARN; delete via Eclipse or `se80` |
| Test hangs | Timeout too low | Add `-timeout 300s` |
| License error in SAP GUI | 3-month license expired | Renew via [minisap](https://go.support.sap.com/minisap/#/minisap) — choose system A4H |

---

*Sources:*
- *[ABAP Cloud Developer Trial 2023 — SAP Community](https://community.sap.com/t5/technology-blog-posts-by-sap/abap-cloud-developer-trial-2023-available-now/ba-p/14057183)*
- *[ABAP Trial Platform Docker FAQ](https://github.com/SAP-docs/abap-platform-trial-image/blob/main/faq-v7.md)*
- *Codebase exploration: `pkg/adt/client.go`, `pkg/adt/codeintel.go`, `pkg/adt/config.go`, `pkg/adt/http.go`, `pkg/adt/integration_test.go`, `cmd/vsp/main.go`*
- *mcp-go v0.17.0 — `NewSSEServer` confirmed at `server/sse.go`*
