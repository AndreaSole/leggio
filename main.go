package main

import (
	"bytes"
	"compress/gzip"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

const version = "0.1.0"

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var icon []byte

// mermaid.js pesa 5 MB: sta compresso nell'eseguibile e si scompatta
// solo se un documento contiene un diagramma.
//
//go:embed third_party/mermaid.min.js.gz
var mermaidGz []byte

var mermaidJS = sync.OnceValue(func() []byte {
	r, err := gzip.NewReader(bytes.NewReader(mermaidGz))
	if err != nil {
		return nil
	}
	b, _ := io.ReadAll(r)
	return b
})

type secondInstance = options.SecondInstanceData

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v":
			fmt.Println("Leggio", version)
			return
		case "--install":
			exitOn(install())
			return
		case "--uninstall":
			exitOn(uninstall())
			return
		}
	}
	// Percorsi assoluti subito: se c'è già una finestra aperta gli argomenti
	// le arrivano così come sono, e lei non sa da quale cartella venivano.
	for i, a := range os.Args[1:] {
		if abs, err := filepath.Abs(a); err == nil && !strings.HasPrefix(a, "-") {
			os.Args[i+1] = abs
		}
	}

	// Con alcuni driver (NVIDIA soprattutto) WebKitGTK disegna una finestra
	// bianca se usa i buffer DMA; per un documento da leggere non servono.
	if runtime.GOOS == "linux" && os.Getenv("WEBKIT_DISABLE_DMABUF_RENDERER") == "" {
		_ = os.Setenv("WEBKIT_DISABLE_DMABUF_RENDERER", "1")
	}

	app := NewApp(os.Args[1:])
	dist, _ := fs.Sub(assets, "frontend/dist")
	bg := &options.RGBA{R: 255, G: 255, B: 255, A: 255}
	if app.ui.Theme == "dark" || app.ui.Theme != "light" && systemPrefersDark() {
		bg = &options.RGBA{R: 31, G: 31, B: 31, A: 255}
	}

	err := wails.Run(&options.App{
		Title:            "Leggio",
		Width:            1180,
		Height:           800,
		MinWidth:         520,
		MinHeight:        360,
		BackgroundColour: bg,
		AssetServer: &assetserver.Options{
			Assets:  dist,
			Handler: http.HandlerFunc(serveLocal),
		},
		OnStartup:                app.startup,
		Bind:                     []interface{}{app},
		EnableDefaultContextMenu: true, // solo su testo selezionato, vedi style.css
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "6f1d2a4e-8c3b-4b7e-9a51-3e0f7c2d9b18",
			OnSecondInstanceLaunch: app.onSecondInstance,
		},
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop: true,
		},
		Windows: &windows.Options{
			Theme: windows.SystemDefault,
		},
		Linux: &linux.Options{
			Icon:             icon,
			ProgramName:      "leggio",
			WebviewGpuPolicy: linux.WebviewGpuPolicyOnDemand,
		},
		Mac: &mac.Options{
			OnFileOpen: app.onFileOpen,
			About:      &mac.AboutInfo{Title: "Leggio " + version, Message: "Visualizzatore di file markdown", Icon: icon},
		},
	})
	exitOn(err)
}

func exitOn(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "Leggio:", err)
		os.Exit(1)
	}
}

// Le immagini che i documenti prendono dal disco. Si servono solo tipi di
// immagine e video, e gli SVG senza script.
var mediaTypes = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif",
	".webp": "image/webp", ".svg": "image/svg+xml", ".bmp": "image/bmp", ".ico": "image/x-icon",
	".avif": "image/avif", ".mp4": "video/mp4", ".webm": "video/webm",
}

var highlightCSS = chromaCSS()

func serveLocal(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/__chroma.css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = w.Write(highlightCSS)
	case "/vendor/mermaid.min.js":
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = w.Write(mermaidJS())
	case "/__file":
		p := r.URL.Query().Get("p")
		ctype, ok := mediaTypes[strings.ToLower(filepath.Ext(p))]
		if !ok || !filepath.IsAbs(p) {
			http.Error(w, "non consentito", http.StatusForbidden)
			return
		}
		f, err := os.Open(p)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		fi, err := f.Stat()
		if err != nil || fi.IsDir() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", ctype)
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src data:")
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeContent(w, r, fi.Name(), fi.ModTime(), f)
	default:
		http.NotFound(w, r)
	}
}
