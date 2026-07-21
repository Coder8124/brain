package tutor

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/pragun/brain/internal/router"
)

// Diagnostic placement, in the spirit of Khan Academy's start-of-course quiz:
// figure out what a student already knows across a subject's subskills, then
// point them at where to begin — and seed the spaced-repetition deck with their
// gaps, which is the loop a cloud placement quiz cannot close.
//
// Questions are multiple-choice on purpose. A diagnostic has to be objectively
// scored — the mastery map is only trustworthy if "correct" is a fact, not a
// model's judgement of a free-text answer.

// MCQ is one multiple-choice diagnostic question.
type MCQ struct {
	Subskill   string   `json:"subskill"`
	Q          string   `json:"q"`
	Options    []string `json:"options"`
	Correct    int      `json:"correct"` // index into Options
	Difficulty string   `json:"difficulty"`
}

var subskillSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"subskills": map[string]any{
			"type":  "array",
			"items": map[string]any{"type": "string"},
		},
	},
	"required":             []string{"subskills"},
	"additionalProperties": false,
}

// Preset is a built-in subject with a curated subskill breakdown, so the common
// starting points need no model call and always cover the real curriculum.
type Preset struct {
	Name      string
	Subskills []string
}

// Presets ship the subjects a student is most likely to place into first. The
// subskill lists follow the College Board course units, trimmed to a
// diagnostic-sized span.
var Presets = []Preset{
	{
		Name: "AP Calculus BC",
		Subskills: []string{
			"limits and continuity",
			"differentiation and its rules",
			"applications of derivatives",
			"integration and accumulation",
			"differential equations",
			"infinite sequences and series",
		},
	},
	{
		Name: "AP Physics C",
		Subskills: []string{
			"kinematics",
			"Newton's laws and forces",
			"work, energy, and momentum",
			"rotational motion",
			"electrostatics and circuits",
			"magnetism and induction",
		},
	},
	{
		Name: "AP Chemistry",
		Subskills: []string{
			"atomic structure and periodicity",
			"bonding and molecular geometry",
			"intermolecular forces",
			"stoichiometry and reactions",
			"kinetics",
			"thermodynamics and equilibrium",
		},
	},
}

// PresetFor returns a built-in subject matching name (case-insensitive, tolerant
// of "calc bc" / "ap chem" style shorthand), if any.
func PresetFor(name string) (Preset, bool) {
	n := strings.ToLower(strings.TrimSpace(name))
	for _, p := range Presets {
		pn := strings.ToLower(p.Name)
		if n == pn || strings.Contains(pn, n) || matchesShorthand(n, pn) {
			return p, true
		}
	}
	return Preset{}, false
}

// matchesShorthand treats "chem"/"physics"/"calc" and "bc"/"c" as hints toward
// the corresponding preset, so a student need not type the exact title.
func matchesShorthand(n, preset string) bool {
	keys := map[string]string{"calc": "calculus", "chem": "chemistry", "physics": "physics"}
	for short, full := range keys {
		if strings.Contains(n, short) && strings.Contains(preset, full) {
			return true
		}
	}
	return false
}

// Subskills returns a subject's component skills — from a preset when one
// matches, otherwise asking the model. Kept small either way; a placement quiz
// that tests twenty things exhausts the student before it learns anything.
func Subskills(rt *router.Router, subject string) ([]string, error) {
	if p, ok := PresetFor(subject); ok {
		return p.Subskills, nil
	}

	model, err := rt.ModelFor(router.T2, true)
	if err != nil {
		return nil, err
	}

	system := "You are a curriculum designer. Break the subject into 4 to 6 core subskills a " +
		"placement quiz should cover, ordered from foundational to advanced. Reply with JSON only. " +
		"Each subskill is a short noun phrase (e.g. 'limits', 'the chain rule')."

	out, err := rt.Local().Chat(model, system, "Subject: "+subject, subskillSchema)
	if err != nil {
		return nil, err
	}
	var res struct {
		Subskills []string `json:"subskills"`
	}
	if err := json.Unmarshal([]byte(cleanJSON(out)), &res); err != nil {
		return nil, fmt.Errorf("subskill breakdown failed to parse: %w", err)
	}

	var kept []string
	for _, s := range res.Subskills {
		if s = strings.TrimSpace(s); s != "" {
			kept = append(kept, s)
		}
	}
	if len(kept) == 0 {
		return nil, fmt.Errorf("could not break %q into subskills", subject)
	}
	if len(kept) > 6 {
		kept = kept[:6]
	}
	return kept, nil
}

var mcqSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"questions": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"q":          map[string]any{"type": "string"},
					"options":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"correct":    map[string]any{"type": "integer"},
					"difficulty": map[string]any{"type": "string", "enum": []string{"easy", "medium", "hard"}},
				},
				"required":             []string{"q", "options", "correct", "difficulty"},
				"additionalProperties": false,
			},
		},
	},
	"required":             []string{"questions"},
	"additionalProperties": false,
}

