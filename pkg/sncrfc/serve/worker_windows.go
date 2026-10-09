package serve

import (
	"os/exec"
	"syscall"
)

func hideWorker(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }
