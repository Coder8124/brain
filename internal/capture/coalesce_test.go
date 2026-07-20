package capture

import "testing"

func focus(ts int64, app string) Event {
	return Event{TS: ts, Kind: Focus, App: app, Title: "w"}
}

func TestIdenticalSamplesExtendRatherThanDuplicate(t *testing.T) {
	c := NewCoalescer(60)
	for _, ts := range []int64{0, 5, 10} {
		if done := c.Push(focus(ts, "Ghostty")); done != nil {
			t.Fatalf("unexpected session closed at ts=%d", ts)
		}
	}
	if got := c.Open().DurS; got != 10 {
		t.Errorf("DurS = %d, want 10", got)
	}
}

func TestStateChangeEmitsCompletedSession(t *testing.T) {
	c := NewCoalescer(60)
	c.Push(focus(0, "Ghostty"))
	c.Push(focus(30, "Ghostty"))

	done := c.Push(focus(45, "Chrome"))
	if done == nil {
		t.Fatal("session should have closed")
	}
	if done.App != "Ghostty" {
		t.Errorf("App = %q, want Ghostty", done.App)
	}
	// Ran right up to the moment Chrome took over, not just to the last sample.
	if done.DurS != 45 {
		t.Errorf("DurS = %d, want 45", done.DurS)
	}
}

func TestLongGapDoesNotInventASessionOverSleep(t *testing.T) {
	c := NewCoalescer(60)
	c.Push(focus(0, "Ghostty"))
	c.Push(focus(20, "Ghostty"))

	// Machine slept for two hours, then the same app is frontmost again.
	done := c.Push(focus(7220, "Ghostty"))
	if done == nil {
		t.Fatal("gap should have closed the session")
	}
	if done.DurS != 20 {
		t.Errorf("DurS = %d, want 20 — must not absorb the sleep window", done.DurS)
	}
	if c.Open().TS != 7220 {
		t.Errorf("post-sleep sample should open a fresh session, got ts=%d", c.Open().TS)
	}
}

func TestFlushClosesOpenSession(t *testing.T) {
	c := NewCoalescer(60)
	c.Push(focus(0, "Ghostty"))
	c.Push(focus(12, "Ghostty"))

	if done := c.Flush(); done == nil || done.DurS != 12 {
		t.Errorf("flush = %+v, want DurS 12", done)
	}
	if c.Flush() != nil {
		t.Error("second flush should return nil")
	}
}

func TestTitleChangeWithinOneAppIsANewSession(t *testing.T) {
	c := NewCoalescer(60)
	c.Push(Event{TS: 0, Kind: Focus, App: "Chrome", Title: "inbox"})

	done := c.Push(Event{TS: 10, Kind: Focus, App: "Chrome", Title: "calendar"})
	if done == nil || done.Title != "inbox" {
		t.Errorf("title change should close the session, got %+v", done)
	}
}
