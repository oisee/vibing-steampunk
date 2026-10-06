//go:build !windows

package adt

import "os"

// attachKillOnClose is a no-op outside Windows: the child reads EOF on stdin
// when vsp exits, however it exits, and is expected to end then.
func attachKillOnClose(*os.Process) (func(), error) {
	return nil, nil
}
