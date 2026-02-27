---
name: sync
description: "Synchronize claude-team-control rules, agents, and skills to target projects"
---

# Sync Command

Synchronize CLAUDE.md rules, agents, and skills from claude-team-control to target projects.

## Usage

```
/sync                                    # Sync all projects (push, default)
/sync all                                # Same as above — sync all projects
/sync pdap-hub                           # Sync one project
/sync pdap-hub pdap-rag-mcp              # Sync selected projects
/sync pdap-hub vibing-steampunk --dry    # Preview changes without applying
/sync status                             # Check sync status without syncing
/sync init                               # Interactive setup for new users
/sync pull                               # Pull new agents/skills from all projects
/sync pull pdap-hub                      # Pull from specific project only
/sync pull pdap-hub,vibing-steampunk     # Pull from multiple projects
/sync pull --dry                         # Preview what would be promoted
```

## Parameters

- **Project names** (positional) — one or more project names from `projects.json`. If omitted or `all`, syncs all projects.
- **`--dry`** — dry-run mode, show what would change without modifying files.
- **`pull`** — pull mode: scan project `.claude/` directories for locally-created agents/skills not in the sync manifest, and promote them to the config repo's `agents/` or `skills/` directories.
- **`status`** — check which files are out of sync without making changes.
- **`init`** — interactive first-run setup: discover projects, create `projects.local.json`, run first sync.

Available projects are defined in `projects.json` (see `projects.json` in the config repo root).

## Finding the Config Repo

Resolve the config repo path in this order:
1. If the current working directory IS claude-team-control (has `sync.ps1`) → use it
2. If `.claude/.sync-manifest.json` exists in the current project → read `config_repo` from it
3. Fallback → `~/claude-team-control`

## Bidirectional Sync Workflow

To promote a locally-created agent from a project to all other projects:

```
/sync pull pdap-hub    # 1. Scan pdap-hub, show candidates, ask user, then promote
/sync                  # 2. Distribute promoted agent to all projects
```

Pull is **interactive** — it always scans first, presents the candidates to the user, and asks for confirmation before promoting anything.

Pull only promotes files **not tracked in the sync manifest** (i.e., created locally in the project, not synced from the config repo). It never overwrites existing files in `agents/` or `skills/`.

**CLAUDE.md rules** are NOT pulled automatically — edit `base/CLAUDE.md` or `overlays/<project>.md` directly.

After pull, if skills were promoted: manually add them to `include_skills` in `projects.json`, then run `/sync`.

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

### Pull mode (`/sync pull` or `/sync pull <projects>`)

Pull is always a two-step interactive flow — **always scan first, ask the user, then promote**. Never promote silently.

**Step 1 — Scan (mandatory)**
1. Parse arguments. If `pull` is the first positional arg, activate pull mode. Extract project names and `--dry` flag.
2. Resolve config repo path (see below).
3. Run scan with `-DryRun`:
   ```
   powershell -ExecutionPolicy Bypass -File "<config-repo>/sync.ps1" -Pull -DryRun [-Project <names>]
   ```
4. Parse the output to extract "Would promote agents" and "Would promote skills" lists.

**Step 2 — Ask the user**
5. If nothing found: report "No new locally-created agents or skills found." and stop.
6. If candidates found: present them to the user in a clear list:
   - Show each candidate agent/skill with its source project
   - Ask: *"Which of these should be promoted to the config repo and distributed to all projects? (all / none / list names)"*
7. Wait for the user's answer. **Do not proceed without explicit confirmation.**

**Step 3 — Promote**
8. Based on user response:
   - **all**: run `sync.ps1 -Pull [-Project <names>]` (no filter — promote everything)
   - **none**: abort, report cancelled
   - **specific names**: run `sync.ps1 -Pull -AgentFilter <selected> [-SkillFilter <selected>] [-Project <names>]`
9. Report promoted files, skipped conflicts, invalid frontmatter warnings.
10. If skills were promoted: remind the user to add them to `include_skills` in `projects.json`.
11. **Suggest next step**: run `/sync` to distribute promoted files to all projects.

> **Note on CLAUDE.md rules**: Pull does NOT automatically promote changes to `.claude/CLAUDE.md` in projects. Rule changes require manual editing of `base/CLAUDE.md` or `overlays/<project>.md` in the config repo, then running `/sync`.

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
