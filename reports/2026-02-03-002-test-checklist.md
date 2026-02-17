# Test Checklist: Transportable Edits Safety Feature

**Date:** 2026-02-03
**Related Report:** 2026-02-03-002-transportable-edits-safety-feature.md

---

## Pre-Test Setup

### Build vsp
```bash
cd /home/alice/dev/vibing-steampunk
go build -o vsp ./cmd/vsp
```

### SAP Connection
```bash
export SAP_URL=http://a4h.desude.su:50000
export SAP_USER=AVINOGRADOVA
export SAP_PASSWORD=<password>
export SAP_CLIENT=001
```

### Test Objects Available
| Object | Type | Package | Transport |
|--------|------|---------|-----------|
| ZPROD_001_TEST | Program | ZPROD | A4HK900114 |
| ZPROD_002_TEST | Program | ZPROD | A4HK900114 |
| ZCL_PROD_TEST | Class | ZPROD | A4HK900114 |

---

## Test Cases

### Test 1: Default Mode - Edit Local Object (Baseline)
**Command:**
```bash
./vsp
```
**Action:** EditSource on $TMP object without transport
**Expected:** SUCCESS
**Status:** [ ] Not tested

---

### Test 2: Default Mode - Edit with Transport (SHOULD BLOCK)
**Command:**
```bash
./vsp
```
**Action:**
```json
EditSource(
  object_url: "/sap/bc/adt/programs/programs/zprod_001_test",
  old_string: "* Test edit with transport - added by vsp",
  new_string: "* Test edit - attempt without flag",
  transport: "A4HK900114"
)
```
**Expected:** ERROR containing:
- "editing transportable objects is disabled"
- "--allow-transportable-edits"
- "SAP_ALLOW_TRANSPORTABLE_EDITS"
**Status:** [ ] Not tested

---

### Test 3: With Flag - Edit with Transport (SHOULD WORK)
**Command:**
```bash
./vsp --allow-transportable-edits
```
**Action:** Same EditSource as Test 2
**Expected:** SUCCESS - edit applied
**Status:** [ ] Not tested

---

### Test 4: With Flag + Wrong Whitelist (SHOULD BLOCK)
**Command:**
```bash
./vsp --allow-transportable-edits --allowed-transports "DEVK*"
```
**Action:** EditSource with transport="A4HK900114"
**Expected:** ERROR - transport not in whitelist
**Status:** [ ] Not tested

---

### Test 5: With Flag + Correct Whitelist (SHOULD WORK)
**Command:**
```bash
./vsp --allow-transportable-edits --allowed-transports "A4HK*"
```
**Action:** EditSource with transport="A4HK900114"
**Expected:** SUCCESS
**Status:** [ ] Not tested

---

### Test 6: WriteSource with Transport - Default Mode (SHOULD BLOCK)
**Command:**
```bash
./vsp
```
**Action:**
```json
WriteSource(
  object_type: "PROG",
  name: "ZPROD_003_TEST",
  source: "REPORT ZPROD_003_TEST.",
  package: "ZPROD",
  transport: "A4HK900114"
)
```
**Expected:** ERROR - blocked by safety
**Status:** [ ] Not tested

---

### Test 7: WriteSource with Transport - With Flag (SHOULD WORK)
**Command:**
```bash
./vsp --allow-transportable-edits
```
**Action:** Same WriteSource as Test 6
**Expected:** SUCCESS - program created
**Status:** [ ] Not tested

---

### Test 8: Verbose Output Check
**Command:**
```bash
./vsp --allow-transportable-edits --verbose
```
**Expected:** Should show:
```
[VERBOSE] Safety: Transportable edits ENABLED (can modify non-local objects)
```
**Status:** [ ] Not tested

---

### Test 9: Environment Variable
**Command:**
```bash
SAP_ALLOW_TRANSPORTABLE_EDITS=true ./vsp --verbose
```
**Expected:** Same as Test 8
**Status:** [ ] Not tested

---

### Test 10: Package Restriction + Transportable Edits
**Command:**
```bash
./vsp --allow-transportable-edits --allowed-packages "ZPROD"
```
**Action:** Try to edit object in different package
**Expected:** ERROR - package not allowed
**Status:** [ ] Not tested

---

## Tool Visibility Tests (IMPLEMENTED)

### Test 11: ListTransports Blocked by Default
**Command:**
```bash
./vsp
```
**Action:** Call ListTransports
**Expected:** ERROR - "transports not enabled"
**Status:** [ ] Not tested

