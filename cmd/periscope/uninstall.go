package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/crl-sdn-vpz/periscope/internal/ui"
)

// removal is one uninstall step: what it removes (shown before asking) and how.
type removal struct {
	what string
	run  func() error
}

// cmdUninstall removes periscope from this computer after showing the plan and asking (or --yes).
func cmdUninstall(args []string) (string, error) {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	yes := fs.Bool("yes", false, "don't ask for confirmation")
	keepConfig := fs.Bool("keep-config", false, "keep the config file (passphrase and settings)")
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	plan, notes := uninstallPlan(*keepConfig)

	fmt.Println(ui.Style(ui.Out, ui.Bold, "This removes periscope from this computer:"))
	for _, r := range plan {
		fmt.Println("  - " + r.what)
	}
	for _, n := range notes {
		fmt.Println(ui.Style(ui.Out, ui.Dim, "  "+n))
	}
	if !*yes {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return "", errors.New("not a terminal: add --yes to uninstall without confirmation")
		}
		fmt.Print("\nUninstall periscope? [y/N] ")
		answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(answer)); a != "y" && a != "yes" {
			return "nothing removed", nil
		}
	}
	fmt.Println()
	failed := 0
	for _, r := range plan {
		if err := r.run(); err != nil {
			failed++
			fmt.Println(ui.Fail(ui.Out, r.what+": "+err.Error()))
			continue
		}
		fmt.Println(ui.OK(ui.Out, r.what))
	}
	if failed > 0 {
		return "", fmt.Errorf("%d step(s) failed; the rest of periscope is removed", failed)
	}
	return "periscope is uninstalled", nil
}

// stopRoles stops the viewer (making faded windows solid) and the sender, if they run here.
func stopRoles() error {
	if _, err := cmdStop(); err != nil && !strings.Contains(err.Error(), "not running") {
		return err
	}
	if _, err := stopSender(); err != nil && !strings.Contains(err.Error(), "not running") {
		return err
	}
	return nil
}

// removePath deletes a file or directory tree; a missing one is fine.
func removePath(p string) error {
	if err := os.RemoveAll(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
