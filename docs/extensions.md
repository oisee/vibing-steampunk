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

The handler gets an `mcpext.Env`: `ADT()` is the server's own ADT client,
`RFC(ctx)` a client on the server's own RFC gateway. See
`pkg/mcpext/example_test.go` for a complete extension.

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
