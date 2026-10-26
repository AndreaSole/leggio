//go:build !windows

package main

import (
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

var (
	darkOnce sync.Once
	dark     bool
)

// systemPrefersDark serve soprattutto su Linux, dove WebKitGTK non sempre
// sa che GNOME o KDE sono in modalità scura.
func systemPrefersDark() bool {
	darkOnce.Do(func() {
		if runtime.GOOS == "darwin" {
			out, _ := exec.Command("defaults", "read", "-g", "AppleInterfaceStyle").Output()
			dark = strings.Contains(string(out), "Dark")
			return
		}
		if out, err := exec.Command("gsettings", "get", "org.gnome.desktop.interface", "color-scheme").Output(); err == nil &&
			strings.Contains(string(out), "dark") {
			dark = true
			return
		}
		if out, err := exec.Command("gsettings", "get", "org.gnome.desktop.interface", "gtk-theme").Output(); err == nil &&
			strings.Contains(strings.ToLower(string(out)), "dark") {
			dark = true
		}
	})
	return dark
}
