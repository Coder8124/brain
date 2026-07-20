package capture

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Rendering the episodic tier for a human. If the raw timeline is not
// interesting to read, no amount of summarisation downstream will rescue it —
// so this exists to be looked at, before any model is pointed at the data.

func HHMM(ts int64) string {
	return time.Unix(ts, 0).Local().Format("15:04")
}

// TodayBounds returns [start, end) of the current local day.
func TodayBounds() (int64, int64) {
	now := time.Now().Local()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return start.Unix(), start.AddDate(0, 0, 1).Unix()
}

func Dur(secs int64) string {
	switch {
	case secs < 60:
		return fmt.Sprintf("%ds", secs)
	case secs < 3600:
		return fmt.Sprintf("%dm", secs/60)
	default:
		return fmt.Sprintf("%dh%02dm", secs/3600, (secs%3600)/60)
	}
}

// Render produces a human-readable day. verbose keeps the incidental sub-8s
// focus flickers that are otherwise noise.
func Render(events []Event, verbose bool) string {
	var b strings.Builder
	shown := 0

	for _, e := range events {
		if !verbose && e.Kind == Focus && e.DurS < IncidentalSecs {
			continue
		}
		shown++

		var label string
		switch e.Kind {
		case Focus:
			label = fmt.Sprintf("%-18s %s", orQ(e.App), e.Title)
		case URL:
			label = fmt.Sprintf("%-18s %s", or(e.App, "web"), e.URL)
		case Commit:
			label = fmt.Sprintf("%-18s %s — %s", "commit", e.Path, e.Title)
		default:
			label = fmt.Sprintf("%-18s %s", string(e.Kind), e.Title)
		}

		d := ""
		if e.DurS > 0 {
			d = Dur(e.DurS)
		}
		fmt.Fprintf(&b, "%s  %7s  %s\n", HHMM(e.TS), d, strings.TrimRight(label, " "))
	}

	if shown == 0 {
		b.WriteString("nothing recorded\n")
	}
	return b.String()
}

// AppTotal is time spent in one app over a window.
type AppTotal struct {
	App  string
	Secs int64
}

// ByApp is the first thing anyone wants from a timeline.
func ByApp(events []Event) []AppTotal {
	totals := map[string]int64{}
	for _, e := range events {
		if e.Kind == Focus && e.DurS >= IncidentalSecs {
			totals[orQ(e.App)] += e.DurS
		}
	}

	out := make([]AppTotal, 0, len(totals))
	for app, secs := range totals {
		out = append(out, AppTotal{app, secs})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Secs != out[j].Secs {
			return out[i].Secs > out[j].Secs
		}
		return out[i].App < out[j].App // stable output for equal totals
	})
	return out
}

func or(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func orQ(s string) string { return or(s, "?") }
