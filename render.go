package main

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	emoji "github.com/yuin/goldmark-emoji"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	gmhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"golang.org/x/text/encoding/charmap"
	xunicode "golang.org/x/text/encoding/unicode"
)

// Doc è un file markdown già trasformato in HTML pronto per la vista.
type Doc struct {
	Path    string  `json:"path"`
	Title   string  `json:"title"`
	HTML    string  `json:"html"`
	Mermaid bool    `json:"mermaid"`
	Missing bool    `json:"missing"`
	Notes   []*Note `json:"notes"`
}

var (
	markdown = newMarkdown(false)
	// Negli appunti un a capo resta un a capo, e i titoli non hanno id: si
	// confonderebbero con quelli del documento a cui sono attaccati.
	noteMarkdown = newMarkdown(true)
	policy       = newPolicy()
)

func newMarkdown(forNotes bool) goldmark.Markdown {
	var parserOpts []parser.Option
	rendererOpts := []renderer.Option{
		gmhtml.WithUnsafe(), // l'HTML grezzo passa, poi lo ripulisce bluemonday
		renderer.WithNodeRenderers(util.Prioritized(newCodeRenderer(), 100)),
	}
	if forNotes {
		rendererOpts = append(rendererOpts, gmhtml.WithHardWraps())
	} else {
		parserOpts = append(parserOpts, parser.WithAutoHeadingID())
	}
	return goldmark.New(
		goldmark.WithExtensions(
			extension.Linkify,
			extension.Strikethrough,
			extension.TaskList,
			extension.Footnote,
			extension.DefinitionList,
			emoji.Emoji,
			extension.NewTable(extension.WithTableCellAlignMethod(extension.TableCellAlignAttribute)),
		),
		goldmark.WithParserOptions(parserOpts...),
		goldmark.WithRendererOptions(rendererOpts...),
	)
}

// newPolicy lascia passare quello che GitHub accetta nei README e nient'altro:
// niente script, stili in linea, iframe o form.
func newPolicy() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowAttrs("class").Matching(regexp.MustCompile(`^[\w\- ]*$`)).Globally()
	p.AllowAttrs("id").Matching(regexp.MustCompile(`^[\p{L}\p{M}\p{N}_:.\-]+$`)).Globally()
	p.AllowAttrs("role").Matching(regexp.MustCompile(`^doc-[a-z]+$`)).Globally()
	p.AllowAttrs("align").Matching(regexp.MustCompile(`(?i)^(left|right|center|justify)$`)).
		OnElements("p", "div", "h1", "h2", "h3", "h4", "h5", "h6", "img", "table", "td", "th")
	p.AllowAttrs("width", "height").Matching(regexp.MustCompile(`^\d+(%|px)?$`)).OnElements("img", "td", "th")
	p.AllowAttrs("start").Matching(bluemonday.Integer).OnElements("ol")
	p.AllowElements("input")
	p.AllowAttrs("type").Matching(regexp.MustCompile(`^checkbox$`)).OnElements("input")
	p.AllowAttrs("checked", "disabled").OnElements("input")
	p.AllowElements("details", "summary", "kbd", "mark", "ins", "del", "sub", "sup", "picture", "source")
	p.AllowAttrs("open").OnElements("details")
	p.AllowAttrs("srcset", "media", "type").OnElements("source")
	p.AllowDataURIImages()
	return p
}

// codeRenderer fa disegnare a mermaid i blocchi ```mermaid e passa tutti gli
// altri blocchi di codice all'evidenziatore di chroma.
type codeRenderer struct {
	highlight renderer.NodeRendererFunc
}

type captureFenced struct{ fn renderer.NodeRendererFunc }

func (c *captureFenced) Register(kind ast.NodeKind, fn renderer.NodeRendererFunc) {
	if kind == ast.KindFencedCodeBlock {
		c.fn = fn
	}
}

