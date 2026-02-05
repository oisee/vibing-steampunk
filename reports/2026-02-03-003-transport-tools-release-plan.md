# Transport Tools & Safety Feature Release Plan

**Date:** 2026-02-03
**Report ID:** 003
**Subject:** Release plan for transportable edits safety feature
**Related Documents:**
- `2026-02-03-SESSION-CONTEXT.md`
- `2026-02-03-002-transportable-edits-safety-feature.md`

---

## Summary

This release adds a safety feature that blocks editing objects in transportable packages by default. Users must explicitly opt-in to edit non-local objects.

## Tool Visibility Logic

### Transport-Related Tools

| Tool | `--allow-transportable-edits` | `--enable-transports` | Notes |
|------|------------------------------|----------------------|-------|
| `ListTransports` | ✅ Visible | ✅ Visible | Read-only, safe |
| `GetTransport` | ✅ Visible | ✅ Visible | Read-only, safe |
| `CreateTransport` | ❌ Hidden | ✅ Visible | Write operation |
| `ReleaseTransport` | ❌ Hidden | ✅ Visible | Dangerous |
| `DeleteTransport` | ❌ Hidden | ✅ Visible | Dangerous |

**Rationale:**
- `--allow-transportable-edits`: User needs to see transports to understand which TR their edits go into
- `--enable-transports`: Full transport management (create/release/delete)

### Edit Operations Behavior

| Scenario | Flag OFF | Flag ON |
|----------|----------|---------|
| Edit object in `$TMP` | ✅ Works | ✅ Works |
| Edit object in `ZPROD` (transportable) | ❌ Blocked with message | ✅ Works |
| Edit object in whitelisted transport | ❌ Blocked (no flag) | ✅ Works |

### Error Message (when blocked)

```
operation 'EditSource' with transport 'A4HK900114' is blocked: editing transportable objects is disabled.
Objects in transportable packages require explicit opt-in.
Use --allow-transportable-edits or SAP_ALLOW_TRANSPORTABLE_EDITS=true to enable.
WARNING: This allows modifications to non-local objects that may affect production systems.
```

## Configuration Options

### CLI Flags
```bash
# Enable editing transportable objects
vsp --allow-transportable-edits

# Whitelist specific transports (wildcards supported)
vsp --allow-transportable-edits --allowed-transports "A4HK*,DEVK900001"

# Whitelist specific packages
vsp --allowed-packages "Z*,$TMP"
```

### Environment Variables
```bash
SAP_ALLOW_TRANSPORTABLE_EDITS=true
SAP_ALLOWED_TRANSPORTS=A4HK*,DEVK*
SAP_ALLOWED_PACKAGES=Z*,$TMP,$*
```

## Release Checklist

### Code Changes (DONE)
- [x] `pkg/adt/safety.go` - `AllowTransportableEdits` field, `CheckTransportableEdit()`
- [x] `pkg/adt/safety_test.go` - 7 test cases
- [x] `pkg/adt/config.go` - `WithAllowTransportableEdits()` option
- [x] `pkg/adt/client.go` - `checkTransportableEdit()` helper
- [x] `pkg/adt/workflows.go` - Checks in `EditSourceWithOptions()`, `WriteSource()`
- [x] `cmd/vsp/main.go` - CLI flags, env vars
- [x] `internal/mcp/server.go` - Config, tool descriptions

### Testing (DONE)
- [x] Unit tests pass (7/7 safety tests)
- [x] EditSource blocked when flag OFF
- [x] EditSource works when flag ON
- [x] WriteSource blocked when flag OFF
- [x] WriteSource works when flag ON
- [x] Transport whitelist works
- [x] ListTransports visible with flag
- [x] GetTransport returns full details

### Documentation (TODO)
- [ ] Update README.md with new flags
- [ ] Update CLAUDE.md if needed
- [ ] Update GitHub issues #17, #18

### Release (TODO)
- [ ] Commit changes
- [ ] Tag release (v2.24.0)
- [ ] Build binaries
- [ ] Update release notes

---

## GitHub Issues to Update

### Issue #17: Transport Management Safety
- Status: RESOLVED
- Solution: `--allow-transportable-edits` flag with whitelist support

### Issue #18: Package Restrictions
- Status: RESOLVED
- Solution: `--allowed-packages` with wildcard support

---

## Version: v2.24.0

### Highlights
1. **Transportable Edits Safety** - Blocks editing non-local objects by default
2. **Transport Whitelisting** - Restrict edits to specific transports
3. **Package Whitelisting** - Restrict edits to specific packages
4. **Improved Error Messages** - Clear guidance on how to enable features

### Breaking Changes
None - new safety features are opt-in blockers, existing workflows unaffected if using `$TMP`.
