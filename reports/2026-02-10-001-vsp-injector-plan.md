# VSP Injector: Bootstrap Deployment Plan

**Date:** 2026-02-10
**Report ID:** 001
**Subject:** End-to-end SAP system bootstrap via ADT
**Status:** Planning

---

## 1. The Vision

> "Приоткрыли тебе дверь через ADT - а ты туда ногу просунул и уже внутрь весь просочился и как дома - хоп!"

**Translation:** "They cracked the door open via ADT - you put your foot in, and then you're inside like at home - hop!"

### Goal
Fresh/wiped SAP system → Connect via ADT → Bootstrap full toolkit → Full AI development power

### Target System
- **a4h-110-adt**: `http://192.168.8.110:50000` (freshly restored)

---

## 2. Current Capabilities

### Embedded ABAP Objects (Ready to Deploy)
```
embedded/abap/
├── zif_vsp_service.intf.abap       # Interface (deploy FIRST)
├── zcl_vsp_apc_handler.clas.abap   # WebSocket APC handler
├── zcl_vsp_rfc_service.clas.abap   # RFC domain (CallRFC)
├── zcl_vsp_debug_service.clas.abap # Debug domain
├── zcl_vsp_report_service.clas.abap# Report domain (RunReport)
├── zcl_vsp_git_service.clas.abap   # Git domain (GitExport)
├── zcl_vsp_amdp_service.clas.abap  # AMDP debugging
├── zcl_vsp_utils.clas.abap         # Utilities
├── zadt_cl_tadir_move.clas.abap    # MoveObject helper
└── test programs...
```

### Existing Install Tools
| Tool | Status | What it Does |
|------|--------|--------------|
| `InstallZADTVSP` | ✅ Works | Deploys 6 core objects to $ZADT_VSP |
| `InstallAbapGit` | ⚠️ Placeholder | ZIP not yet embedded |
| `InstallDummyTest` | ✅ Works | Verifies install workflow |
| `ListDependencies` | ✅ Works | Shows available dependencies |

### Embedded Dependencies (TODO)
```
embedded/deps/
├── embed.go                 # Placeholder structure
├── abapgit-standalone.zip   # TODO: Embed
└── abapgit-dev.zip          # TODO: Embed (optional)
```

---

## 3. Deployment Phases

### Phase 0: Connection Test
```bash
# Verify ADT connectivity to fresh system
vsp --url http://192.168.8.110:50000 --user AVINOGRADOVA test-connection
```

**Checks:**
- [ ] HTTP connectivity
- [ ] Authentication
- [ ] ADT discovery endpoint
- [ ] Basic read operation (SearchObject)

### Phase 1: ADT-Only Bootstrap (No Dependencies)

**What ADT can do natively:**
- Create/update PROG, CLAS, INTF
- Create local packages ($*)
- Lock/Unlock/Activate objects
- Syntax check before save

**Deployment Order (Dependency-Aware):**
1. Package `$ZADT_VSP` (container)
2. Interface `ZIF_VSP_SERVICE` (no dependencies)
3. Class `ZCL_VSP_UTILS` (implements interface)
4. Class `ZCL_VSP_APC_HANDLER` (uses utils)
5. Class `ZCL_VSP_RFC_SERVICE` (uses handler)
6. Class `ZCL_VSP_DEBUG_SERVICE` (uses handler)
7. Class `ZCL_VSP_REPORT_SERVICE` (uses handler)
8. Class `ZCL_VSP_GIT_SERVICE` (uses handler, optional)

**Command:**
```bash
vsp deploy --vsp --target http://192.168.8.110:50000
```

### Phase 2: SAPC/SICF Configuration

**Manual Steps Required:**
1. **SAPC** (APC Application Configuration)
   - Create APC application `ZADT_VSP`
   - Assign handler class `ZCL_VSP_APC_HANDLER`
   - Configure WebSocket path

2. **SICF** (HTTP Service Activation)
   - Activate `/sap/bc/apc/sap/zadt_vsp`
   - Or create custom SICF node

**Can We Automate?**
- ❓ SAPC creation via ADT - needs investigation
- ❓ SICF activation via ADT - needs investigation
- ⚠️ May require RFC calls (chicken-and-egg problem)

**Workaround:**
- Provide step-by-step instructions
- Or: Include a "helper report" that creates SAPC/SICF entries

### Phase 3: abapGit Deployment

**Option A: Standalone (Single Report)**
```bash
vsp deploy --git
# Creates: ZABAPGIT program in $ABAPGIT package
```

**Source Options:**
1. Embedded snapshot (faster, offline)
2. Latest from GitHub (always current)

**Option B: Developer Edition (Full Packages)**
```bash
vsp deploy --git-dev
# Creates: $ZGIT_DEV, $ZGIT_DEV_UI, $ZGIT_DEV_*
```

### Phase 4: Full Power Mode

**Once ZADT_VSP + abapGit installed:**
```bash
# Deploy any abapGit-compatible ZIP
vsp deploy --source mypackage.zip --package $ZMYPACKAGE

# Or use GitExport first to create ZIP
vsp git-export --packages '$ZSOURCE' --output source.zip
vsp deploy --source source.zip --package $ZTARGET --target http://new-system:50000
```

---

## 4. CLI Interface Design

### Proposed Commands

```bash
# Check deployment prerequisites
vsp deploy check [--target URL]

# Deploy ZADT_VSP WebSocket handler
vsp deploy vsp [--target URL] [--package $ZADT_VSP]

# Deploy abapGit
vsp deploy git [--edition standalone|dev] [--target URL]

# Deploy custom ZIP
vsp deploy package --source FILE.zip --package $ZPACKAGE [--target URL]

# Full bootstrap (vsp + git + verify)
vsp deploy bootstrap [--target URL]

# Show deployment status
vsp deploy status [--target URL]
```

