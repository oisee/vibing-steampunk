# abapGit Dependencies & Submodules Analysis

**Date:** 2026-02-03
**Report ID:** 001
**Subject:** Git submodules support in abapGit and dependency management patterns
**Tags:** `roadmap`, `dependency-management`, `abapgit-integration`

---

## Executive Summary

Git submodules are **not supported** by abapGit. The community deliberately chose alternative approaches for ABAP dependency management. This report analyzes the current state and identifies opportunities for vsp to provide dependency automation.

## Background

Git submodules allow embedding one repository inside another, commonly used for:
- Shared libraries
- Third-party dependencies
- Monorepo component separation

Example from other ecosystems:
```bash
git submodule add https://github.com/lib/dependency vendor/dependency
git submodule update --init
```

## abapGit's Decision Against Submodules

### GitHub Issue #27 (2014)

The abapGit team considered and rejected native submodule support:

> "In terms of package management submodules will not be very flexible. Links via .abapgit are at least not worse and probably better (e.g. to keep semantic version and not commit)."

**Reasons cited:**
1. Submodules lock to specific commits, not semantic versions
2. ABAP has no native package resolution mechanism
3. SAP package hierarchy doesn't map cleanly to git submodules
4. Installation order matters (dependencies must exist before dependents)

### GitHub Issue #2236 - Dependency References

Later discussion (2018+) explored `.abapgit.xml` dependency declarations:

```xml
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0">
  <asx:values>
    <DATA>
      <DEPENDENCIES>
        <item>
          <url>https://github.com/sbcgua/abap_data_parser</url>
          <version>^1.0.0</version>
        </item>
      </DEPENDENCIES>
    </DATA>
  </asx:values>
</asx:abap>
```

### ABAP Package Manager (apm) Concept

The community discussed an npm-like package manager:
- Separate from abapGit core
- Handles dependency resolution
- Manages installation order
- Could be "abapmerge"-able for standalone distribution

Status: Conceptual, not fully implemented in abapGit core.

## Current Dependency Patterns in ABAP Ecosystem

| Pattern | Tool | Description |
|---------|------|-------------|
| Manual | abapGit UI | User installs dependencies one-by-one |
| `.abapgit.xml` links | abapGit | Declares dependencies, no auto-install |
| abaplint deps | abaplint | Static analysis of dependencies |
| apack | apack | Package manifest format (community) |

### apack Manifest Format

Some projects use `apack-manifest.xml`:
```xml
<?xml version="1.0" encoding="utf-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0">
  <asx:values>
    <DATA>
      <GROUP_ID>github.com/sbcgua</GROUP_ID>
      <ARTIFACT_ID>mockup_loader</ARTIFACT_ID>
      <VERSION>2.1.0</VERSION>
      <DEPENDENCIES>
        <item>
          <GROUP_ID>github.com/sbcgua</GROUP_ID>
          <ARTIFACT_ID>abap_data_parser</ARTIFACT_ID>
          <VERSION>^1.0.0</VERSION>
          <GIT_URL>https://github.com/sbcgua/abap_data_parser.git</GIT_URL>
        </item>
      </DEPENDENCIES>
    </DATA>
  </asx:values>
</asx:abap>
```

## Opportunity for vsp

### Current vsp Capabilities

vsp already has building blocks for dependency management:

| Capability | Status | File |
|------------|--------|------|
| Git clone/fetch | Via Bash | - |
| Parse abapGit files | Partial | `pkg/adt/fileparser.go` |
| Batch import | ✅ | `pkg/dsl/batch.go` |
| Dependency ordering | ✅ | `RAPOrder()` in DSL |
| Package operations | ✅ | `CreatePackage`, `GetPackage` |

### Proposed: Dependency Resolution Tool

**Tool:** `ResolveDependencies`

**Input:**
```json
{
  "repo_url": "https://github.com/example/abap-project",
  "target_package": "$ZPROJECT",
  "recursive": true
}
```

**Workflow:**
1. Clone/fetch repository
2. Parse `.abapgit.xml` or `apack-manifest.xml` for dependencies
3. Recursively resolve dependency tree
4. Topological sort (dependencies before dependents)
5. Import in correct order using existing `Import().RAPOrder()`

**Output:**
```json
{
  "resolved": [
    {"name": "abap_data_parser", "version": "1.2.0", "package": "$ZPARSER"},
    {"name": "mockup_loader", "version": "2.1.0", "package": "$ZMOCKUP"}
  ],
  "import_order": ["abap_data_parser", "mockup_loader"],
  "conflicts": []
}
```

### Proposed: Dependency Graph Visualization

Leverage existing `GetCallGraph` patterns:

```
mockup_loader (2.1.0)
├── abap_data_parser (^1.0.0) ✅ resolved: 1.2.0
└── ajson (^1.0.0) ✅ resolved: 1.1.5
    └── (no dependencies)
```

## Implementation Phases

### Phase 1: Manifest Parsing (Low effort)
- [ ] Parse `.abapgit.xml` dependency section
- [ ] Parse `apack-manifest.xml` format
- [ ] Extract dependency URLs and version constraints

### Phase 2: Resolution Engine (Medium effort)
- [ ] Implement semver constraint matching
- [ ] Build dependency graph
- [ ] Detect circular dependencies
- [ ] Topological sort for install order

### Phase 3: Automated Installation (Medium effort)
- [ ] Clone repositories to temp directory
- [ ] Create target packages if needed
- [ ] Import in dependency order
- [ ] Report success/failure per dependency

### Phase 4: Lock File (Optional)
- [ ] Generate `abap-lock.json` with resolved versions
- [ ] Support reproducible builds
- [ ] Diff lock files for upgrade detection

## Risk Assessment

| Risk | Mitigation |
|------|------------|
| Version conflicts | Fail fast with clear error message |
| Circular dependencies | Detect and report before import |
| Missing dependencies | Check object existence before import |
| Transport conflicts | Use `$TMP` for initial testing |

## References

- [abapGit Docs](https://docs.abapgit.org/)
- [abapGit Issue #27 - Submodules](https://github.com/abapGit/abapGit/issues/27)
- [abapGit Issue #2236 - Dependencies](https://github.com/abapGit/abapGit/issues/2236)
- [abapGit Issue #952 - Super packages](https://github.com/abapGit/abapGit/issues/952)
- [apack on GitHub](https://github.com/sbcgua/abap_package_manager)

## Conclusion

While abapGit chose not to implement git submodules, there's a clear need for dependency automation in the ABAP ecosystem. vsp is well-positioned to fill this gap by:

1. Parsing existing manifest formats (`.abapgit.xml`, `apack-manifest.xml`)
2. Leveraging existing batch import infrastructure
3. Providing dependency resolution as an MCP tool

This would complement abapGit rather than compete with it, providing the "apm" functionality the community has discussed but not fully implemented.

---

**TODO Tags:**
- `[ROADMAP]` Dependency resolution tool
- `[ROADMAP]` apack manifest support
- `[ROADMAP]` Dependency graph visualization
