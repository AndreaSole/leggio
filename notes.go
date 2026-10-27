package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Note è un appunto dell'utente su un documento: sul file intero (Section
// vuota) o su una sezione, cioè un titolo. Gli appunti stanno nella cartella
// dati di MdView, mai nel file markdown.
type Note struct {
	ID      string    `json:"id"`
	Section string    `json:"section"`           // id del titolo, "" per la nota sul file
	Heading string    `json:"heading,omitempty"` // testo del titolo, per ritrovarlo se l'id cambia
	Text    string    `json:"text"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
	HTML    string    `json:"html,omitempty"` // solo verso la vista, non si salva
}

type notesDisk struct {
	Version int                   `json:"version"`
	Files   map[string]*notesFile `json:"files"` // chiave: pathKey del documento
}

type notesFile struct {
	Path  string  `json:"path"`
	Notes []*Note `json:"notes"`
}

// NoteStore tiene gli appunti in un solo file JSON.
type NoteStore struct {
	mu   sync.Mutex
	path string
	data notesDisk
}

func notesPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "MdView", "appunti.json")
}

func OpenNotes(path string) *NoteStore {
	s := &NoteStore{path: path, data: notesDisk{Version: 1, Files: map[string]*notesFile{}}}
	if raw, err := os.ReadFile(path); err == nil {
		var d notesDisk
		if json.Unmarshal(raw, &d) == nil && d.Files != nil {
			s.data = d
		}
	}
	return s
}

// For restituisce una copia degli appunti di un documento: prima la nota
// sul file, poi quelle delle sezioni nell'ordine in cui sono state scritte.
func (s *NoteStore) For(doc string) []*Note {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.data.Files[pathKey(doc)]
	if f == nil {
		return nil
	}
	out := make([]*Note, 0, len(f.Notes))
	for _, n := range f.Notes {
		c := *n
		out = append(out, &c)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Section == "" && out[j].Section != "" })
	return out
}

// Set scrive l'appunto di una sezione (o del file se section è vuota); con
// il testo vuoto lo cancella. Restituisce l'appunto salvato, o nil se cancellato.
func (s *NoteStore) Set(doc, section, heading, text string) (*Note, error) {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	s.mu.Lock()
	defer s.mu.Unlock()
	key := pathKey(doc)
	f := s.data.Files[key]
	if f == nil {
		f = &notesFile{Path: doc}
	}
	idx := -1
	for i, n := range f.Notes {
		if n.Section == section {
			idx = i
			break
		}
	}
	now := time.Now().Truncate(time.Second)
	var saved *Note
	switch {
	case text == "" && idx < 0:
		return nil, nil
	case text == "":
		f.Notes = append(f.Notes[:idx], f.Notes[idx+1:]...)
	case idx >= 0:
		n := f.Notes[idx]
		n.Text, n.Updated = text, now
		if heading != "" {
			n.Heading = heading
		}
		saved = n
	default:
		saved = &Note{ID: newID(), Section: section, Heading: heading, Text: text, Created: now, Updated: now}
		f.Notes = append(f.Notes, saved)
	}
	if len(f.Notes) == 0 {
		delete(s.data.Files, key)
	} else {
		s.data.Files[key] = f
	}
	if err := s.save(); err != nil {
		return nil, err
	}
	if saved == nil {
		return nil, nil
	}
	c := *saved
	return &c, nil
}

// Noted elenca i documenti che hanno almeno un appunto.
func (s *NoteStore) Noted() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.data.Files))
	for _, f := range s.data.Files {
		out = append(out, f.Path)
	}
	sort.Strings(out)
	return out
}

func (s *NoteStore) save() error {
	if s.path == "" {
		return nil
	}
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
