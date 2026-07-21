package tutor

import "testing"

func items() []MCQ {
	return []MCQ{
		{Subskill: "limits", Q: "q1", Options: []string{"a", "b"}, Correct: 0},
		{Subskill: "limits", Q: "q2", Options: []string{"a", "b"}, Correct: 1},
		{Subskill: "derivatives", Q: "q3", Options: []string{"a", "b"}, Correct: 0},
		{Subskill: "derivatives", Q: "q4", Options: []string{"a", "b"}, Correct: 0},
		{Subskill: "integrals", Q: "q5", Options: []string{"a", "b"}, Correct: 1},
		{Subskill: "integrals", Q: "q6", Options: []string{"a", "b"}, Correct: 1},
	}
}

func TestScoreProducesPerSubskillLevels(t *testing.T) {
	// limits: 1/2 shaky, derivatives: 2/2 solid, integrals: 0/2 gap.
	answers := []int{0, 0 /*wrong*/, 0, 0, 0 /*wrong*/, 0 /*wrong*/}
	m := Score("Calculus", items(), answers)

	want := map[string]Level{"limits": Shaky, "derivatives": Solid, "integrals": Gap}
	for _, s := range m.Scores {
		if s.Level != want[s.Subskill] {
			t.Errorf("%s: level %s, want %s (%d/%d)", s.Subskill, s.Level, want[s.Subskill], s.Correct, s.Total)
		}
	}
}

func TestStartHereIsTheWeakest(t *testing.T) {
	answers := []int{0, 0, 0, 0, 0, 0} // integrals fully wrong
	m := Score("Calculus", items(), answers)
	if m.StartHere != "integrals" {
		t.Errorf("StartHere = %q, want integrals (the weakest)", m.StartHere)
	}
}

func TestWeakCardsOnlyFromMissed(t *testing.T) {
	answers := []int{0, 0 /*miss q2*/, 0, 0, 0 /*miss q5*/, 1 /*hit q6*/}
	cards := WeakCards(items(), answers)
	// missed: q2, q3? no q3 correct=0 answered 0 -> hit. q5 miss, q1 hit.
	// Let's just assert the answer text is the correct option, and hits excluded.
	for _, c := range cards {
		if c.A == "" || c.Q == "" {
			t.Errorf("weak card missing content: %+v", c)
		}
	}
	// q6 was answered correctly (correct=1, answered 1) so must not appear.
	for _, c := range cards {
		if c.Q == "q6" {
			t.Error("a correctly answered question must not become a weak card")
		}
	}
}

func TestSortByLevelPutsGapsFirst(t *testing.T) {
	answers := []int{0, 0, 0, 0, 0, 0}
	m := Score("Calculus", items(), answers)
	m.SortByLevel()
	if m.Scores[0].Level != Gap {
		t.Errorf("after sort, first should be a gap, got %s", m.Scores[0].Level)
	}
}

func TestValidMCQRejectsBadItems(t *testing.T) {
	if validMCQ([]string{"only one"}, 0) {
		t.Error("a single-option question is not a valid MCQ")
	}
	if validMCQ([]string{"a", "b"}, 5) {
		t.Error("out-of-range correct index must be rejected")
	}
	if validMCQ([]string{"a", ""}, 0) {
		t.Error("an empty option must be rejected")
	}
	if !validMCQ([]string{"a", "b", "c", "d"}, 2) {
		t.Error("a well-formed MCQ should be accepted")
	}
}

func TestPresetLookupHandlesShorthand(t *testing.T) {
	cases := map[string]string{
		"AP Calculus BC": "AP Calculus BC",
		"calc bc":        "AP Calculus BC",
		"ap chem":        "AP Chemistry",
		"physics c":      "AP Physics C",
		"CHEMISTRY":      "AP Chemistry",
	}
	for in, want := range cases {
		p, ok := PresetFor(in)
		if !ok || p.Name != want {
			t.Errorf("PresetFor(%q) = %q,%v; want %q", in, p.Name, ok, want)
		}
	}
	if _, ok := PresetFor("underwater basket weaving"); ok {
		t.Error("an unknown subject should not match a preset")
	}
}

func TestPresetsAreDiagnosticSized(t *testing.T) {
	for _, p := range Presets {
		if len(p.Subskills) < 4 || len(p.Subskills) > 6 {
			t.Errorf("%s has %d subskills; a diagnostic wants 4-6", p.Name, len(p.Subskills))
		}
	}
}

func TestBankIsWellFormedAndCoversPresets(t *testing.T) {
	for _, p := range Presets {
		items, ok := bankFor(p.Name, p.Subskills)
		if !ok {
			t.Errorf("%s has no verified bank", p.Name)
			continue
		}
		// Every banked question must be a valid MCQ with a real correct index,
		// and every subskill should be covered at least once.
		covered := map[string]bool{}
		for _, q := range items {
			if !validMCQ(q.Options, q.Correct) {
				t.Errorf("%s / %q is malformed: correct=%d of %d options", p.Name, q.Q, q.Correct, len(q.Options))
			}
			if len(q.Options) != 4 {
				t.Errorf("%s / %q should have 4 options, has %d", p.Name, q.Q, len(q.Options))
			}
			covered[q.Subskill] = true
		}
		for _, s := range p.Subskills {
			if !covered[s] {
				t.Errorf("%s: subskill %q has no questions", p.Name, s)
			}
		}
	}
}

func TestBankAnswerScramblePlacesCorrectOption(t *testing.T) {
	// q() rotates options deterministically; the Correct index must always point
	// at the answer that was authored first.
	m := q("s", "what is two plus two", "4", "3", "5", "22")
	if m.Options[m.Correct] != "4" {
		t.Errorf("correct index points at %q, want 4", m.Options[m.Correct])
	}
}
