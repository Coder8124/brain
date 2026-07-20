package secretary

import (
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/pragun/brain/internal/capture"
	"github.com/pragun/brain/internal/routine"
)

// A Brief is what the secretary says when you open it: the proactive digest
// that makes this a secretary and not an archive. It is assembled, not asked
// for.
//
// Every line is arithmetic over data already captured — no model runs to build
// a brief, so it is instant and it works offline. The model's only role is the
// upstream extraction of commitments; surfacing them is deterministic.
type Brief struct {
	Greeting string  `json:"greeting"`
	Loops    []Loop  `json:"loops"`
	Dormant  []Nudge `json:"dormant"`
	Usual    []Nudge `json:"usual"`
	Review   int     `json:"review"`
}

type Loop struct {
	ID      int64  `json:"id"`
	Text    string `json:"text"`
	Who     string `json:"who"`
	AgeDays int    `json:"age_days"`
	Due     string `json:"due"`
	// Stale marks a loop old enough to lead with — the thing most likely to
	// have fallen through a crack.
	Stale bool `json:"stale"`
}

type Nudge struct {
	Text   string `json:"text"`
	Detail string `json:"detail"`
}

// Compose builds the briefing for a given moment. now is passed in rather than
// read from the clock so the whole thing is testable.
func Compose(db *sql.DB, now time.Time) (Brief, error) {
	var b Brief
	b.Greeting = greeting(now)

	// --- open loops, stalest first ---
	loops, err := Open_(db)
	if err != nil {
		return b, err
	}
	sort.Slice(loops, func(i, j int) bool { return loops[i].Created < loops[j].Created })
	for _, c := range loops {
		age := int(now.Sub(time.Unix(c.Created, 0)).Hours() / 24)
		b.Loops = append(b.Loops, Loop{
			ID: c.ID, Text: c.Text, Who: c.Who, AgeDays: age, Due: c.DueHint,
			// Three days with no movement is when a loop stops being "recent"
			// and starts being "forgotten".
			Stale: age >= 3,
		})
	}

	// --- what has gone quiet, and what you usually do around now ---
	// A wide window so the baseline is stable; both signals are pure counts.
	events, err := capture.Range(db, now.AddDate(0, 0, -400).Unix(), now.Unix()+1)
	if err != nil {
		return b, err
	}

	for _, a := range routine.FindAnomalies(events, now.Unix()) {
		b.Dormant = append(b.Dormant, Nudge{
			Text:   fmt.Sprintf("%s has gone quiet", a.App),
			Detail: fmt.Sprintf("%.0f days since last use, usually every %.1f", a.ActualGapD, a.TypicalGapD),
		})
		if len(b.Dormant) >= 3 {
			break
		}
	}

	for _, n := range usualNow(events, now) {
		b.Usual = append(b.Usual, n)
		if len(b.Usual) >= 3 {
			break
		}
	}

	return b, nil
}

// usualNow finds routines whose typical window contains the current time, so
// the brief can say "you normally have X open around now". This is the
// anticipatory half of a secretary — not "what did you do" but "what comes
// next".
func usualNow(events []capture.Event, now time.Time) []Nudge {
	nowOffset := int64(now.Hour()*3600 + now.Minute()*60)
	weekday := now.Weekday() != time.Saturday && now.Weekday() != time.Sunday

	// Consider both app and site routines; a morning that always starts on a
	// particular site is as much a routine as one that starts in an app.
	var candidates []routine.Periodic
	candidates = append(candidates, routine.FindPeriodic(events)...)
	candidates = append(candidates, routine.FindPeriodicSites(events)...)

	var out []Nudge
	seen := map[string]bool{}
	for _, p := range candidates {
		if p.Weekday != weekday || seen[p.App] {
			continue
		}
		// Within the routine's own spread, widened slightly so a brief opened a
		// few minutes early still anticipates the pattern.
		lo := p.MedianStart - p.SpreadS - 15*60
		hi := p.MedianStart + p.SpreadS + 15*60
		if nowOffset >= lo && nowOffset <= hi {
			seen[p.App] = true
			out = append(out, Nudge{
				Text:   fmt.Sprintf("you usually have %s open around now", p.App),
				Detail: fmt.Sprintf("%s on %s, %.0f%% of days", p.Window(), p.Cadence(), p.Consistency*100),
			})
		}
	}
	return out
}

func greeting(now time.Time) string {
	switch h := now.Hour(); {
	case h < 5:
		return "Still up"
	case h < 12:
		return "Morning"
	case h < 17:
		return "Afternoon"
	case h < 22:
		return "Evening"
	default:
		return "Late one"
	}
}

// Headline is the single most important thing to say, for a collapsed orb
// tooltip or a one-line summary. A secretary leads with the point.
func (b Brief) Headline() string {
	for _, l := range b.Loops {
		if l.Stale {
			who := ""
			if l.Who != "" {
				who = " (" + l.Who + ")"
			}
			return fmt.Sprintf("%dd open: %s%s", l.AgeDays, l.Text, who)
		}
	}
	if len(b.Loops) > 0 {
		return fmt.Sprintf("%d open loop(s)", len(b.Loops))
	}
	if len(b.Dormant) > 0 {
		return b.Dormant[0].Text
	}
	if b.Review > 0 {
		return fmt.Sprintf("%d proposals to review", b.Review)
	}
	return "nothing pressing"
}

// IsQuiet reports whether the brief has nothing worth interrupting for, so the
// app can stay out of the way rather than manufacturing noise.
func (b Brief) IsQuiet() bool {
	return len(b.Loops) == 0 && len(b.Dormant) == 0 && len(b.Usual) == 0 && b.Review == 0
}
