package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNotesSaveUpdateDelete(t *testing.T) {
	store := filepath.Join(t.TempDir(), "MdView", "appunti.json")
	doc := filepath.Join(t.TempDir(), "guida.md")
	s := OpenNotes(store)

	if _, err := s.Set(doc, "", "", "Da rileggere"); err != nil {
		t.Fatal(err)
	}
	n, err := s.Set(doc, "installazione", "Installazione", "Su Windows\r\nserve WebView2  ")
	if err != nil || n == nil || n.Text != "Su Windows\nserve WebView2" {
		t.Fatalf("appunto di sezione: %+v, %v", n, err)
	}
	first := n.ID

	// Si riapre dal disco: tutto è ancora lì, nota sul file per prima.
	s = OpenNotes(store)
	notes := s.For(doc)
	if len(notes) != 2 || notes[0].Section != "" || notes[1].Heading != "Installazione" {
		t.Fatalf("dopo la riapertura: %+v", notes)
	}

	// Riscrivere la stessa sezione aggiorna, non duplica.
	n, _ = s.Set(doc, "installazione", "Installazione", "Testo nuovo")
	if n.ID != first || len(s.For(doc)) != 2 {
		t.Fatalf("aggiornamento: id %s, %d appunti", n.ID, len(s.For(doc)))
	}

	// Il testo vuoto cancella; senza appunti il documento sparisce dall'elenco.
	if n, _ := s.Set(doc, "installazione", "", "  "); n != nil {
		t.Fatal("il testo vuoto doveva cancellare")
	}
	if got := s.Noted(); len(got) != 1 || got[0] != doc {
		t.Fatalf("Noted = %v", got)
	}
	_, _ = s.Set(doc, "", "", "")
	if got := s.Noted(); len(got) != 0 {
		t.Fatalf("Noted dopo aver cancellato tutto = %v", got)
	}
	raw, _ := os.ReadFile(store)
	if strings.Contains(string(raw), "guida.md") {
		t.Errorf("il documento senza appunti è rimasto nel file: %s", raw)
	}
}

func TestNoteRenderHasNoHeadingIDs(t *testing.T) {
	html := RenderNote("## Da fare\n\n- [ ] controllare ![x](img.png)", "/docs")
	mustNotContain(t, html, `id="`)
	mustContain(t, html, "<h2>Da fare</h2>", `type="checkbox"`, "/__file?p=")
	mustContain(t, RenderNote("riga uno\nriga due", "/docs"), "riga uno<br/>")
}