### Alternative: Flags on Main Binary
```bash
vsp --deploy-vsp --target http://192.168.8.110:50000
vsp --deploy-git --target http://192.168.8.110:50000
vsp --deploy-bootstrap --target http://192.168.8.110:50000
```

---

## 5. Implementation Plan

### Step 1: Test Current InstallZADTVSP on a4h-110
```bash
# Connect to fresh system
export SAP_URL=http://192.168.8.110:50000
export SAP_USER=AVINOGRADOVA
export SAP_PASSWORD=...

# Check prerequisites
vsp tool InstallZADTVSP --check_only=true

# Deploy if OK
vsp tool InstallZADTVSP
```

### Step 2: Document SAPC/SICF Setup
- Create step-by-step guide
- Screenshots or transaction sequences
- Consider helper report for automation

### Step 3: Embed abapGit Standalone
```bash
# On existing system with abapGit
vsp git-export --packages 'ZABAPGIT' --output embedded/deps/abapgit-standalone.zip

# Or download from GitHub releases
curl -L https://github.com/abapGit/abapGit/releases/latest/download/abapGit.zip \
  -o embedded/deps/abapgit-standalone.zip
```

### Step 4: Implement CLI Deploy Commands
- Add `deploy` subcommand to vsp
- Implement `deploy check`, `deploy vsp`, `deploy git`
- Add `--target` flag for multi-system deployment

### Step 5: End-to-End Test
```bash
# Full bootstrap on fresh system
vsp deploy bootstrap --target http://192.168.8.110:50000

# Verify
vsp --url http://192.168.8.110:50000 tool GetFeatures
```

---

## 6. Dependency Chain Visualization

```
┌─────────────────────────────────────────────────────────────┐
│                    FRESH SAP SYSTEM                         │
│                    (ADT enabled only)                       │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│  Phase 1: ADT Native Deployment                             │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ WriteSource (PROG, CLAS, INTF)                      │   │
│  │ CreatePackage ($ZADT_VSP)                           │   │
│  │ Activate objects                                     │   │
│  └─────────────────────────────────────────────────────┘   │
│                              │                              │
│                              ▼                              │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ $ZADT_VSP Package                                   │   │
│  │ ├── ZIF_VSP_SERVICE (interface)                     │   │
│  │ ├── ZCL_VSP_APC_HANDLER (WebSocket handler)         │   │
│  │ ├── ZCL_VSP_RFC_SERVICE (RFC domain)                │   │
│  │ ├── ZCL_VSP_DEBUG_SERVICE (debug domain)            │   │
│  │ ├── ZCL_VSP_REPORT_SERVICE (report domain)          │   │
│  │ └── ZCL_VSP_GIT_SERVICE (git domain)                │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│  Phase 2: SAPC/SICF Configuration (Manual or Helper)        │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ SAPC: Create APC application ZADT_VSP               │   │
│  │ SICF: Activate /sap/bc/apc/sap/zadt_vsp             │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│  UNLOCKED: WebSocket Tools                                  │
│  ├── CallRFC (call any function module)                     │
│  ├── RunReport (execute reports with spool)                 │
│  ├── GitExport (export 158 object types)                    │
│  ├── SetBreakpoint (WebSocket-based debugging)              │
│  └── GetBreakpoints, DebuggerListen, etc.                   │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│  Phase 3: abapGit Deployment (Optional)                     │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ ZABAPGIT program (standalone)                        │   │
│  │ OR                                                   │   │
│  │ $ZGIT_DEV packages (full developer edition)          │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│  FULL POWER MODE                                            │
│  ├── Deploy any abapGit ZIP                                 │
│  ├── 158 object types supported                             │
│  ├── Complex dependencies (DDIC, FUGR, etc.)                │
│  ├── CI/CD pipeline integration                             │
│  └── Cross-system migrations                                │
└─────────────────────────────────────────────────────────────┘
```

---

## 7. Risk Assessment

| Risk | Mitigation |
|------|------------|
| SAPC/SICF requires manual config | Provide clear instructions; investigate automation |
| Activation failures (dependency order) | Sort by type: INTF → CLAS; retry logic |
| Object name conflicts | Check existence before create; upsert strategy |
| Transport requirements | Use local packages ($*) for bootstrap |
| abapGit not installed | Fall back to ADT-only deployment |
| WebSocket blocked by firewall | Document port requirements |

---

## 8. Success Criteria

- [ ] Fresh a4h-110 system accessible via ADT
- [ ] InstallZADTVSP deploys all 6 objects successfully
- [ ] SAPC/SICF configured (manual or automated)
- [ ] WebSocket connection works (CallRFC test)
- [ ] abapGit standalone deployed
- [ ] GitExport works via WebSocket
- [ ] Full bootstrap < 5 minutes

---

## 9. Next Actions

1. **Test InstallZADTVSP on a4h-110** - Verify current capability
2. **Document SAPC/SICF setup** - Step-by-step guide
3. **Embed abapGit standalone** - Add ZIP to embedded/deps/
4. **Implement `vsp deploy` subcommand** - CLI interface
5. **Create bootstrap verification script** - Automated testing

---

## 10. Related Documents

- `embedded/abap/README.md` - ZADT_VSP installation guide
- `reports/2025-12-05-003-adt-universal-deployment.md` - Factory Pattern strategy
- `reports/2025-12-08-001-abapgit-integration-design.md` - abapGit integration

