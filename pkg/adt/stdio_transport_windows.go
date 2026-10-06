//go:build windows

package adt

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// attachKillOnClose puts the child into a Job Object that kills every process
// in it when the job's last handle closes. vsp holds that handle, so the child
// ends with vsp even when vsp is killed and never gets to close its stdin.
// The returned func closes the handle (and so ends the child).
//
// The child is assigned after it has started, so there is a short window in
// which it runs outside the job: a grandchild it starts in that window is not
// in the job and does not end with vsp. The process is deliberately not
// created suspended (that would need CreateProcess by hand instead of
// os/exec); instead the helper contract says to start no children before the
// first frame arrives, and by then the assignment has long happened, because
// vsp writes the first frame only after attachKillOnClose has returned.
func attachKillOnClose(p *os.Process) (func(), error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("CreateJobObject: %w", err)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return nil, fmt.Errorf("SetInformationJobObject: %w", err)
	}
	proc, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(p.Pid))
	if err != nil {
		windows.CloseHandle(job)
		return nil, fmt.Errorf("OpenProcess: %w", err)
	}
	defer windows.CloseHandle(proc)
	if err := windows.AssignProcessToJobObject(job, proc); err != nil {
		windows.CloseHandle(job)
		return nil, fmt.Errorf("AssignProcessToJobObject: %w", err)
	}
	return func() { windows.CloseHandle(job) }, nil
}
