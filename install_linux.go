package main

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

//go:embed build/linux/leggio-256.png
var icon256 []byte

// install registra Leggio presso il desktop (GNOME, KDE, XFCE...): voce nel
// menu, "Apri con" per i .md e programma predefinito per il markdown.
func install() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, _ = filepath.EvalSymlinks(exe)
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		data = filepath.Join(home, ".local", "share")
	}
	apps := filepath.Join(data, "applications")
	icons := filepath.Join(data, "icons", "hicolor", "256x256", "apps")
	for _, d := range []string{apps, icons} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(icons, "leggio.png"), icon256, 0o644); err != nil {
		return err
	}
	entry := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=Leggio
GenericName=Visualizzatore markdown
Comment=Legge i file markdown già impaginati
Exec="%s" %%F
Icon=leggio
Terminal=false
Categories=Utility;TextTools;Viewer;
MimeType=text/markdown;text/x-markdown;
StartupWMClass=leggio
`, exe)
	if err := os.WriteFile(filepath.Join(apps, "leggio.desktop"), []byte(entry), 0o644); err != nil {
		return err
	}
	_ = exec.Command("update-desktop-database", apps).Run()
	_ = exec.Command("gtk-update-icon-cache", "-f", "-t", filepath.Join(data, "icons", "hicolor")).Run()
	if err := exec.Command("xdg-mime", "default", "leggio.desktop", "text/markdown", "text/x-markdown").Run(); err != nil {
		fmt.Println("Leggio è nel menu e in \"Apri con\"; per renderlo predefinito scegli Leggio dalle proprietà di un file .md.")
	} else {
		fmt.Println("Fatto: Leggio è nel menu e apre i file .md con un doppio clic.")
	}
	fmt.Println("Se sposti l'eseguibile, rilancia leggio --install.")
	return nil
}

func uninstall() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		data = filepath.Join(home, ".local", "share")
	}
	_ = os.Remove(filepath.Join(data, "applications", "leggio.desktop"))
	_ = os.Remove(filepath.Join(data, "icons", "hicolor", "256x256", "apps", "leggio.png"))
	_ = exec.Command("update-desktop-database", filepath.Join(data, "applications")).Run()
	fmt.Println("Leggio tolto dal menu e dalle associazioni dei file.")
	return nil
}
