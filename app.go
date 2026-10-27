package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// UI sono le preferenze della finestra che la vista salva.
type UI struct {
	Theme         string   `json:"theme"` // "auto", "light" o "dark"
	SidebarWidth  int      `json:"sidebarWidth"`
	SidebarHidden bool     `json:"sidebarHidden"`
	Zoom          float64  `json:"zoom"`
	Expanded      []string `json:"expanded"`
}

type FileEntry struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Missing bool   `json:"missing"`
}

// State è tutto ciò che serve alla vista per disegnare la barra laterale.
type State struct {
	Files      []FileEntry `json:"files"`
	Folders    []*Folder   `json:"folders"`
	Current    string      `json:"current"`
	UI         UI          `json:"ui"`
	SystemDark bool        `json:"systemDark"`
	Sep        string      `json:"sep"`
	Noted      []string    `json:"noted"` // documenti con almeno un appunto
}

// Change è la risposta alle azioni: il nuovo stato e il file da mostrare.
type Change struct {
	State *State `json:"state"`
	Show  string `json:"show"`
}

type saved struct {
	Files   []string `json:"files"`
	Folders []string `json:"folders"`
	Current string   `json:"current"`
	UI      UI       `json:"ui"`
}

type App struct {
	ctx context.Context

	mu      sync.Mutex
	scanMu  sync.Mutex // una sola scansione delle cartelle alla volta
	files   []string
	folders []*Folder
	current string
	ui      UI
	notes   *NoteStore

	watched    string
	watchedMod time.Time
	watchedLen int64
}

func NewApp(args []string) *App {
	a := &App{ui: UI{Theme: "auto", SidebarWidth: 260, Zoom: 1}, notes: OpenNotes(notesPath())}
	a.load()
	if show := a.addPaths(args); show != "" {
		a.current = show
	}
	a.save()
	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	applyWindowTheme(ctx, a.ui.Theme)
	go a.watch()
}

// onSecondInstance riceve i file aperti mentre MdView è già in esecuzione.
func (a *App) onSecondInstance(data secondInstance) {
	a.external(data.Args)
	wruntime.WindowUnminimise(a.ctx)
	wruntime.WindowShow(a.ctx)
	wruntime.WindowSetAlwaysOnTop(a.ctx, true)
	go func() {
		time.Sleep(150 * time.Millisecond)
		wruntime.WindowSetAlwaysOnTop(a.ctx, false)
	}()
}

// onFileOpen è l'"Apri con" del Finder su macOS.
func (a *App) onFileOpen(path string) { a.external([]string{path}) }

// external accoglie i file arrivati da fuori. Il file da mostrare diventa
// subito quello corrente, così se la vista non è ancora pronta a ricevere
// l'evento lo trova comunque chiedendo lo stato.
func (a *App) external(paths []string) {
	show := a.addPaths(paths)
	if show != "" {
		a.mu.Lock()
		a.current = show
		a.mu.Unlock()
	}
	a.save()
	if a.ctx != nil {
		wruntime.EventsEmit(a.ctx, "change", &Change{State: a.GetState(), Show: show})
	}
}

// --- metodi chiamati dalla vista ---

func (a *App) GetState() *State {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := &State{
		Folders:    a.folders,
		Current:    a.current,
		UI:         a.ui,
		SystemDark: systemPrefersDark(),
		Sep:        string(filepath.Separator),
		Noted:      a.notes.Noted(),
	}
	for _, p := range a.files {
		_, err := os.Stat(p)
		s.Files = append(s.Files, FileEntry{Path: p, Name: filepath.Base(p), Missing: err != nil})
	}
	if s.Files == nil {
		s.Files = []FileEntry{}
	}
	if s.Folders == nil {
		s.Folders = []*Folder{}
	}
	return s
}

func (a *App) AddPaths(paths []string) *Change {
	show := a.addPaths(paths)
	a.save()
	return &Change{State: a.GetState(), Show: show}
}

var markdownFilter = []wruntime.FileFilter{
	{DisplayName: "Markdown", Pattern: "*.md;*.markdown;*.mdown;*.mkd;*.mkdn;*.mdwn"},
	{DisplayName: "Tutti i file", Pattern: "*.*"},
}

func (a *App) OpenFileDialog() (*Change, error) {
	paths, err := wruntime.OpenMultipleFilesDialog(a.ctx, wruntime.OpenDialogOptions{
		Title:            "Apri file markdown",
		DefaultDirectory: a.lastDir(),
		Filters:          markdownFilter,
	})
	if err != nil || len(paths) == 0 {
		return nil, err
	}
	return a.AddPaths(paths), nil
}

