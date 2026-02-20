---
name: sync
description: "Synchronize claude-team-config rules, agents, and skills to target projects"
---

# Sync Command

Synchronize CLAUDE.md rules, agents, and skills from claude-team-config to target projects.

## Usage

```
/sync                                    # Sync all projects (default)
/sync all                                # Same as above — sync all projects
/sync pdap-hub                           # Sync one project
/sync pdap-hub pdap-rag-mcp              # Sync selected projects
/sync pdap-hub vibing-steampunk --dry    # Preview changes without applying
/sync status                             # Check sync status without syncing
/sync init                               # Interactive setup for new users
```

## Parameters

- **Project names** (positional) — one or more project names from `projects.json`. If omitted or `all`, syncs all projects.
- **`--dry`** — dry-run mode, show what would change without modifying files.
- **`status`** — check which files are out of sync without making changes.
- **`init`** — interactive first-run setup: discover projects, create `projects.local.json`, run first sync.

Available projects are defined in `projects.json` (see `projects.json` in the config repo root).

## Finding the Config Repo

Resolve the config repo path in this order:
1. If the current working directory IS claude-team-config (has `sync.ps1`) → use it
2. If `.claude/.sync-manifest.json` exists in the current project → read `config_repo` from it
3. Fallback → `~/claude-team-config`

## Process

### Default sync (`/sync` or `/sync <projects>`)

1. **Parse arguments** — extract project names and flags.
2. **Resolve config repo path** (see above).
3. **Build the PowerShell command**:
   - Base: `powershell -ExecutionPolicy Bypass -File "<config-repo>/sync.ps1"`
   - If specific projects: add `-Project <name1>,<name2>,...`
   - If `--dry` flag: add `-DryRun`
4. **Execute sync.ps1** using the Bash tool.
5. **Report results** — summarize changes per project.
6. **Remind about commits** — if changes were applied, remind the user to commit in each affected project.

### Status check (`/sync status`)

1. **Resolve config repo path**.
2. Run: `python "<config-repo>/scripts/sync-check.py"` with `{"cwd": "<current-dir>"}` piped to stdin.
3. Report which files are out of sync.

### Init (`/sync init`)

1. **Check** if `projects.json` has any reachable projects.
2. If not, **scan** common locations (`~/`, `~/dev/`, `~/projects/`, `~/repos/`) for directories containing `.git`.
3. **Present** found repos to the user, suggest adding them.
4. **Help create** `projects.local.json` if paths differ from `~/` convention.
5. **Run first sync** after configuration.

## Rules

- Always use `powershell -ExecutionPolicy Bypass -File` to invoke sync.ps1
- Show the full sync output to the user
- If sync fails, show the error and suggest fixes
- Do NOT automatically commit changes — only remind the user
- For `/sync init`, ask the user which discovered projects to add — don't add all automatically
