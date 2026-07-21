package record

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pragun/brain/internal/router"
	"github.com/pragun/brain/internal/tutor"
)

// Processing a finished session into saved study material.
//
// The model does three jobs from the captured screen text: name the session,
// write durable notes, and pull out the terms worth remembering. The notes land
// as a normal vault note (type: source) so retrieval and the tutor can use them
// like anything else, and the raw video — if one was captured — is filed in a
// folder named for the session.

var recordSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"title": map[string]any{"type": "string"},
		"notes": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"terms": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"term":       map[string]any{"type": "string"},
					"definition": map[string]any{"type": "string"},
				},
				"required":             []string{"term", "definition"},
				"additionalProperties": false,
			},
		},
	},
	"required":             []string{"title", "notes", "terms"},
	"additionalProperties": false,
}

// Result reports what a processed recording produced.
type Result struct {
	Title     string
	NotePath  string
	VideoPath string
	Cards     int
	Folder    string
}

// Process turns a session into a titled note (and optional filed video), and
// seeds flashcards from the terms. If chosenName is non-empty it overrides the
// generated title — the user can always name it themselves.
func Process(rt *router.Router, db *sql.DB, vault string, s *Session, chosenName string) (Result, error) {
	if s == nil || s.Empty() {
		return Result{}, fmt.Errorf("nothing was captured — was a studious screen on view?")
	}

	model, err := rt.ModelFor(router.T2, true)
	if err != nil {
		return Result{}, err
	}

	const system = "You are a study assistant. From the text captured off a screen during a study " +
		"session, produce JSON with: a short descriptive title; 4-10 durable notes that explain the " +
		"material in your own words (concepts and how they connect, not a transcript); and the key " +
		"terms with concise definitions. Paraphrase; do not copy verbatim."

	out, err := rt.Local().Chat(model, system, truncate(s.DedupedText(), 8000), recordSchema)
	if err != nil {
		return Result{}, err
	}
	var parsed struct {
		Title string   `json:"title"`
		Notes []string `json:"notes"`
		Terms []struct {
			Term       string `json:"term"`
			Definition string `json:"definition"`
		} `json:"terms"`
	}
	if err := json.Unmarshal([]byte(cleanJSON(out)), &parsed); err != nil {
		return Result{}, fmt.Errorf("could not parse the session summary: %w", err)
	}

	title := strings.TrimSpace(chosenName)
	if title == "" {
		title = strings.TrimSpace(parsed.Title)
	}
	if title == "" {
		title = "Study session " + time.Unix(s.Started, 0).Format("2006-01-02 15:04")
	}
	slug := slugify(title)

	// A titled folder holds the session's artifacts; the note also lives at a
	// predictable sources/ path so the vault index picks it up.
	folder := filepath.Join(vault, "recordings", slug)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return Result{}, err
	}

	res := Result{Title: title, Folder: folder}

	// Move the captured video into the folder under the session's name.
	if s.VideoPath != "" {
		if _, err := os.Stat(s.VideoPath); err == nil {
			dst := filepath.Join(folder, slug+".mp4")
			if os.Rename(s.VideoPath, dst) == nil {
				res.VideoPath = dst
			}
		}
	}

	// The note: readable study material, as a source-type vault note.
	notePath := filepath.Join(vault, "sources", slug+".md")
	if err := os.MkdirAll(filepath.Dir(notePath), 0o755); err != nil {
		return res, err
	}
	res.NotePath = notePath
	if err := os.WriteFile(notePath, []byte(renderNote(title, parsed.Notes, parsed.Terms, s, res.VideoPath)), 0o644); err != nil {
		return res, err
	}

	// Seed flashcards from the terms, so a recording feeds straight into review.
	if err := tutor.InitDeck(db); err == nil {
		for _, t := range parsed.Terms {
			term, def := strings.TrimSpace(t.Term), strings.TrimSpace(t.Definition)
			if term == "" || def == "" {
				continue
			}
			if ok, _ := tutor.AddCard(db, tutor.Card{
				Q: "What is " + term + "?", A: def, Source: "sources/" + slug,
			}); ok {
				res.Cards++
			}
		}
	}

	return res, nil
}

func renderNote(title string, notes []string, terms []struct {
	Term       string `json:"term"`
	Definition string `json:"definition"`
}, s *Session, video string) string {
	var b strings.Builder
	b.WriteString("---\ntype: source\n")
	fmt.Fprintf(&b, "title: %s\n", title)
	fmt.Fprintf(&b, "captured: %s\n", time.Unix(s.Started, 0).Format("2006-01-02"))
	fmt.Fprintf(&b, "duration_min: %d\n", s.DurationMin())
	if video != "" {
		fmt.Fprintf(&b, "video: %s\n", filepath.Base(video))
	}
	b.WriteString("source: screen recording\n---\n\n")

	for _, n := range notes {
		if n = strings.TrimSpace(n); n != "" {
			fmt.Fprintf(&b, "- %s\n", n)
		}
	}
	if len(terms) > 0 {
		b.WriteString("\n## Key terms\n\n")
		for _, t := range terms {
			if strings.TrimSpace(t.Term) != "" {
				fmt.Fprintf(&b, "- **%s** — %s\n", strings.TrimSpace(t.Term), strings.TrimSpace(t.Definition))
			}
		}
	}
	return b.String()
}

func slugify(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteByte('-')
		}
	}
	out := strings.Trim(collapseDashes(b.String()), "-")
	if out == "" {
		out = fmt.Sprintf("session-%d", time.Now().Unix())
	}
	return out
}

func collapseDashes(s string) string {
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func cleanJSON(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = s[i+1:]
		}
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}
	return strings.TrimSpace(s)
}
