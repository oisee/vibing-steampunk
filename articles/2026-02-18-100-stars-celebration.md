# Agentic ABAP at 100 Stars: The Numbers, The Community, and What's Cooking

**Or: Two Months, 44 Releases, 5 Contributors, and One Very Patient Go Binary**

---

TL;DR:
- **100+ stars** on GitHub (103 and counting)
- **197 commits**, **44 releases** (v2.12 -> v2.26), **99 tools**
- **5 contributors**, **10 PRs** (3 currently open with new features)
- **26 forks** - people are building on top of it
- What changed since the original article, what's brewing in branches, and what's next

---

Two months ago, I published "Agentic ABAP: Why I Built a Bridge for Claude Code" and shared vs-punk with the SAP community. The response was... unexpected.

People didn't just star the repo. They started using it. Then they started fixing it. Then they started extending it.

This is the honest update.

## By the Numbers

| Metric | Dec 17, 2025 | Feb 18, 2026 | Delta |
|--------|:------------:|:------------:|:-----:|
| **GitHub Stars** | ~15 | **103** | +88 |
| **Forks** | 2 | **26** | +24 |
| **Commits** | ~50 | **197** | +147 |
| **Releases** | v2.12.x | **v2.26.0** (44 total) | +32 releases |
| **MCP Tools** | ~45 | **99** (54 focused / 99 expert) | +54 |
| **Unit Tests** | ~150 | **270+** | +120 |
| **Contributors** | 1 | **5** | +4 |
| **Pull Requests** | 0 | **10** (7 merged, 3 open) | +10 |

That's roughly one release every two days. Some were big features. Some were one-line fixes from the community that made the difference between "works on my machine" and "works on yours too."

## What Changed: The Highlight Reel

### The Releases That Mattered Most

**v2.13.0 - Call Graph & RCA** (Dec 21)
The answer to Fred's question from the comments. Claude can now trace call hierarchies, compare static vs. actual execution paths, and find untested code paths. Root Cause Analysis stopped being a slide deck and became a tool.

**v2.16.0 - abapGit Integration** (Dec 23)
158 object types. Export entire packages as abapGit-compatible ZIPs. Via WebSocket for reliability. This was the "okay, now it's actually useful for migrations" moment.

**v2.17.0 - One-Command Install** (Dec 24)
Christmas present to myself. `InstallZADTVSP` deploys the WebSocket handler to SAP in one command. `InstallAbapGit` does the same for abapGit. No more manual ABAP copy-paste.

**v2.20.0 - CLI Mode** (Jan 6)
vsp stopped being just an MCP server. `vsp search`, `vsp source`, `vsp export` - direct terminal operations. Multi-system profiles in `.vsp.json`. Manage dev, QA, and prod from one config.

**v2.21.0 - Method-Level Operations** (Jan 6)
The feature nobody asked for that everyone needed. `GetSource(class="ZCL_BIG", method="TINY_METHOD")` returns 50 lines instead of 2000+. **95% token reduction**. This single change made AI-assisted development of large classes actually practical.

**v2.24.0 - Transportable Edits Safety** (Feb 3)
The "we're getting serious about production" release. Editing objects in transportable packages is now blocked by default. You must explicitly enable it with `--allow-transportable-edits`. Package whitelisting, transport whitelisting, operation filtering. Enterprise governance for AI agents.

### The Feature That Changed Everything: EditSource

In the original article, I called EditSource the "star of the show." Two months later, I stand by that.

But it got better. Method-level constraint means Claude can now say:

```
EditSource(
  object="ZCL_ORDER_PROCESSING",
  method="VALIDATE_DATES",
  old="IF lv_end < lv_start.",
  new="IF lv_end < lv_start OR lv_end IS INITIAL."
)
```

No risk of accidentally editing a different method. No downloading 3000 lines of source to change one condition.

## The Community

This is the part I'm most proud of.

### Contributors

| Who | What They Built |
|-----|-----------------|
| **@vitalratel** (4 PRs) | CLI mode foundation, RunReport background jobs, MoveObject tool, WebSocket refactoring |
| **@kts982** (1 PR) | Transport API fix + EditSource transport support |
| **@ingenium-it-engineering** (3 PRs) | Package validation fix, SetMessages tool, CreateStructure/CreateTableType (open) |
| **@marianfoo** (1 PR) | DebuggerGetVariables schema fix (open) |

