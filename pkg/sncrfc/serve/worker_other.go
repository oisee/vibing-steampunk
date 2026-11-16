//go:build !windows

package serve

import "os/exec"

func hideWorker(*exec.Cmd) {}
