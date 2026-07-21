package tutor

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pragun/brain/internal/router"
)

// Screen notes: when the screen is captured in tutor mode, decide whether what
// is on it is worth studying, and if so distil it into a note.
//
// The gate matters more than the extraction. Screen capture sees everything —
// bank balances, private messages, a video paused on a frame. The studious
// filter is what keeps the vault full of what you were learning and empty of
// what you were doing, and it runs before any text is sent to a model or
// written anywhere.

// studiousCues are words and shapes that mark a screen as learning material.
// Deliberately broad on the signal side; precision comes from requiring several
// to co-occur, below.
var studiousCues = []string{
	"theorem", "definition", "chapter", "lecture", "problem set", "exercise",
	"proof", "equation", "formula", "hypothesis", "vocabulary", "syllabus",
	"assignment", "textbook", "study", "quiz", "flashcard", "derivation",
	"example", "solution", "figure", "abstract", "citation", "footnote",
}

// nonStudiousDomains are contexts we never want to note even if they look
// text-heavy — messaging, mail, finance. A screen dominated by these is
// excluded regardless of cue count.
var nonStudiousDomains = []string{
	"inbox", "compose", "checkout", "cart", "balance", "password",
	"direct message", "dm", "chat", "feed", "timeline", "notification",
}

// LooksStudious decides whether captured screen text is study material.
//
// Requires either an explicit strong signal (a heading like "Chapter 4") or
// several weaker cues together, and vetoes on any non-studious domain marker.
// Tuned for precision: a tutor that notes your bank statement loses trust
// instantly, so a missed page is far cheaper than a wrong one.
func LooksStudious(text string) bool {
	low := strings.ToLower(text)

	for _, bad := range nonStudiousDomains {
		if strings.Contains(low, bad) {
			return false
		}
	}

	// Too little text is almost never a page of study material.
	if len(strings.Fields(low)) < 40 {
		return false
	}

	hits := 0
	for _, cue := range studiousCues {
		if strings.Contains(low, cue) {
			hits++
		}
	}
	return hits >= 3
}

var noteSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"topic": map[string]any{"type": "string"},
		"notes": map[string]any{
			"type":  "array",
			"items": map[string]any{"type": "string"},
		},
	},
	"required":             []string{"topic", "notes"},
	"additionalProperties": false,
}

// ScreenNote is what the tutor distils from a studious screen: a topic and a
// few durable points, ready to become a vault note through the review queue.
type ScreenNote struct {
	Topic string
	Notes []string
}

// Distil turns OCR'd screen text into study notes. Runs only after
// LooksStudious has passed, so the model never sees a rejected screen.
//
// The prompt forbids copying the page verbatim: the value of a study note is
// the compression, and a transcript of the screen is just the screen again.
func Distil(rt *router.Router, ocrText string) (*ScreenNote, error) {
	model, err := rt.ModelFor(router.T1, true)
	if err != nil {
		return nil, err
	}

	const system = "You are a study assistant reading text captured from a screen. " +
		"Reply with JSON only. Give the topic in a few words, and 2-5 durable notes a student " +
		"would want later — concepts, definitions, relationships. Paraphrase tightly; do not copy " +
		"sentences verbatim. If the text is not actually educational, return an empty notes list."

	out, err := rt.Local().Chat(model, system, truncate(ocrText, 4000), noteSchema)
	if err != nil {
		return nil, err
	}

	var res struct {
		Topic string   `json:"topic"`
		Notes []string `json:"notes"`
	}
	if err := json.Unmarshal([]byte(cleanJSON(out)), &res); err != nil {
		return nil, fmt.Errorf("screen distil returned unparseable JSON: %w", err)
	}

	var kept []string
	for _, n := range res.Notes {
		if s := strings.TrimSpace(n); len(s) > 8 {
			kept = append(kept, s)
		}
	}
	if len(kept) == 0 {
		return nil, nil // the model agreed it was not worth noting
	}
	return &ScreenNote{Topic: strings.TrimSpace(res.Topic), Notes: kept}, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
