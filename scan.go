package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

var markdownExt = map[string]bool{
	".md": true, ".markdown": true, ".mdown": true, ".mkd": true, ".mkdn": true, ".mdwn": true,
}

func isMarkdown(path string) bool {
	return markdownExt[strings.ToLower(filepath.Ext(path))]
}

// Cartelle che non contengono documentazione da leggere e che possono
// essere enormi.
var skipDirs = map[string]bool{
	"node_modules": true, "__pycache__": true, "$recycle.bin": true, "system volume information": true,
}

const (
	maxVisited = 200000 // voci di disco guardate prima di fermarsi
	maxFiles   = 20000  // file markdown mostrati al massimo
)

// Node è una voce dell'albero nella barra laterale.
type Node struct {
	Name     string  `json:"name"`
	Path     string  `json:"path"`
	Dir      bool    `json:"dir,omitempty"`
	Children []*Node `json:"children,omitempty"`
}

// Folder è una cartella aperta con i suoi file markdown.
type Folder struct {
	Path      string  `json:"path"`
	Name      string  `json:"name"`
	Tree      []*Node `json:"tree"`
	Count     int     `json:"count"`
	Truncated bool    `json:"truncated"`
	Missing   bool    `json:"missing"`

	visited int
	sig     string
	files   map[string]bool
}

// ScanFolder cerca i file markdown sotto root, saltando cartelle nascoste
// e dipendenze, e tiene solo i rami che ne contengono.
func ScanFolder(root string) *Folder {
	f := &Folder{Path: root, Name: filepath.Base(root), files: map[string]bool{}}
	if f.Name == "." || f.Name == string(filepath.Separator) || f.Name == "" {
		f.Name = root
	}
	if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
		f.Missing = true
		return f
	}

	dirs := map[string]*Node{root: {Path: root, Dir: true}}
	var sig strings.Builder
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() && p != root {
				return filepath.SkipDir
			}
			return nil
		}
		f.visited++
		if f.visited > maxVisited || f.Count >= maxFiles {
			f.Truncated = true
			return filepath.SkipAll
		}
		if p == root {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if strings.HasPrefix(name, ".") || skipDirs[strings.ToLower(name)] {
				return filepath.SkipDir
			}
			dirs[p] = &Node{Name: name, Path: p, Dir: true}
			return nil
		}
		// I file nascosti comprendono i "._nome.md" che macOS lascia su
		// chiavette e dischi di rete: non sono testo.
		if !isMarkdown(name) || strings.HasPrefix(name, ".") || !d.Type().IsRegular() && d.Type()&fs.ModeSymlink == 0 {
			return nil
		}
		parent := dirs[filepath.Dir(p)]
		if parent == nil {
			return nil
		}
		parent.Children = append(parent.Children, &Node{Name: name, Path: p})
		f.files[pathKey(p)] = true
		f.Count++
		sig.WriteString(p)
		sig.WriteByte(0)
		return nil
	})

	// Aggancia le cartelle ai genitori partendo dalle più profonde, così
	// una cartella resta solo se sotto c'è almeno un file.
	paths := make([]string, 0, len(dirs))
	for p := range dirs {
		if p != root {
			paths = append(paths, p)
		}
	}
	sort.Slice(paths, func(i, j int) bool { return len(paths[i]) > len(paths[j]) })
	for _, p := range paths {
		n := dirs[p]
		if len(n.Children) == 0 {
			continue
		}
		if parent := dirs[filepath.Dir(p)]; parent != nil {
			parent.Children = append(parent.Children, n)
		}
	}
	sortTree(dirs[root].Children)
	f.Tree = dirs[root].Children
	f.sig = sig.String()
	return f
}

func sortTree(nodes []*Node) {
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].Dir != nodes[j].Dir {
			return nodes[i].Dir
		}
		return naturalLess(nodes[i].Name, nodes[j].Name)
	})
	for _, n := range nodes {
		if n.Dir {
			sortTree(n.Children)
		}
	}
}

// naturalLess confronta senza badare alle maiuscole e mette "2" prima di "10".
func naturalLess(a, b string) bool {
	a, b = strings.ToLower(a), strings.ToLower(b)
	for a != "" && b != "" {
		na, ra := leadingDigits(a)
		nb, rb := leadingDigits(b)
		if na != "" && nb != "" {
			ta, tb := strings.TrimLeft(na, "0"), strings.TrimLeft(nb, "0")
			if len(ta) != len(tb) {
				return len(ta) < len(tb)
			}
			if ta != tb {
				return ta < tb
			}
			a, b = ra, rb
			continue
		}
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		a, b = a[1:], b[1:]
	}
	return len(a) < len(b)
}

func leadingDigits(s string) (string, string) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i], s[i:]
}

// readme sceglie il file da mostrare quando si apre una cartella.
func (f *Folder) readme() string {
	for _, want := range []string{"readme.md", "index.md", "readme.markdown"} {
		for _, n := range f.Tree {
			if !n.Dir && strings.ToLower(n.Name) == want {
				return n.Path
			}
		}
	}
	var first func([]*Node) string
	first = func(nodes []*Node) string {
		for _, n := range nodes {
			if !n.Dir {
				return n.Path
			}
		}
		for _, n := range nodes {
			if p := first(n.Children); p != "" {
				return p
			}
		}
		return ""
	}
	return first(f.Tree)
}

func (f *Folder) has(path string) bool { return f.files[pathKey(path)] }

// pathKey rende confrontabili i percorsi: su Windows le maiuscole non contano.
func pathKey(p string) string {
	p = filepath.Clean(p)
	if runtime.GOOS == "windows" {
		return strings.ToLower(p)
	}
	return p
}
