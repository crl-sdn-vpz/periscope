package proc

import (
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	jobOnce sync.Once
	job     windows.Handle // kept open for the life of the process; closing it kills the job
	jobErr  error
)

// joinKillOnCloseJob puts this process in a job that kills every member when its last handle closes,
// i.e. when this process exits. Children are created inside the job, and so are their children:
// assigning each child after it starts would race with launchers such as Chocolatey's ffmpeg shim,
// which start the real ffmpeg before the assignment and leave it orphaned.
func joinKillOnCloseJob() error {
	jobOnce.Do(func() {
		h, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			jobErr = fmt.Errorf("create job object: %w", err)
			return
		}
		info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
			BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
				LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
			},
		}
		if _, err := windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
			jobErr = fmt.Errorf("configure job object: %w", err)
			return
		}
		if err := windows.AssignProcessToJobObject(h, windows.CurrentProcess()); err != nil {
			jobErr = fmt.Errorf("join job object: %w", err)
			return
		}
		job = h
	})
	return jobErr
}

func tie(cmd *exec.Cmd) error {
	if err := joinKillOnCloseJob(); err != nil {
		return err
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	return nil
}

func waitExit(pid int, timeout time.Duration) bool {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return true // no such process (anymore)
	}
	defer windows.CloseHandle(h)
	ev, err := windows.WaitForSingleObject(h, uint32(timeout.Milliseconds()))
	return err == nil && ev == windows.WAIT_OBJECT_0
}

func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS,
	}
}
