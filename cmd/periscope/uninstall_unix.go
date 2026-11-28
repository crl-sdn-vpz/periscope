//go:build !windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/crl-sdn-vpz/periscope/internal/paths"
)

func uninstallPlan(keepConfig bool) ([]removal, []string, error) {
	home, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(home) {
		return nil, nil, errors.New("HOME is not set to an absolute path: nothing removed")
	}
	data := os.Getenv("XDG_DATA_HOME")
	if !filepath.IsAbs(data) {
		data = filepath.Join(home, ".local", "share")
	}
	plan := []removal{{"stop the viewer and the sender (windows back to solid)", stopRoles}}
	if _, err := exec.LookPath("gsettings"); err == nil {
		plan = append(plan, removal{"periscope's GNOME hotkeys", removeGnomeBindings})
	}
	var files []string
	for _, f := range []string{
		filepath.Join(data, "applications", "periscope-toggle.desktop"),
		filepath.Join(data, "applications", "periscope-menu.desktop"),
		filepath.Join(data, "applications", "periscope-uninstall.desktop"),
		filepath.Join(data, "icons", "hicolor", "256x256", "apps", "periscope.png"),
	} {
		if exists(f) {
			files = append(files, f)
		}
	}
	if len(files) > 0 {
		plan = append(plan, removal{"app launchers and icon", func() error {
			for _, f := range files {
				if err := removePath(f); err != nil {
					return err
				}
			}
			return nil
		}})
	}
	if dir, err := paths.StateDir(); err == nil && ownDir(dir) == nil {
		plan = append(plan, removal{"logs and runtime files (" + dir + ")", func() error { return removePath(dir) }})
	}
	if !keepConfig {
		r, err := configRemoval()
		if err != nil {
			return nil, nil, err
		}
		plan = append(plan, r)
	}
	for _, b := range []string{filepath.Join(home, ".local", "bin", "periscope"), filepath.Join(home, ".local", "bin", "d1")} {
		if exists(b) {
			plan = append(plan, removal{"the program (" + b + ")", func() error { return removePath(b) }})
		}
	}
	port := "9000"
	if c, err := loadConfig(); err == nil {
		port = fmt.Sprint(c.Port)
	}
	notes := []string{
		"Kept: mpv, ffmpeg and the other system packages the installer added (other apps may use them).",
		"Kept: the firewall rule for UDP " + port + ", if the installer added one: sudo ufw status numbered,",
		"      then sudo ufw delete <number of the periscope rule>",
	}
	return plan, notes, nil
}

// removeGnomeBindings drops periscope's entries (also the v0.2 d1 ones, same ids) from GNOME's
// custom keybindings and resets their keys.
func removeGnomeBindings() error {
	const schema = "org.gnome.settings-daemon.plugins.media-keys"
	out, err := exec.Command("gsettings", "get", schema, "custom-keybindings").Output()
	if err != nil {
		return nil // no GNOME settings daemon schema here: nothing to remove
	}
	list := strings.TrimSpace(string(out))
	list = strings.TrimPrefix(list, "@as ")
	var keep, ours []string
	for _, e := range strings.Split(strings.Trim(list, "[]"), ",") {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if strings.Contains(e, "/periscope-") {
			ours = append(ours, strings.Trim(e, "'"))
		} else {
			keep = append(keep, e)
		}
	}
	if len(ours) == 0 {
		return nil
	}
	for _, path := range ours {
		for _, key := range []string{"name", "command", "binding"} {
			_ = exec.Command("gsettings", "reset", schema+".custom-keybinding:"+path, key).Run()
		}
	}
	return exec.Command("gsettings", "set", schema, "custom-keybindings", "["+strings.Join(keep, ", ")+"]").Run()
}