func newCodeRenderer() *codeRenderer {
	hl := highlighting.NewHTMLRenderer(
		highlighting.WithGuessLanguage(false),
		highlighting.WithFormatOptions(chromahtml.WithClasses(true)),
	)
	c := &captureFenced{}
	hl.RegisterFuncs(c)
	return &codeRenderer{highlight: c.fn}
}

func (r *codeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindFencedCodeBlock, r.render)
}

func (r *codeRenderer) render(w util.BufWriter, src []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.FencedCodeBlock)
	if !strings.EqualFold(string(n.Language(src)), "mermaid") {
		return r.highlight(w, src, node, entering)
	}
	if entering {
		_, _ = w.WriteString(`<div class="mermaid">`)
		lines := n.Lines()
		for i := 0; i < lines.Len(); i++ {
			seg := lines.At(i)
			_, _ = w.Write(util.EscapeHTML(seg.Value(src)))
		}
		_, _ = w.WriteString("</div>\n")
	}
	return ast.WalkSkipChildren, nil
}

// slugIDs genera gli id dei titoli come GitHub, accenti compresi, così
// gli indici scritti a mano ([Perché](#perché)) funzionano.
type slugIDs struct{ used map[string]bool }

func (s *slugIDs) Generate(value []byte, _ ast.NodeKind) []byte {
	base := slugify(string(value))
	if base == "" {
		base = "sezione"
	}
	id := base
	for i := 1; s.used[id]; i++ {
		id = fmt.Sprintf("%s-%d", base, i)
	}
	s.used[id] = true
	return []byte(id)
}

func (s *slugIDs) Put(value []byte) { s.used[string(value)] = true }

func slugify(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case unicode.IsLetter(r), unicode.IsNumber(r), unicode.Is(unicode.M, r), r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('-')
		}
	}
	return b.String()
}

// RenderFile legge un file e lo restituisce come HTML ripulito.
func RenderFile(path string) (*Doc, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Doc{Path: path, Title: filepath.Base(path), Missing: true}, nil
		}
		return nil, err
	}
	doc := RenderBytes(data, filepath.Dir(path))
	doc.Path = path
	if doc.Title == "" {
		doc.Title = filepath.Base(path)
	}
	return doc, nil
}

func RenderBytes(data []byte, baseDir string) *Doc {
	src := stripFrontMatter(decodeText(data))
	var buf bytes.Buffer
	ctx := parser.NewContext(parser.WithIDs(&slugIDs{used: map[string]bool{}}))
	if err := markdown.Convert(src, &buf, parser.WithContext(ctx)); err != nil {
		return &Doc{HTML: "<pre>" + html.EscapeString(string(src)) + "</pre>"}
	}
	clean := policy.SanitizeBytes(buf.Bytes())
	return postProcess(clean, baseDir)
}

// RenderNote impagina il testo di un appunto; link e immagini relativi
// valgono rispetto alla cartella del documento.
func RenderNote(text, baseDir string) string {
	var buf bytes.Buffer
	if err := noteMarkdown.Convert([]byte(text), &buf); err != nil {
		return "<p>" + html.EscapeString(text) + "</p>"
	}
	return postProcess(policy.SanitizeBytes(buf.Bytes()), baseDir).HTML
}

// decodeText porta il testo in UTF-8: toglie il BOM, legge l'UTF-16 che
// esce dalle redirezioni di PowerShell e ripiega su Windows-1252 per i
// vecchi file che UTF-8 non sono.
func decodeText(b []byte) []byte {
	if bytes.HasPrefix(b, []byte{0xFF, 0xFE}) || bytes.HasPrefix(b, []byte{0xFE, 0xFF}) {
		dec := xunicode.UTF16(xunicode.LittleEndian, xunicode.ExpectBOM).NewDecoder()
		if out, err := dec.Bytes(b); err == nil {
			return out
		}
	}
	b = bytes.TrimPrefix(b, []byte("\xEF\xBB\xBF"))
	if utf8.Valid(b) {
		return b
	}
	if out, err := charmap.Windows1252.NewDecoder().Bytes(b); err == nil {
		return out
	}
	return b
}