---

### Test 12: ListTransports Works with --allow-transportable-edits
**Command:**
```bash
./vsp --allow-transportable-edits
```
**Action:** Call ListTransports
**Expected:** SUCCESS - shows list of transports
**Status:** [ ] Not tested

---

### Test 13: GetTransport Works with --allow-transportable-edits
**Command:**
```bash
./vsp --allow-transportable-edits
```
**Action:** Call GetTransport with transport="A4HK900114"
**Expected:** SUCCESS - shows transport details
**Status:** [ ] Not tested

---

### Test 14: CreateTransport Still Requires --enable-transports
**Command:**
```bash
./vsp --allow-transportable-edits
```
**Action:** Call CreateTransport
**Expected:** ERROR - "requires --enable-transports flag"
**Status:** [ ] Not tested

---

## Post-Test Cleanup

### Objects to Review
- [ ] ZPROD_001_TEST - may have test edits
- [ ] ZPROD_002_TEST - created during testing
- [ ] ZPROD_003_TEST - may be created in Test 7
- [ ] ZCL_PROD_TEST - created during testing

### Transport Status
- [ ] Check A4HK900114 for test objects
- [ ] Decide: keep or release transport

---

## Test Results Summary

| Test | Description | Expected | Actual | Pass? |
|------|-------------|----------|--------|-------|
| 1 | Edit local, no transport | SUCCESS | | |
| 2 | Edit with transport, no flag | BLOCK | ✅ BLOCKED | ✅ |
| 3 | Edit with transport, with flag | SUCCESS | ✅ SUCCESS | ✅ |
| 4 | Edit with transport, wrong whitelist | BLOCK | ✅ BLOCKED | ✅ |
| 5 | Edit with transport, correct whitelist | SUCCESS | ✅ SUCCESS | ✅ |
| 6 | WriteSource with transport, no flag | BLOCK | ✅ BLOCKED | ✅ |
| 7 | WriteSource with transport, with flag | SUCCESS | ✅ SUCCESS | ✅ |
| 8 | Verbose output check | Shows message | ⏳ Manual | |
| 9 | Environment variable | Works | ✅ Works | ✅ |
| 10 | Package restriction | BLOCK wrong pkg | ⏳ Manual | |
| 11 | ListTransports blocked by default | BLOCK | ⏳ Manual | |
| 12 | ListTransports with --allow-transportable-edits | SUCCESS | ⏳ Manual | |
| 13 | GetTransport with --allow-transportable-edits | SUCCESS | ⏳ Manual | |
| 14 | CreateTransport still requires --enable-transports | BLOCK | ⏳ Manual | |

---

## Test Session: 2026-02-03

### Tested via MCP (SAP_ALLOW_TRANSPORTABLE_EDITS=true, SAP_ALLOWED_TRANSPORTS=A4HK*)

**Test 3 & 5: EditSource with transport + whitelist**
```
EditSource on ZPROD_001_TEST with transport A4HK900114
Result: SUCCESS - edit applied, object activated
Added: "* Safety feature test - 2026-02-03"
```

**Test 7: WriteSource with transport**
```
WriteSource ZPROD_003_TEST with transport A4HK900114
Result: SUCCESS - program created and activated
```

**Test 9: Environment variable**
```
.mcp.json configured with SAP_ALLOW_TRANSPORTABLE_EDITS=true
Config was applied successfully - edits allowed
```

### CLI Tests (bypassing MCP, direct vsp invocation)

**Test 2: EditSource with transport, flag OFF → BLOCKED ✅**
```
"EditSource failed: operation 'EditSource' with transport 'A4HK900114' is blocked:
editing transportable objects is disabled..."
```

**Test 4: EditSource with wrong whitelist (DEVK*) → BLOCKED ✅**
```
"EditSource failed: operation 'EditSource' with transport 'A4HK900114' is blocked
by safety configuration (allowed transports: [DEVK*])"
```

**Test 6: WriteSource with transport, flag OFF → BLOCKED ✅**
```
"WriteSource failed: operation 'WriteSource' with transport 'A4HK900114' is blocked:
editing transportable objects is disabled..."
```

### Objects Created
- ZPROD_003_TEST - New program created via WriteSource

---

## Notes

- Tool visibility implemented: ListTransports/GetTransport work with --enable-transports OR --allow-transportable-edits
- .vsp.json updated to enable GetTransport, GetUserTransports, ListTransports
- MCP restart required for .vsp.json changes to take effect
