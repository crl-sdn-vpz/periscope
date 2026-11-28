package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func uninstallPlan(keepConfig bool) ([]removal, []string, error) {
	local := filepath.Join(os.Getenv("LOCALAPPDATA"), "periscope")
	if err := ownDir(local); err != nil { // an empty LOCALAPPDATA would make this relative
		return nil, nil, err
	}
	bin := filepath.Join(local, "bin")
	plan := []removal{{"stop the sender", stopRoles}}
	if exec.Command("schtasks", "/Query", "/TN", "PeriscopeSender").Run() == nil {
		plan = append(plan, removal{"the logon task from older versions (PeriscopeSender)", func() error {
			return exec.Command("schtasks", "/Delete", "/TN", "PeriscopeSender", "/F").Run()
		}})
	}
	if programs, err := windows.KnownFolderPath(windows.FOLDERID_Programs, 0); err == nil {
		menu := filepath.Join(programs, "Periscope")
		if filepath.IsAbs(menu) && exists(menu) {
			plan = append(plan, removal{"the Start menu folder Periscope", func() error { return removePath(menu) }})
		}
	}
	plan = append(plan, removal{bin + " from your PATH", func() error { return removeFromUserPath(bin) }})
	if !keepConfig {
		r, err := configRemoval()
		if err != nil {
			return nil, nil, err
		}
		plan = append(plan, r)
	}
	plan = append(plan, removal{"the program, logs and older versions (" + local + ")", func() error {
		return removeAfterExit(local)
	}})
	notes := []string{"Kept: ffmpeg (installed with winget; remove it with: winget uninstall Gyan.FFmpeg)."}
	return plan, notes, nil
}

// removeFromUserPath drops dir from the user's PATH (HKCU\Environment) and tells running programs.
func removeFromUserPath(dir string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, "Environment", registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	val, typ, err := k.GetStringValue("Path")
	if err == registry.ErrNotExist {
		return nil
	}
	if err != nil {
		return err
	}
	var keep []string
	for _, p := range strings.Split(val, ";") {
		if p != "" && !strings.EqualFold(strings.TrimRight(p, `\`), strings.TrimRight(dir, `\`)) {
			keep = append(keep, p)
		}
	}
	joined := strings.Join(keep, ";")
	if joined == val {
		return nil
	}
	if typ == registry.EXPAND_SZ {
		err = k.SetExpandStringValue("Path", joined)
	} else {
		err = k.SetStringValue("Path", joined)
	}
	if err != nil {
		return err
	}
	broadcastEnvironmentChange()
	return nil
}

// broadcastEnvironmentChange lets Explorer pick up the new PATH without signing out.
func broadcastEnvironmentChange() {
	env, _ := windows.UTF16PtrFromString("Environment")
	var result uintptr
	const hwndBroadcast, wmSettingChange, smtoAbortIfHung = 0xffff, 0x001A, 0x0002
	_, _, _ = windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW").Call(
		hwndBroadcast, wmSettingChange, 0, uintptr(unsafe.Pointer(env)), smtoAbortIfHung, 2000,
		uintptr(unsafe.Pointer(&result)))
}

// removeAfterExit deletes dir: what it can right away, and the rest (at least the running
// periscope.exe, which Windows won't delete) from a hidden cmd.exe that retries for ~15 s after this
// process exits. Retrying also rides out short locks such as an antivirus scanning a fresh log.
func removeAfterExit(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, e := range entries {
		if !strings.EqualFold(e.Name(), "bin") {
			_ = removePath(filepath.Join(dir, e.Name())) // best effort: the retry loop below finishes it
		}
	}
	loop := fmt.Sprintf(`for /l %%i in (1,1,15) do @(ping -n 2 127.0.0.1 >nul & rmdir /s /q "%s" 2>nul & if not exist "%s" exit /b 0)`, dir, dir)
	cmd := exec.Command("cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		// The exact command line: exec's argument quoting (\") is not what cmd.exe understands.
		CmdLine:       "cmd.exe /d /c " + loop,
		HideWindow:    true,
		CreationFlags: windows.CREATE_NO_WINDOW | windows.CREATE_NEW_PROCESS_GROUP,
	}
	return cmd.Start()
}
