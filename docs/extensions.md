# Extensions

An extension adds `SAP()` actions to vsp from a Go module of its own, for
object types and workflows that do not belong in vsp's core. It is compiled
into a binary of its own; the release binary carries none and is unchanged.

```go
package main

import (
	"github.com/oisee/vibing-steampunk/pkg/vsp"

	"example.com/vsp-forms/forms"
)

func main() { vsp.Run(forms.New()) }
```

That binary has every built-in command and action, plus the extensions'.
Several extensions combine: `vsp.Run(a.New(), b.New())`.

## Writing one

An extension implements `mcpext.Extension` (package `pkg/mcpext`): a name, a
help text, and its actions. Each `mcpext.Action` declares

- `Action` and `Type` -- the call it serves, `SAP(action="read", target="SEGM <name>")`;
  `Type` is empty for an action without a target;
- `Class` -- `Read`, `Mutate` or `Execute`, what it does to the system;
- `Op` -- the operation the safety configuration is checked against: a read
  operation for `Read`, one that changes the system (`OpCreate`, `OpUpdate`,
  `OpDelete`, `OpActivate`, `OpWorkflow`, or `OpTransport`, which also needs
  `--enable-transports`) for `Mutate` and `Execute`;
- `Handler` -- the code, called with the object name and the call's params.

See `pkg/mcpext/example_test.go` for a complete extension.

### What a handler gets: `mcpext.Env`

| Method | What it is |
|---|---|
| `ADT()` | the server's own ADT client, with its safety configuration and mutation gate |
| `RFC(ctx)` | a client on the server's own RFC gateway, as `SAP(action="rfc")` uses it |
| `RFCDedicated(ctx, timeout)` | a connection of its own on that gateway, for a call that outlasts the shared one |
| `DropRFC(ctx)` | forget the shared RFC client after its connection died |
| `ZADTVSP(ctx)` | the server's WebSocket to ZADT_VSP, for an extension with an ABAP service of its own |
| `System()` | the connected system: `.vsp.json` name, URL, client, user, language, read-only |
| `Setting(key)` | the extension's own setting for this system (below) |
| `StartAsync(kind, fn)` | a background task, reported by `GET_ASYNC_RESULT` (`wait_seconds` waits up to 30 minutes) |
| `Logf(...)` | a diagnostic line under `--verbose` |

A workflow that locks an object and writes under the lock runs the mutation
gate first with `ADT().PrepareMutation(ctx, adt.MutationContext{...})` and
uses the returned context for the whole lock window; `ADT().CheckMutation`
is the same gate without that. Both are the gate every built-in write runs.

### Settings

Each system in `.vsp.json` can carry settings for each extension:

```json
{"systems": {"dev": {"url": "...", "extensions": {"forms": {"allow_create": true}}}}}
```

`vsp` reads none of them; `Env.Setting("allow_create")` returns the forms
extension's own value for the connected system.

### Optional interfaces

An extension implements those it needs:

- `Versioned` -- its version, shown by `SAP(action="info")` and `vsp --version`;
- `Requirer` -- the `mcpext.APIVersion` it needs at least; an older binary
  refuses it at startup instead of failing on first use;
- `Starter`, `Closer` -- told when an MCP server starts and stops;
- `CommandProvider` -- command-line commands, at the top level or under a
  built-in command (`Parent: "transport"` gives `vsp transport <cmd>`). Their
  `RunE` gets an `Env` built from the same system, credentials and flags as
  every built-in command. A command whose name is taken, or whose parent does
  not exist, stops `vsp.Run`.

### Helpers and tests

`mcpext.String`, `Bool`, `Int` and `Strings` read params the way the built-in
actions do; `mcpext.JSON`, `Text` and `Errorf` build results.

`pkg/mcpext/mcpexttest` has a fake `Env` for a handler's own tests, and
`mcpexttest.Check(t, ext)`, which puts an extension through the core's rules
without a SAP system: accepted by `vsp.Run`, every `Mutate` and `Execute`
action refused under `--read-only` before its handler runs, listed in the
help.

The package changes only by addition; `mcpext.APIVersion` grows with each.

## What the core guarantees

- **Built-ins come first.** Extension actions are tried after every built-in
  router. An extension that declares a type or action a built-in router
  matches, or the same name or action as another extension, stops `vsp.Run`
  with an error naming both parties. Nothing depends on argument order.
- **The core runs the checks.** Before a handler runs, `--read-only` refuses a
  `Mutate` or `Execute` action, and the declared `Op` goes through
  `--allowed-ops`/`--disallowed-ops` like a built-in operation. A write the
  handler makes through `Env.ADT()` passes the client's mutation gate and
  `--allowed-packages` as well.
- **The read-only invariant covers extensions.** `TestReadOnlyInvariant`
  registers a fake extension and walks its actions by their declared class,
  as it walks the built-in ones.
- **Help.** `SAP(action="help")` lists the extensions;
  `SAP(action="help", target="<name>")` shows an extension's help and actions.

## Not supported

Extensions are compiled in, not loaded at run time: Go plugins do not work on
Windows. CLI subcommands for extensions may follow in the same style.
