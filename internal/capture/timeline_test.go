package capture

import (
	"strings"
	"testing"
)

func TestFormatsDurationsAtEachScale(t *testing.T) {
	cases := map[int64]string{45: "45s", 600: "10m", 3700: "1h01m"}
	for secs, want := range cases {
		if got := Dur(secs); got != want {
			t.Errorf("Dur(%d) = %q, want %q", secs, got, want)
		}
	}
}

func TestHidesIncidentalFlickersUnlessVerbose(t *testing.T) {
	events := []Event{
		{TS: 0, Kind: Focus, App: "Finder", DurS: 3},
		{TS: 10, Kind: Focus, App: "Ghostty", DurS: 600},
	}
	if strings.Contains(Render(events, false), "Finder") {
		t.Error("sub-8s flicker should be hidden by default")
	}
	if !strings.Contains(Render(events, true), "Finder") {
		t.Error("verbose should show flickers")
	}
}

func TestByAppTotalsAndSortsExcludingFlickers(t *testing.T) {
	events := []Event{
		{TS: 0, Kind: Focus, App: "Ghostty", DurS: 300},
		{TS: 400, Kind: Focus, App: "Chrome", DurS: 900},
		{TS: 1400, Kind: Focus, App: "Ghostty", DurS: 200},
		{TS: 2400, Kind: Focus, App: "Finder", DurS: 2},
	}
	got := ByApp(events)
	if len(got) != 2 {
		t.Fatalf("got %d apps, want 2: %+v", len(got), got)
	}
	if got[0] != (AppTotal{"Chrome", 900}) {
		t.Errorf("first = %+v, want Chrome 900", got[0])
	}
	if got[1] != (AppTotal{"Ghostty", 500}) {
		t.Errorf("second = %+v, want Ghostty 500", got[1])
	}
}

func TestEmptyDaySaysSo(t *testing.T) {
	if got := Render(nil, false); got != "nothing recorded\n" {
		t.Errorf("Render(nil) = %q", got)
	}
}