// Diagnostic builds the placement quiz: a spanning set of MCQs across the
// subject's subskills at mixed difficulty.
//
// Generated per subskill (narrow calls beat one big prompt on small models),
// perPer questions each. Malformed items are dropped rather than trusted, so a
// bad option list never corrupts the score.
func Diagnostic(rt *router.Router, subject string, subskills []string, perSkill int) ([]MCQ, error) {
	// Prefer the verified bank for preset subjects: correct answers and instant,
	// where a local model would be slow and sometimes wrong.
	if items, ok := bankFor(subject, subskills); ok {
		return items, nil
	}

	// T1: MCQ generation is a narrow, well-scoped task, and a placement quiz
	// makes several calls — the smaller, faster model keeps the whole diagnostic
	// under a reasonable wait where the reasoning model would take minutes.
	// Correctness is best-effort here, which is why arbitrary subjects carry a
	// caveat the presets do not.
	model, err := rt.ModelFor(router.T1, true)
	if err != nil {
		return nil, err
	}

	var out []MCQ
	for _, skill := range subskills {
		system := fmt.Sprintf("You are writing a placement quiz for %q, testing the subskill %q. "+
			"Write %d multiple-choice questions spanning easy to hard. Reply with JSON only. "+
			"Each has exactly 4 options, exactly one correct, and correct is the 0-based index of "+
			"the right option. Make the wrong options plausible, not silly.", subject, skill, perSkill)

		res, err := rt.Local().Chat(model, system, "Subskill: "+skill, mcqSchema)
		if err != nil {
			continue // one weak subskill shouldn't sink the whole diagnostic
		}
		var parsed struct {
			Questions []struct {
				Q          string   `json:"q"`
				Options    []string `json:"options"`
				Correct    int      `json:"correct"`
				Difficulty string   `json:"difficulty"`
			} `json:"questions"`
		}
		if json.Unmarshal([]byte(cleanJSON(res)), &parsed) != nil {
			continue
		}
		for _, q := range parsed.Questions {
			if !validMCQ(q.Options, q.Correct) || strings.TrimSpace(q.Q) == "" {
				continue
			}
			out = append(out, MCQ{
				Subskill: skill, Q: strings.TrimSpace(q.Q),
				Options: q.Options, Correct: q.Correct, Difficulty: q.Difficulty,
			})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("could not generate diagnostic questions for %q", subject)
	}
	return out, nil
}

func validMCQ(options []string, correct int) bool {
	if len(options) < 2 || correct < 0 || correct >= len(options) {
		return false
	}
	for _, o := range options {
		if strings.TrimSpace(o) == "" {
			return false
		}
	}
	return true
}

// ---- scoring: pure arithmetic over the answers ----

// Level is a subskill's placement result.
type Level string

const (
	Gap   Level = "gap"   // <40% — start here
	Shaky Level = "shaky" // 40–80% — needs review
	Solid Level = "solid" // >=80% — already comfortable
)

type SubskillScore struct {
	Subskill       string
	Correct, Total int
	Level          Level
}

// MasteryMap is the diagnostic's output: where the student stands per subskill
// and where to begin.
type MasteryMap struct {
	Subject   string
	Scores    []SubskillScore
	StartHere string // the weakest subskill worth beginning with
}

func levelFor(correct, total int) Level {
	if total == 0 {
		return Shaky
	}
	switch r := float64(correct) / float64(total); {
	case r >= 0.8:
		return Solid
	case r >= 0.4:
		return Shaky
	default:
		return Gap
	}
}

// Score turns answers (index chosen per question, -1 for skipped) into a mastery
// map. answers must line up with items by position.
func Score(subject string, items []MCQ, answers []int) MasteryMap {
	type acc struct{ correct, total int }
	by := map[string]*acc{}
	order := []string{}

	for i, q := range items {
		a, ok := by[q.Subskill]
		if !ok {
			a = &acc{}
			by[q.Subskill] = a
			order = append(order, q.Subskill)
		}
		a.total++
		if i < len(answers) && answers[i] == q.Correct {
			a.correct++
		}
	}

	m := MasteryMap{Subject: subject}
	worstRatio := 2.0
	for _, skill := range order {
		a := by[skill]
		lvl := levelFor(a.correct, a.total)
		m.Scores = append(m.Scores, SubskillScore{Subskill: skill, Correct: a.correct, Total: a.total, Level: lvl})

		// "Start here" is the weakest subskill — the biggest lever — preferring
		// foundational ones (earlier in the ordered list) on ties.
		if r := float64(a.correct) / float64(a.total); r < worstRatio {
			worstRatio = r
			m.StartHere = skill
		}
	}
	return m
}

// WeakCards turns the questions a student missed into spaced-repetition cards,
// so the diagnostic feeds straight into daily practice. This is the loop a
// cloud placement quiz cannot close.
func WeakCards(items []MCQ, answers []int) []Card {
	var cards []Card
	for i, q := range items {
		if i < len(answers) && answers[i] == q.Correct {
			continue // got it right — no need to drill
		}
		cards = append(cards, Card{
			Q:      q.Q,
			A:      q.Options[q.Correct],
			Source: "diagnostic/" + slugSkill(q.Subskill),
		})
	}
	return cards
}

func slugSkill(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else if r == ' ' || r == '-' {
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// SortByLevel orders a mastery map gaps-first, for presenting "here's where to
// focus" without making the reader hunt.
func (m *MasteryMap) SortByLevel() {
	rank := map[Level]int{Gap: 0, Shaky: 1, Solid: 2}
	sort.SliceStable(m.Scores, func(i, j int) bool {
		return rank[m.Scores[i].Level] < rank[m.Scores[j].Level]
	})
}
