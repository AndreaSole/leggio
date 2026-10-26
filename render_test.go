package main

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func render(t *testing.T, md string) string {
	t.Helper()
	return RenderBytes([]byte(md), "/docs").HTML
}

func mustContain(t *testing.T, html string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if !strings.Contains(html, p) {
			t.Errorf("manca %q in:\n%s", p, html)
		}
	}
}

func mustNotContain(t *testing.T, html string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if strings.Contains(html, p) {
			t.Errorf("non doveva esserci %q in:\n%s", p, html)
		}
	}
}

func TestHeadingIDsLikeGitHub(t *testing.T) {
	html := render(t, "# Perché usarlo?\n\n## Perché usarlo?\n\n### API `v2` & co.\n")
	mustContain(t, html, `id="perché-usarlo"`, `id="perché-usarlo-1"`, `id="api-v2--co"`)
}

func TestSanitizesScripts(t *testing.T) {
	html := render(t, "<script>alert(1)</script>\n\n<img src=x onerror=alert(1)>\n\n[a](javascript:alert(1))\n\n<iframe src=\"https://x\"></iframe>\n")
	mustNotContain(t, html, "<script", "onerror", "javascript:", "<iframe")
}

func TestKeepsGitHubReadmeHTML(t *testing.T) {
	html := render(t, "<p align=\"center\"><img src=\"logo.png\" width=\"120\"></p>\n\n<details><summary>Altro</summary>\n\ntesto\n\n</details>\n\n<kbd>Ctrl</kbd>\n")
	mustContain(t, html, `align="center"`, `width="120"`, "<details>", "<summary>Altro</summary>", "<kbd>Ctrl</kbd>")
}

func TestLocalImagesGoThroughHandler(t *testing.T) {
	html := render(t, "![a](img/foto%201.png) ![b](https://example.com/b.png) ![c](data:image/png;base64,AAAA)\n")
	want := "/__file?p=" + url.QueryEscape(filepath.Join("/docs", "img", "foto 1.png"))
	mustContain(t, html, strings.ReplaceAll(want, "&", "&amp;"), `src="https://example.com/b.png"`, `src="data:image/png;base64,AAAA"`)
}

func TestTablesTasksAlerts(t *testing.T) {
	html := render(t, "| a | b |\n|:--|--:|\n| 1 | 2 |\n\n- [x] fatto\n- [ ] da fare\n\n> [!WARNING]\n> Occhio qui.\n")
	mustContain(t, html,
		`<div class="table-wrap"><table>`, `align="right"`,
		`<ul class="tasks">`, `<li class="task"><input checked="" disabled="" type="checkbox"/>`,
		`<div class="alert alert-warning"><p class="alert-title">Attenzione</p>`, "Occhio qui.")
	mustNotContain(t, html, "[!WARNING]")
}

func TestCodeAndMermaid(t *testing.T) {
	doc := RenderBytes([]byte("```go\nfunc main() {}\n```\n\n```mermaid\ngraph TD; A-->B\n```\n"), "/docs")
	mustContain(t, doc.HTML, `class="chroma"`, `<div class="mermaid">graph TD; A--&gt;B`)
	if !doc.Mermaid {
		t.Error("il diagramma mermaid non è stato notato")
	}
}

func TestFrontMatterHidden(t *testing.T) {
	html := render(t, "---\ntitle: Prova\ntags: [a]\n---\n# Titolo\n")
	mustNotContain(t, html, "title: Prova")
	mustContain(t, html, "Titolo")

	// Un --- iniziale senza chiavi YAML resta una riga orizzontale.
	html = render(t, "---\nTesto normale\n\n---\n")
	mustContain(t, html, "Testo normale")
}

func TestFootnotesAndTitle(t *testing.T) {
	doc := RenderBytes([]byte("# Il titolo\n\nUna nota[^1].\n\n[^1]: Ecco.\n"), "/docs")
	if doc.Title != "Il titolo" {
		t.Errorf("titolo = %q", doc.Title)
	}
	mustContain(t, doc.HTML, `href="#fn:1"`, `id="fn:1"`)
}

func TestDecodeText(t *testing.T) {
	if got := string(decodeText([]byte("\xEF\xBB\xBFciao"))); got != "ciao" {
		t.Errorf("BOM: %q", got)
	}
	if got := string(decodeText([]byte("perch\xe9"))); got != "perché" {
		t.Errorf("Windows-1252: %q", got)
	}
	utf16 := []byte{0xFF, 0xFE, 'o', 0, 'k', 0}
	if got := string(decodeText(utf16)); got != "ok" {
		t.Errorf("UTF-16: %q", got)
	}
}

func TestLocalPath(t *testing.T) {
	cases := map[string]string{
		"altro.md":            filepath.Join("/docs", "altro.md"),
		"../su/giù.md":        filepath.Join("/su", "giù.md"),
		"sotto/a%20b.md":      filepath.Join("/docs", "sotto", "a b.md"),
		"file:///tmp/x.md":    filepath.FromSlash("/tmp/x.md"),
		"img.png?raw=true":    filepath.Join("/docs", "img.png"),
		"https://example.com": "",
		"#ancora":             "",
		"mailto:a@b.c":        "",
	}
	for in, want := range cases {
		got, ok := localPath("/docs", in)
		if want == "" && ok || want != "" && got != want {
			t.Errorf("localPath(%q) = %q, %v; voglio %q", in, got, ok, want)
		}
	}
}

func TestScanFolder(t *testing.T) {
	root := t.TempDir()
	write := func(rel string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte("# x"), 0o644)
	}
	write("README.md")
	write("docs/guida.MD")
	write("docs/capitolo10.md")
	write("docs/capitolo2.md")
	write("vuota/nota.txt")
	write(".git/info.md")
	write("._README.md")
	write("node_modules/pkg/README.md")

	f := ScanFolder(root)
	if f.Count != 4 {
		t.Fatalf("trovati %d file, voglio 4", f.Count)
	}
	if len(f.Tree) != 2 || !f.Tree[0].Dir || f.Tree[0].Name != "docs" || f.Tree[1].Name != "README.md" {
		t.Fatalf("albero inatteso: %+v", f.Tree)
	}
	var names []string
	for _, n := range f.Tree[0].Children {
		names = append(names, n.Name)
	}
	if strings.Join(names, ",") != "capitolo2.md,capitolo10.md,guida.MD" {
		t.Errorf("ordine: %v", names)
	}
	if r := f.readme(); filepath.Base(r) != "README.md" {
		t.Errorf("readme = %q", r)
	}
}