// stripFrontMatter nasconde l'intestazione YAML (--- ... ---) come fa VS Code.
// Serve un "chiave:" sulla prima riga, altrimenti un --- iniziale resta una riga orizzontale.
func stripFrontMatter(src []byte) []byte {
	lines := bytes.SplitAfter(src, []byte("\n"))
	if len(lines) < 3 || strings.TrimRight(string(lines[0]), "\r\n ") != "---" {
		return src
	}
	if first := strings.TrimSpace(string(lines[1])); !strings.Contains(first, ":") && !strings.HasPrefix(first, "#") {
		return src
	}
	off := len(lines[0])
	for _, l := range lines[1:] {
		off += len(l)
		if t := strings.TrimRight(string(l), "\r\n "); t == "---" || t == "..." {
			return src[off:]
		}
	}
	return src
}

var alertRe = regexp.MustCompile(`(?i)^\s*\[!(note|tip|important|warning|caution)\]\s*`)

var alertTitles = map[string]string{
	"note":      "Nota",
	"tip":       "Suggerimento",
	"important": "Importante",
	"warning":   "Attenzione",
	"caution":   "Cautela",
}

// postProcess sistema l'HTML già ripulito: immagini locali, tabelle che
// scorrono, liste di attività e i riquadri > [!NOTE] di GitHub.
func postProcess(clean []byte, baseDir string) *Doc {
	doc := &Doc{}
	nodes, err := html.ParseFragment(bytes.NewReader(clean), &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div})
	if err != nil {
		doc.HTML = string(clean)
		return doc
	}
	root := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	for _, n := range nodes {
		root.AppendChild(n)
	}

	var tables, alerts []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.DataAtom {
			case atom.Img:
				rewriteLocal(n, "src", baseDir)
			case atom.Source:
				rewriteLocal(n, "srcset", baseDir)
			case atom.Table:
				tables = append(tables, n)
			case atom.Blockquote:
				alerts = append(alerts, n)
			case atom.Li:
				if isTaskItem(n) {
					addClass(n, "task")
					if n.Parent != nil {
						addClass(n.Parent, "tasks")
					}
				}
			case atom.H1:
				if doc.Title == "" {
					doc.Title = strings.TrimSpace(textOf(n))
				}
			case atom.Div:
				if hasClass(n, "mermaid") {
					doc.Mermaid = true
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)

	for _, t := range tables {
		wrap := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div,
			Attr: []html.Attribute{{Key: "class", Val: "table-wrap"}}}
		t.Parent.InsertBefore(wrap, t)
		t.Parent.RemoveChild(t)
		wrap.AppendChild(t)
	}
	for _, q := range alerts {
		makeAlert(q)
	}

	var out bytes.Buffer
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		_ = html.Render(&out, c)
	}
	doc.HTML = out.String()
	return doc
}

func makeAlert(q *html.Node) {
	p := firstElement(q)
	if p == nil || p.DataAtom != atom.P || p.FirstChild == nil || p.FirstChild.Type != html.TextNode {
		return
	}
	t := p.FirstChild
	m := alertRe.FindStringSubmatch(t.Data)
	if m == nil {
		return
	}
	kind := strings.ToLower(m[1])
	t.Data = t.Data[len(m[0]):]
	if t.Data == "" {
		p.RemoveChild(t)
		if br := p.FirstChild; br != nil && br.DataAtom == atom.Br {
			p.RemoveChild(br)
		}
	}
	if p.FirstChild == nil {
		q.RemoveChild(p)
	}
	q.Data, q.DataAtom = "div", atom.Div
	q.Attr = []html.Attribute{{Key: "class", Val: "alert alert-" + kind}}
	title := &html.Node{Type: html.ElementNode, Data: "p", DataAtom: atom.P,
		Attr: []html.Attribute{{Key: "class", Val: "alert-title"}}}
	title.AppendChild(&html.Node{Type: html.TextNode, Data: alertTitles[kind]})
	q.InsertBefore(title, q.FirstChild)
}