What strikes me is the diversity of contributions. One person fixed a production transport issue. Another is adding entirely new object creation capabilities. A third fixed a schema bug that affected debugging.

These aren't cosmetic PRs. They're battle-tested fixes from people running vsp against real SAP systems.

### The Conversations

Some of my favorite moments from the LinkedIn thread:

**Mike Pokraka** (SAP Press author) asked about syntax check ordering - and he was right. We were already doing it optimally, but the question led to better documentation.

**Attila Berencsi** nailed the core insight: *"ABAP is not to be handled as a plain JS project. The context is not only a folder with text files."* This is exactly why vsp exists - because the SAP object model needs a specialized bridge, not a generic file watcher.

**Martin Ceronio**: *"With community contributions like these, I don't see what value Joule is eventually going to add..."* - No comment. (Okay, one comment: Joule and vsp solve different problems. But the sentiment matters.)

## What's Brewing (Branches & Experiments)

### `one-tool-mode` - The Token Economy Experiment

This is a fascinating branch born from @vitalratel's PR #16. The idea: what if instead of 99 separate MCP tools, we expose **one universal SAP tool** with internal routing?

Why? Because every tool you register in MCP costs tokens in the system prompt. 99 tools = ~8,000 tokens of tool descriptions that Claude reads on every single message. One tool with a `action` parameter? ~200 tokens.

It's a radical approach. The trade-off is discoverability (Claude needs to "know" the actions exist without seeing them listed). We're evaluating both approaches.

### `feature/debug-daemon-parked` - The Debugging Dream

I'll be honest: interactive debugging via HTTP is hard. SAP's debugger is deeply session-stateful. We have the tools (breakpoints, listen, attach, step, inspect), and they work - but the reliability isn't where I want it to be for "just works" out of the box.

Current recommended workflow: Use vsp for **analysis** (dumps, traces, call graphs), and for **breakpoint sharing** with SAP GUI via `--terminal-id`. Set a breakpoint in vsp, hit it in SAP GUI. Best of both worlds.

## The Honest Assessment

### What Works Brilliantly
- **CRUD operations** - Rock solid. EditSource is the crown jewel.
- **Analysis & RCA** - Dumps, traces, call graphs, ATC checks. Claude as a debugger analyst.
- **abapGit integration** - 158 object types, one-command deployment.
- **Safety system** - Read-only mode, package restrictions, transport governance.
- **CLI mode** - Multi-system management without MCP overhead.

### What's Harder Than Expected
- **Interactive debugging** - Session statefulness makes HTTP unreliable. WebSocket is better but not perfect.
- **UI5/BSP writes** - ADT filestore is read-only. Need alternate API.
- **AMDP/HANA debugging** - Experimental. Session works, breakpoints need more investigation.

### What I Didn't See Coming
- **Method-level operations** - Emerged from actual usage. Now I can't imagine working without it.
- **Community transport governance** - People want AI in production-adjacent systems. Safety features became the most requested category.
- **The "one tool" question** - Token economics in MCP are a real design constraint I hadn't considered initially.

## What's Next

The roadmap is shifting from "add more tools" to "make existing tools smarter":

1. **Better debugging** - WebSocket-based session persistence
2. **Test intelligence** - "You changed method X, here are the tests that cover it"
3. **abapGit import** - The reverse of export (deserialize objects from ZIP)
4. **One-tool mode** - If the experiment succeeds on the branch
5. **More community PRs** - 3 open right now, all adding real capabilities

## The Conclusion

100 stars is a nice number. But the number that matters more is **5** - the number of people who cared enough to open their IDE, write Go code, and submit a PR to make this tool better for everyone.

vsp started as a personal experiment: "Can I make Claude Code work with SAP?" Two months later, it's becoming a community toolkit.

The bridge works. People are crossing it. And they're building things on the other side that I never imagined.

Download the binary. Greenlist your sandbox. Join the conversation.

---

**GitHub**: oisee/vibing-steampunk
**Stars**: 103 (thank you!)
**Latest**: v2.26.0

*Previous article: "Agentic ABAP: Why I Built a Bridge for Claude Code" (Dec 17, 2025)*

#ABAP #ClaudeCode #MCP #SAP #OpenSource #AI #GoLang #ECC #S4HANA #Community #100Stars
