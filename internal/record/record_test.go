package record

import (
	"testing"
)

func TestDedupCollapsesRepeatedFrames(t *testing.T) {
	s := &Session{Frames: []Frame{
		{TS: 0, Text: "Chapter 4: Eigenvalues and the characteristic polynomial of a matrix"},
		{TS: 4, Text: "Chapter 4: Eigenvalues and the characteristic polynomial of a matrix"},
		{TS: 8, Text: "Chapter 4: Eigenvalues and the characteristic polynomial of a matrix"},
		{TS: 12, Text: "Chapter 5: Diagonalization and change of basis for linear maps"},
	}}
	out := s.DedupedText()
	if n := countOccurrences(out, "Chapter 4"); n != 1 {
		t.Errorf("repeated identical frames should collapse to one, got %d", n)
	}
	if countOccurrences(out, "Chapter 5") != 1 {
		t.Error("the distinct frame should be kept")
	}
}

func TestEmptySession(t *testing.T) {
	if !(&Session{}).Empty() {
		t.Error("a session with no frames is empty")
	}
	if (&Session{Frames: []Frame{{Text: "hello world content here"}}}).Empty() {
		t.Error("a session with content is not empty")
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Eigenvalues & Eigenvectors": "eigenvalues-eigenvectors",
		"  AP Calc BC: Limits  ":     "ap-calc-bc-limits",
		"!!!":                        "", // becomes a timestamp fallback, non-empty
	}
	for in, want := range cases {
		got := slugify(in)
		if want == "" {
			if got == "" {
				t.Errorf("slugify(%q) must fall back to a non-empty slug", in)
			}
			continue
		}
		if got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func countOccurrences(s, sub string) int {
	n := 0
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			n++
		}
	}
	return n
}
