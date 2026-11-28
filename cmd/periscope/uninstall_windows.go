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

func uninstallPlan(keepConfig bool) ([]removal, []string) {
	local := filepath.Join(os.Getenv("LOCALAPPDATA"), "periscope")
	bin := filepath.Join(local, "bin")
	plan := []removal{{"stop the sender", stopRoles}}
	if exec.Command("schtasks", "/Query", "/TN", "PeriscopeSender").Run() == nil {
		plan = append(plan, removal{"the logon task from older versions (PeriscopeSender)", func() error {
			return exec.Command("schtasks", "/Delete", "/TN", "PeriscopeSender", "/F").Run()
		}})
	}
	if programs, err := windows.KnownFolderPath(windows.FOLDERID_Programs, 0); err == nil {
		menu := filepath.Join(programs, "Periscope")
		if exists(menu) {
			plan = append(plan, removal{"the Start menu folder Periscope", func() error { return removePath(menu) }})
		}
	}
	plan = append(plan, removal{bin + " from your PATH", func() error { return removeFromUserPath(bin) }})
	if !keepConfig {
		if p, err := configPath(); err == nil {
			dir := filepath.Dir(p)
			plan = append(plan, removal{"config and passphrase (" + dir + ")", func() error { return removePath(dir) }})
		}
	}
	plan = append(plan, removal{"the program, logs and older versions (" + local + ")", func() error {
		return removeAfterExit(local)
	}})
	notes := []string{"Kept: ffmpeg (installed with winget; remove it with: winget uninstall Gyan.FFmpeg)."}
	return plan, notes
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

// removeAfterExit deletes dir a few seconds after this process exits: it holds periscope.exe, which
// Windows won't delete while it runs. A hidden cmd.exe waits, then removes the folder.
func removeAfterExit(dir string) error {
	cmd := exec.Command("cmd.exe", "/c", fmt.Sprintf(`ping -n 4 127.0.0.1 >nul & rmdir /s /q "%s"`, dir))
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_NO_WINDOW | windows.CREATE_NEW_PROCESS_GROUP,
	}
	return cmd.Start()
}
