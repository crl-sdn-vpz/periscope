//go:build !windows

package ui

import "os"

func enableANSI(*os.File) bool { return true }
