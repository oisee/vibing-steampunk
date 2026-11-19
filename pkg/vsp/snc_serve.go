package vsp

// `vsp snc-serve` and `vsp snc-serve-worker` are hidden: vsp starts them
// itself, as the transport command of a system with an "snc" block in
// ~/.vsp.json (see pkg/config, SNCSettings). They are dispatched before cobra
// and before ./.env is loaded, because stdout carries frames only and the
// worker's environment must be the one vsp handed over, not a project's .env.

import (
	"io"

	"github.com/oisee/vibing-steampunk/pkg/sncrfc/serve"
)

// sncServeInvocation reports whether argv runs one of the hidden SNC commands.
func sncServeInvocation(args []string) bool {
	return len(args) > 1 && (args[1] == serve.Command || args[1] == serve.WorkerCommand)
}

// runSNCServe runs the hidden SNC command named in args[1].
func runSNCServe(args []string, in io.Reader, out, errOut io.Writer) int {
	if args[1] == serve.WorkerCommand {
		return serve.WorkerMain(args[2:], in, out)
	}
	return serve.Main(args[2:], in, out, errOut)
}
