package dashboard

import (
	"encoding/json"
	"io"
)

// schemaVersion is the current NDJSON fragment contract version. Every
// fragment carries V=schemaVersion; the parser drops anything else.
const schemaVersion = 1

// Fragment is one NDJSON line a segment producer may emit. All concrete
// fragment types live in this package, so the interface methods stay
// unexported.
type Fragment interface {
	kind() string
	sec() string
}

// Section returns a fragment's declared section (may be empty).
func Section(f Fragment) string {
	return f.sec()
}

// Card is a single card rendered inside a Group fragment.
type Card struct {
	Title string   `json:"title"`
	Desc  string   `json:"desc,omitempty"`
	State string   `json:"state,omitempty"`
	Ref   string   `json:"ref,omitempty"`
	Chips []string `json:"chips,omitempty"`
}

// Tile is a compact label/value fragment.
type Tile struct {
	V       int    `json:"v"`
	Type    string `json:"type"`
	Section string `json:"section,omitempty"`
	Label   string `json:"label"`
	Value   string `json:"value"`
	Sub     string `json:"sub,omitempty"`
	Accent  string `json:"accent,omitempty"`
}

func (t Tile) kind() string { return t.Type }
func (t Tile) sec() string  { return t.Section }

// NewTile constructs a valid Tile fragment.
func NewTile(section, label, value string) Tile {
	return Tile{V: schemaVersion, Type: "tile", Section: section, Label: label, Value: value}
}

// Bar is a labeled numeric value, optionally bounded by Max.
type Bar struct {
	V       int      `json:"v"`
	Type    string   `json:"type"`
	Section string   `json:"section,omitempty"`
	Label   string   `json:"label"`
	Value   float64  `json:"value"`
	Max     *float64 `json:"max,omitempty"`
	Note    string   `json:"note,omitempty"`
}

func (b Bar) kind() string { return b.Type }
func (b Bar) sec() string  { return b.Section }

// Group is a titled collection of cards.
type Group struct {
	V       int    `json:"v"`
	Type    string `json:"type"`
	Section string `json:"section,omitempty"`
	Title   string `json:"title,omitempty"`
	Cards   []Card `json:"cards"`
}

func (g Group) kind() string { return g.Type }
func (g Group) sec() string  { return g.Section }

// Note is a free-text fragment.
type Note struct {
	V       int    `json:"v"`
	Type    string `json:"type"`
	Section string `json:"section,omitempty"`
	Text    string `json:"text"`
}

func (n Note) kind() string { return n.Type }
func (n Note) sec() string  { return n.Section }

// HTML is a raw HTML fragment (rendering deferred past Slice 1).
type HTML struct {
	V       int    `json:"v"`
	Type    string `json:"type"`
	Section string `json:"section,omitempty"`
	HTML    string `json:"html"`
}

func (h HTML) kind() string { return h.Type }
func (h HTML) sec() string  { return h.Section }

// Emit writes each fragment as one compact NDJSON line to w.
func Emit(w io.Writer, frags []Fragment) error {
	for _, f := range frags {
		b, err := json.Marshal(f)
		if err != nil {
			return err
		}
		if _, err := w.Write(b); err != nil {
			return err
		}
		if _, err := w.Write([]byte{'\n'}); err != nil {
			return err
		}
	}
	return nil
}
