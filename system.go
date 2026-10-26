package main

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Tipi che si possono aprire con il loro programma senza rischi: un
// collegamento in un README scaricato non deve poter lanciare un .exe.
var openableExt = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".svg": true, ".bmp": true,
	".pdf": true, ".txt": true, ".log": true, ".csv": true, ".json": true, ".xml": true, ".yaml": true, ".yml": true,
	".docx": true, ".xlsx": true, ".pptx": true, ".odt": true, ".ods": true, ".odp": true,
	".mp3": true, ".mp4": true, ".webm": true, ".ogg": true, ".wav": true,
}

func safeToOpen(path string) bool { return openableExt[strings.ToLower(filepath.Ext(path))] }

func openWithSystem(path string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	_ = cmd.Start()
}

func revealInFolder(path string) {
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("explorer", "/select,", path).Start()
	case "darwin":
		_ = exec.Command("open", "-R", path).Start()
	default:
		// Nautilus, Dolphin e Nemo capiscono la richiesta D-Bus e
		// selezionano il file; gli altri aprono almeno la cartella.
		uri := "file://" + filepath.ToSlash(path)
		err := exec.Command("dbus-send", "--session", "--dest=org.freedesktop.FileManager1",
			"--type=method_call", "/org/freedesktop/FileManager1",
			"org.freedesktop.FileManager1.ShowItems", "array:string:"+uri, "string:").Run()
		if err != nil {
			_ = exec.Command("xdg-open", filepath.Dir(path)).Start()
		}
	}
}

func applyWindowTheme(ctx context.Context, theme string) {
	if ctx == nil {
		return
	}
	switch theme {
	case "dark":
		wruntime.WindowSetDarkTheme(ctx)
	case "light":
		wruntime.WindowSetLightTheme(ctx)
	default:
		wruntime.WindowSetSystemDefaultTheme(ctx)
	}
}
