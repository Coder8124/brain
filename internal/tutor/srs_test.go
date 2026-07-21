package tutor

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func deckDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if err := InitDeck(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestAddCardIsIdempotent(t *testing.T) {
	db := deckDB(t)
	added, _ := AddCard(db, Card{Q: "What is an eigenvalue?", A: "a scalar"})
	if !added {
		t.Fatal("first add should insert")
	}
	// Same question, different whitespace/case — must not duplicate.
	added, _ = AddCard(db, Card{Q: "what is  an eigenvalue?", A: "a scalar"})
	if added {
		t.Error("a fingerprint-equal card must not be added twice")
	}
}

func TestNewCardsAreDueNow(t *testing.T) {
	db := deckDB(t)
	AddCard(db, Card{Q: "q1", A: "a1"})
	due, _ := Due(db, time.Now(), 10)
	if len(due) != 1 {
		t.Fatalf("a fresh card should be due, got %d", len(due))
	}
}

func TestSuccessfulReviewGrowsTheInterval(t *testing.T) {
	db := deckDB(t)
	AddCard(db, Card{Q: "q1", A: "a1"})
	now := time.Now()
	due, _ := Due(db, now, 1)
	id := due[0].ID

	// Three good reviews: intervals should be 1, 6, then >6 days.
	Review(db, id, Good, now)
	if n, _ := DueCount(db, now); n != 0 {
		t.Error("a reviewed card should not still be due today")
	}
	if n, _ := DueCount(db, now.AddDate(0, 0, 2)); n != 1 {
		t.Error("after 1-day interval the card should be due in 2 days")
	}

	Review(db, id, Good, now.AddDate(0, 0, 1))
	// Now interval is 6 days: not due at +3, due at +8.
	if n, _ := DueCount(db, now.AddDate(0, 0, 4)); n != 0 {
		t.Error("second review should push the card ~6 days out")
	}
	if n, _ := DueCount(db, now.AddDate(0, 0, 8)); n != 1 {
		t.Error("card should come due again after the 6-day interval")
	}
}

func TestFailureResetsTheCard(t *testing.T) {
	db := deckDB(t)
	AddCard(db, Card{Q: "q1", A: "a1"})
	now := time.Now()
	due, _ := Due(db, now, 1)
	id := due[0].ID

	// Build up an interval, then fail.
	Review(db, id, Good, now)
	Review(db, id, Good, now.AddDate(0, 0, 1))
	Review(db, id, Again, now.AddDate(0, 0, 7))

	// After a failure the card is back to a 1-day interval.
	if n, _ := DueCount(db, now.AddDate(0, 0, 9)); n != 1 {
		t.Error("a failed card should return to a short interval, not stay far out")
	}
}

func TestEaseNeverDropsBelowFloor(t *testing.T) {
	db := deckDB(t)
	AddCard(db, Card{Q: "hard", A: "a"})
	now := time.Now()
	id := int64(1)
	// Repeatedly barely-pass; ease should drift down but floor at 1.3.
	for i := 0; i < 10; i++ {
		Review(db, id, Good, now.AddDate(0, 0, i))
	}
	var ease float64
	db.QueryRow("SELECT ease FROM cards WHERE id = 1").Scan(&ease)
	if ease < 1.3 {
		t.Errorf("ease = %v, must not fall below the SM-2 floor of 1.3", ease)
	}
}
