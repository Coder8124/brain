package memory

import (
	"database/sql"
	"math"
	"testing"

	_ "modernc.org/sqlite"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if err := Init(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// storeVec inserts a memory with a supplied embedding, bypassing the provider so
// recall ranking is testable deterministically.
func storeVec(t *testing.T, db *sql.DB, text string, kind Kind, salience float64, vec []float32) {
	t.Helper()
	_, err := db.Exec(
		`INSERT OR IGNORE INTO memories (text, kind, salience, source, created, vec, fingerprint)
		 VALUES (?,?,?,?,?,?,?)`,
		text, string(kind), salience, "test", 1, floatsToBlob(vec), fingerprint(text))
	if err != nil {
		t.Fatal(err)
	}
}

func TestRecallRanksBySimilarity(t *testing.T) {
	db := testDB(t)
	storeVec(t, db, "prefers morning meetings", Preference, 0.5, []float32{1, 0, 0})
	storeVec(t, db, "launching in Q4", Context, 0.5, []float32{0, 1, 0})
	storeVec(t, db, "Sarah is the CFO", Person, 0.5, []float32{0, 0, 1})

	// A query pointing along the "meetings" axis should surface that memory.
	got, err := recallByVec(db, []float32{0.9, 0.1, 0}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || got[0].Text != "prefers morning meetings" {
		t.Errorf("recall should rank the closest memory first, got %+v", got)
	}
}

func TestSalienceBreaksNearTies(t *testing.T) {
	db := testDB(t)
	// Two memories almost equally close; the more salient should win.
	storeVec(t, db, "trivial", Fact, 0.1, []float32{1, 0.02, 0})
	storeVec(t, db, "important", Fact, 0.95, []float32{1, 0.0, 0})
	got, _ := recallByVec(db, []float32{1, 0, 0}, 2)
	if got[0].Text != "important" {
		t.Errorf("salience should break a near tie toward the important memory, got %q", got[0].Text)
	}
}

func TestDedupOnNormalisedText(t *testing.T) {
	db := testDB(t)
	storeVec(t, db, "Prefers  short   emails", Preference, 0.5, []float32{1, 0, 0})
	storeVec(t, db, "prefers short emails", Preference, 0.5, []float32{1, 0, 0})
	if n, _ := Count(db); n != 1 {
		t.Errorf("the same fact in different whitespace/case should store once, got %d", n)
	}
}

func TestContextRendersRecalled(t *testing.T) {
	mems := []Memory{
		{Text: "prefers short emails", Kind: Preference},
		{Text: "Sarah is the CFO", Kind: Person},
	}
	ctx := Render(mems)
	for _, want := range []string{"prefers short emails", "Sarah is the CFO", "preference", "person"} {
		if !contains(ctx, want) {
			t.Errorf("context missing %q:\n%s", want, ctx)
		}
	}
	if Render(nil) != "" {
		t.Error("empty memory should render no context")
	}
}

func TestCosineSane(t *testing.T) {
	if math.Abs(cosine([]float32{1, 0}, []float32{1, 0})-1) > 1e-6 {
		t.Error("identical vectors should be 1")
	}
	if cosine([]float32{1, 0}, []float32{0, 1}) != 0 {
		t.Error("orthogonal vectors should be 0")
	}
	if cosine([]float32{1}, []float32{1, 2}) != 0 {
		t.Error("mismatched dims must not panic")
	}
}

func TestForget(t *testing.T) {
	db := testDB(t)
	storeVec(t, db, "temporary", Fact, 0.5, []float32{1, 0, 0})
	all, _ := All(db)
	if len(all) != 1 {
		t.Fatal("setup")
	}
	Forget(db, all[0].ID)
	if n, _ := Count(db); n != 0 {
		t.Error("forgotten memory should be gone")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
