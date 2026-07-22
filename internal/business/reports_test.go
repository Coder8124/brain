package business

import (
	"strings"
	"testing"

	"github.com/pragun/brain/internal/sheet"
)

func TestDeckMarkdownRendersSlides(t *testing.T) {
	d := Deck{Title: "Q3 Review", Slides: []Slide{
		{Title: "Revenue", Bullets: []string{"Up 57%", "Record quarter"}},
		{Title: "Next steps", Bullets: []string{"Hire two reps"}},
	}}
	md := d.Markdown()
	for _, want := range []string{"# Q3 Review", "## Revenue", "- Up 57%", "## Next steps"} {
		if !strings.Contains(md, want) {
			t.Errorf("deck markdown missing %q:\n%s", want, md)
		}
	}
	if strings.Count(md, "\n---\n") < 2 {
		t.Error("slides should be separated by --- markers")
	}
}

func TestColumnGuessResolvesSynonyms(t *testing.T) {
	tbl := sheet.Table{Headers: []string{"vendor", "Cost (USD)"}}
	if i := columnGuess(tbl, "", []string{"amount", "cost", "total"}); i != 1 {
		t.Errorf("columnGuess should match 'Cost (USD)' via 'cost', got %d", i)
	}
	if i := columnGuess(tbl, "vendor", nil); i != 0 {
		t.Errorf("explicit name should win, got %d", i)
	}
	if i := columnGuess(tbl, "", []string{"nonexistent"}); i != -1 {
		t.Errorf("no match should be -1, got %d", i)
	}
}

func TestSlugifyIsFilesystemSafe(t *testing.T) {
	if got := slugify("Q3 Board Deck!"); got != "q3-board-deck" {
		t.Errorf("slugify = %q", got)
	}
	if got := slugify("!!!"); got != "untitled" {
		t.Errorf("empty slug should fall back, got %q", got)
	}
}
