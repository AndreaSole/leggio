//go:build !linux

package main

import "errors"

var errNoInstall = errors.New("--install serve solo su Linux: su Windows usa \"Apri con\" su un file .md e scegli Leggio")

func install() error   { return errNoInstall }
func uninstall() error { return errNoInstall }