func isTaskItem(li *html.Node) bool {
	c := firstElement(li)
	if c != nil && c.DataAtom == atom.P {
		c = firstElement(c)
	}
	return c != nil && c.DataAtom == atom.Input && attr(c, "type") == "checkbox"
}

// rewriteLocal fa passare le immagini su disco dal gestore /__file.
func rewriteLocal(n *html.Node, key, baseDir string) {
	for i, a := range n.Attr {
		if a.Key != key {
			continue
		}
		if p, ok := localPath(baseDir, a.Val); ok {
			n.Attr[i].Val = "/__file?p=" + url.QueryEscape(p)
		}
	}
}

var winAbs = regexp.MustCompile(`^[A-Za-z]:[\\/]`)

// localPath dice se un riferimento punta a un file su disco e quale.
// I percorsi che iniziano con / che non esistono si cercano dalla radice
// del repository, come su GitHub.
func localPath(baseDir, ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.HasPrefix(ref, "#") || strings.HasPrefix(ref, "//") {
		return "", false
	}
	if winAbs.MatchString(ref) || strings.HasPrefix(ref, `\\`) {
		return filepath.Clean(ref), true
	}
	u, err := url.Parse(ref)
	if err != nil {
		return "", false
	}
	switch strings.ToLower(u.Scheme) {
	case "":
	case "file":
		p := u.Path
		if winAbs.MatchString(strings.TrimPrefix(p, "/")) {
			p = strings.TrimPrefix(p, "/")
		} else if u.Host != "" && u.Host != "localhost" {
			p = "//" + u.Host + p
		}
		return filepath.Clean(filepath.FromSlash(p)), true
	default:
		return "", false
	}
	p, err := url.PathUnescape(u.Path)
	if err != nil {
		p = u.Path
	}
	if p == "" {
		return "", false
	}
	p = filepath.FromSlash(p)
	if strings.HasPrefix(p, string(filepath.Separator)) || filepath.IsAbs(p) {
		if _, err := os.Stat(p); err != nil {
			if repo := repoRoot(baseDir); repo != "" {
				return filepath.Join(repo, p), true
			}
		}
		return filepath.Clean(p), true
	}
	return filepath.Join(baseDir, p), true
}

func repoRoot(dir string) string {
	for d := dir; ; {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
}

func firstElement(n *html.Node) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode {
			return c
		}
		if c.Type == html.TextNode && strings.TrimSpace(c.Data) != "" {
			return nil
		}
	}
	return nil
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasClass(n *html.Node, class string) bool {
	for _, c := range strings.Fields(attr(n, "class")) {
		if c == class {
			return true
		}
	}
	return false
}

func addClass(n *html.Node, class string) {
	for i, a := range n.Attr {
		if a.Key == "class" {
			if !hasClass(n, class) {
				n.Attr[i].Val = strings.TrimSpace(a.Val + " " + class)
			}
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: "class", Val: class})
}

func textOf(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(textOf(c))
	}
	return b.String()
}

// chromaCSS genera i colori del codice per il tema chiaro e per lo scuro.
func chromaCSS() []byte {
	var out bytes.Buffer
	f := chromahtml.New(chromahtml.WithClasses(true))
	for _, t := range []struct{ scope, style string }{
		{`[data-theme="light"]`, "github"},
		{`[data-theme="dark"]`, "github-dark"},
	} {
		var css bytes.Buffer
		_ = f.WriteCSS(&css, styles.Get(t.style))
		for _, line := range strings.Split(css.String(), "\n") {
			if i := strings.Index(line, "*/"); i >= 0 {
				line = line[i+2:]
			}
			brace := strings.Index(line, "{")
			if brace < 0 {
				continue
			}
			sels := strings.Split(line[:brace], ",")
			for i, s := range sels {
				sels[i] = t.scope + " " + strings.TrimSpace(s)
			}
			out.WriteString(strings.Join(sels, ", ") + " " + line[brace:] + "\n")
		}
	}
	return out.Bytes()
}