func (a *App) OpenFolderDialog() (*Change, error) {
	dir, err := wruntime.OpenDirectoryDialog(a.ctx, wruntime.OpenDialogOptions{
		Title:            "Apri cartella",
		DefaultDirectory: a.lastDir(),
	})
	if err != nil || dir == "" {
		return nil, err
	}
	return a.AddPaths([]string{dir}), nil
}

func (a *App) CloseFile(path string) *State {
	a.mu.Lock()
	for i, p := range a.files {
		if pathKey(p) == pathKey(path) {
			a.files = append(a.files[:i], a.files[i+1:]...)
			break
		}
	}
	a.mu.Unlock()
	a.save()
	return a.GetState()
}

func (a *App) CloseFolder(path string) *State {
	a.mu.Lock()
	for i, f := range a.folders {
		if pathKey(f.Path) == pathKey(path) {
			a.folders = append(a.folders[:i], a.folders[i+1:]...)
			break
		}
	}
	a.mu.Unlock()
	a.save()
	return a.GetState()
}

// Render legge e converte un file e lo segna come quello da sorvegliare.
func (a *App) Render(path string) (*Doc, error) {
	doc, err := RenderFile(path)
	if err != nil {
		return nil, err
	}
	doc.Notes = a.notes.For(path)
	for _, n := range doc.Notes {
		n.HTML = RenderNote(n.Text, filepath.Dir(path))
	}
	a.mu.Lock()
	a.current = path
	a.watched = path
	a.watchedMod, a.watchedLen = stamp(path)
	a.mu.Unlock()
	a.save()
	if a.ctx != nil {
		wruntime.WindowSetTitle(a.ctx, filepath.Base(path)+" — MdView")
	}
	return doc, nil
}

// SaveNote scrive l'appunto di una sezione (section = id del titolo) o
// del file (section vuota). Con il testo vuoto lo cancella e restituisce nil.
func (a *App) SaveNote(path, section, heading, text string) (*Note, error) {
	n, err := a.notes.Set(path, section, heading, text)
	if err != nil || n == nil {
		return nil, err
	}
	n.HTML = RenderNote(n.Text, filepath.Dir(path))
	return n, nil
}

// Clear dice che non si sta guardando nessun file.
func (a *App) Clear() {
	a.mu.Lock()
	a.current, a.watched = "", ""
	a.mu.Unlock()
	a.save()
	if a.ctx != nil {
		wruntime.WindowSetTitle(a.ctx, "MdView")
	}
}

// Target dice alla vista cosa fare di un collegamento cliccato.
type Target struct {
	Kind     string  `json:"kind"` // "md", "done" o "none"
	Path     string  `json:"path,omitempty"`
	Fragment string  `json:"fragment,omitempty"`
	Change   *Change `json:"change,omitempty"`
}

// Follow segue un collegamento: i markdown si aprono qui, il web nel
// browser, gli altri file con il loro programma o nella loro cartella.
func (a *App) Follow(docPath, href string) *Target {
	href = strings.TrimSpace(href)
	lower := strings.ToLower(href)
	for _, s := range []string{"http://", "https://", "mailto:"} {
		if strings.HasPrefix(lower, s) {
			wruntime.BrowserOpenURL(a.ctx, href)
			return &Target{Kind: "done"}
		}
	}
	fragment := ""
	if i := strings.LastIndex(href, "#"); i >= 0 {
		href, fragment = href[:i], href[i+1:]
	}
	p, ok := localPath(filepath.Dir(docPath), href)
	if !ok {
		return &Target{Kind: "none"}
	}
	fi, err := os.Stat(p)
	switch {
	case err != nil:
		return &Target{Kind: "none", Path: p}
	case fi.IsDir():
		openWithSystem(p)
		return &Target{Kind: "done"}
	case isMarkdown(p):
		ch := a.AddPaths([]string{p})
		return &Target{Kind: "md", Path: p, Fragment: fragment, Change: ch}
	case safeToOpen(p):
		openWithSystem(p)
	default:
		revealInFolder(p)
	}
	return &Target{Kind: "done"}
}

func (a *App) Reveal(path string) { revealInFolder(path) }

func (a *App) SaveUI(ui UI) {
	a.mu.Lock()
	changedTheme := ui.Theme != a.ui.Theme
	a.ui = ui
	a.mu.Unlock()
	a.save()
	if changedTheme {
		applyWindowTheme(a.ctx, ui.Theme)
	}
}

// Rescan rilegge le cartelle aperte, per esempio quando la finestra torna in primo piano.
func (a *App) Rescan() *State {
	a.rescan(true)
	return a.GetState()
}

