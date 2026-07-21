package tutor

import (
	"database/sql"
	"time"
)

// Spaced repetition. Inspired by TurboLearn AI's flashcard/quiz study flow and
// mempalace's emphasis on durable memory (see CREDITS.md), implemented with the
// SM-2 algorithm — the same scheduler behind Anki and SuperMemo.
//
// The tutor already generates good questions; SRS is what turns a one-off quiz
// into actual retention. Cards are scheduled so you see each one just as you are
// about to forget it, which is where review time pays off most.

// Grade is the learner's self-rating of a recalled card, SM-2's 0–5 quality.
// We expose the three that matter in practice.
type Grade int

const (
	Again Grade = 2 // failed to recall; reset the interval
	Good  Grade = 4 // recalled with some effort
	Easy  Grade = 5 // recalled instantly
)

const DeckSchema = `
CREATE TABLE IF NOT EXISTS cards (
    id        INTEGER PRIMARY KEY,
    q         TEXT NOT NULL,
    a         TEXT NOT NULL,
    source    TEXT,
    ease      REAL NOT NULL DEFAULT 2.5,   -- SM-2 ease factor
    interval  INTEGER NOT NULL DEFAULT 0,  -- days until next review
    reps      INTEGER NOT NULL DEFAULT 0,  -- successful reps in a row
    due       INTEGER NOT NULL,            -- unix seconds
    created   INTEGER NOT NULL,
    fingerprint TEXT UNIQUE                -- dedup identical questions
);
CREATE INDEX IF NOT EXISTS cards_due ON cards(due);
`

func InitDeck(db *sql.DB) error {
	_, err := db.Exec(DeckSchema)
	return err
}

// AddCard files a generated card into the deck, due immediately. Idempotent on
// the question text so regenerating a quiz does not pile up duplicates.
func AddCard(db *sql.DB, c Card) (bool, error) {
	now := time.Now().Unix()
	res, err := db.Exec(
		`INSERT OR IGNORE INTO cards (q, a, source, due, created, fingerprint)
		 VALUES (?,?,?,?,?,?)`,
		c.Q, c.A, c.Source, now, now, fingerprintCard(c.Q))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// DueCard is a card ready for review.
type DueCard struct {
	ID     int64
	Q, A   string
	Source string
	Reps   int
}

// Due returns cards whose review time has arrived, oldest-due first so the most
// overdue material is seen before it decays further.
func Due(db *sql.DB, now time.Time, limit int) ([]DueCard, error) {
	rows, err := db.Query(
		`SELECT id, q, a, source, reps FROM cards WHERE due <= ? ORDER BY due LIMIT ?`,
		now.Unix(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DueCard
	for rows.Next() {
		var c DueCard
		var src sql.NullString
		if err := rows.Scan(&c.ID, &c.Q, &c.A, &src, &c.Reps); err != nil {
			return nil, err
		}
		c.Source = src.String
		out = append(out, c)
	}
	return out, rows.Err()
}

func DueCount(db *sql.DB, now time.Time) (int, error) {
	var n int
	err := db.QueryRow("SELECT COUNT(*) FROM cards WHERE due <= ?", now.Unix()).Scan(&n)
	return n, err
}

// Review applies a grade to a card and reschedules it by SM-2.
//
// The rules: a failed grade sends the card back to the start of the ladder
// (interval 1 day, streak reset) — forgetting means you relearn. A success grows
// the interval by the card's ease factor, and the ease itself drifts up for easy
// cards and down for hard ones, so genuinely difficult cards come back more
// often. Ease is floored at 1.3, SM-2's lower bound, so nothing ever gets stuck.
func Review(db *sql.DB, id int64, grade Grade, now time.Time) error {
	var ease float64
	var interval, reps int
	if err := db.QueryRow("SELECT ease, interval, reps FROM cards WHERE id = ?", id).
		Scan(&ease, &interval, &reps); err != nil {
		return err
	}

	q := float64(grade)
	if grade < Good {
		reps = 0
		interval = 1
	} else {
		reps++
		switch reps {
		case 1:
			interval = 1
		case 2:
			interval = 6
		default:
			interval = int(float64(interval)*ease + 0.5)
		}
		// SM-2 ease update.
		ease = ease + (0.1 - (5-q)*(0.08+(5-q)*0.02))
		if ease < 1.3 {
			ease = 1.3
		}
	}

	due := now.Add(time.Duration(interval) * 24 * time.Hour).Unix()
	_, err := db.Exec(
		"UPDATE cards SET ease = ?, interval = ?, reps = ?, due = ? WHERE id = ?",
		ease, interval, reps, due, id)
	return err
}

func fingerprintCard(q string) string {
	// Normalise whitespace and case so trivially different phrasings of the same
	// question collapse.
	var b []rune
	prevSpace := false
	for _, r := range q {
		if r == ' ' || r == '\t' || r == '\n' {
			if !prevSpace {
				b = append(b, ' ')
			}
			prevSpace = true
			continue
		}
		prevSpace = false
		if r >= 'A' && r <= 'Z' {
			r += 32
		}
		b = append(b, r)
	}
	return string(b)
}