// --- interni ---

func (a *App) addPaths(paths []string) (show string) {
	for _, p := range paths {
		if p == "" || strings.HasPrefix(p, "-") {
			continue
		}
		p, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		if fi.IsDir() {
			f := a.folderFor(p, true)
			if f == nil {
				f = ScanFolder(p)
				a.mu.Lock()
				a.folders = append(a.folders, f)
				// I file già aperti a parte ora stanno nell'albero.
				kept := a.files[:0]
				for _, q := range a.files {
					if !f.has(q) {
						kept = append(kept, q)
					}
				}
				a.files = kept
				a.mu.Unlock()
			}
			if r := f.readme(); r != "" {
				show = r
			}
			continue
		}
		if a.folderFor(p, false) == nil && !a.hasFile(p) {
			a.mu.Lock()
			a.files = append(a.files, p)
			a.mu.Unlock()
		}
		show = p
	}
	return show
}

// folderFor trova la cartella aperta che è dir (exact) o che contiene il file.
func (a *App) folderFor(p string, exact bool) *Folder {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, f := range a.folders {
		if exact && pathKey(f.Path) == pathKey(p) || !exact && f.has(p) {
			return f
		}
	}
	return nil
}

func (a *App) hasFile(p string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, q := range a.files {
		if pathKey(q) == pathKey(p) {
			return true
		}
	}
	return false
}

func (a *App) lastDir() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.current != "" {
		return filepath.Dir(a.current)
	}
	return ""
}

// watch ricarica il file aperto quando cambia su disco e ogni tanto
// riguarda le cartelle per i file nuovi o tolti.
func (a *App) watch() {
	tick := time.NewTicker(700 * time.Millisecond)
	defer tick.Stop()
	for n := 1; ; n++ {
		<-tick.C
		a.mu.Lock()
		path, mod, size := a.watched, a.watchedMod, a.watchedLen
		a.mu.Unlock()
		if path != "" {
			if m, s := stamp(path); !m.Equal(mod) || s != size {
				a.mu.Lock()
				if a.watched == path {
					a.watchedMod, a.watchedLen = m, s
				}
				a.mu.Unlock()
				wruntime.EventsEmit(a.ctx, "doc-changed", path)
			}
		}
		if n%4 == 0 && a.rescan(false) {
			wruntime.EventsEmit(a.ctx, "change", &Change{State: a.GetState()})
		}
	}
}

// rescan riscansiona le cartelle; senza force salta quelle troppo grandi
// per essere guardate di continuo. Dice se qualcosa è cambiato.
func (a *App) rescan(force bool) bool {
	if !a.scanMu.TryLock() {
		return false
	}
	defer a.scanMu.Unlock()
	a.mu.Lock()
	folders := append([]*Folder(nil), a.folders...)
	a.mu.Unlock()
	changed := false
	for _, old := range folders {
		if !force && old.visited > 20000 {
			continue
		}
		f := ScanFolder(old.Path)
		if f.sig == old.sig && f.Missing == old.Missing {
			continue
		}
		changed = true
		a.mu.Lock()
		for i := range a.folders {
			if a.folders[i] == old {
				a.folders[i] = f
			}
		}
		a.mu.Unlock()
	}
	return changed
}

func stamp(path string) (time.Time, int64) {
	fi, err := os.Stat(path)
	if err != nil {
		return time.Time{}, -1
	}
	return fi.ModTime(), fi.Size()
}

func statePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "MdView", "stato.json")
}

func (a *App) load() {
	data, err := os.ReadFile(statePath())
	if err != nil {
		return
	}
	var s saved
	if json.Unmarshal(data, &s) != nil {
		return
	}
	if s.UI.Theme != "" {
		a.ui = s.UI
	}
	if a.ui.Zoom <= 0 {
		a.ui.Zoom = 1
	}
	for _, p := range s.Folders {
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			a.folders = append(a.folders, ScanFolder(p))
		}
	}
	for _, p := range s.Files {
		if _, err := os.Stat(p); err == nil {
			a.files = append(a.files, p)
		}
	}
	if _, err := os.Stat(s.Current); err == nil {
		a.current = s.Current
	}
}

func (a *App) save() {
	path := statePath()
	if path == "" {
		return
	}
	a.mu.Lock()
	s := saved{Files: a.files, Current: a.current, UI: a.ui}
	for _, f := range a.folders {
		s.Folders = append(s.Folders, f.Path)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	a.mu.Unlock()
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	tmp := path + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil {
		_ = os.Rename(tmp, path)
	}
}
