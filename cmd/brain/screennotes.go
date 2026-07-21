package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/pragun/brain/internal/capture"
	"github.com/pragun/brain/internal/capture/sources"
	"github.com/pragun/brain/internal/flavor"
	"github.com/pragun/brain/internal/index"
	"github.com/pragun/brain/internal/rollup"
	"github.com/pragun/brain/internal/router"
	"github.com/pragun/brain/internal/tutor"
)

// screenWatcher runs the tutor's screen note-taking inside the capture daemon.
//
// It only does anything in tutor mode with screen notes enabled, so the check
// is re-read each tick — the user can switch flavors while the daemon runs and
// this starts or stops accordingly, without a restart.
type screenWatcher struct {
	ix      *index.Index
	rt      *router.Router
	vault   string
	scratch string
	// dedup so the same page held on screen for ten minutes is noted once, not
	// every tick.
	lastTopic string
}

func newScreenWatcher(ix *index.Index, vault, scratch string) (*screenWatcher, error) {
	rt, err := openRouter()
	if err != nil {
		return nil, err
	}
	return &screenWatcher{ix: ix, rt: rt, vault: vault, scratch: scratch}, nil
}

// enabled reports whether screen note-taking should run right now.
func (w *screenWatcher) enabled() bool {
	cfg, err := flavor.Load(w.vault)
	return err == nil && cfg.Active == flavor.Tutor && cfg.ScreenNotes
}

// tick captures the screen once and, if it is study material, queues a note.
// Returns a short status for the daemon log, or "" when nothing happened.
func (w *screenWatcher) tick() string {
	if !w.enabled() {
		return ""
	}

	text, err := sources.CaptureScreenText(w.scratch)
	if err != nil {
		return "" // permission missing or capture failed; stay quiet
	}
	if !tutor.LooksStudious(text) {
		w.lastTopic = "" // left the study page; allow the next one to note again
		return ""
	}

	note, err := tutor.Distil(w.rt, text)
	if err != nil || note == nil {
		return ""
	}
	if strings.EqualFold(note.Topic, w.lastTopic) {
		return "" // same page as last tick
	}
	w.lastTopic = note.Topic

	// Record the capture as a screen event so the note has traceable evidence,
	// exactly like every other proposal. The event carries no OCR text — only
	// that a studious screen was seen and its topic.
	ev := capture.Event{TS: capture.Now(), Kind: capture.Screen, App: "study", Title: note.Topic}
	if err := capture.Insert(w.ix.DB, ev); err != nil {
		return ""
	}
	var evID int64
	w.ix.DB.QueryRow("SELECT last_insert_rowid()").Scan(&evID)

	body := "- " + strings.Join(note.Notes, "\n- ")
	prop := &rollup.Proposal{
		Kind:     rollup.NewNote,
		Target:   "study/" + slugifyTopic(note.Topic),
		Conf:     0.6,
		Evidence: []int64{evID},
		Model:    "tutor-screen",
		Payload:  rollup.Payload{Title: note.Topic, Type: "study", Body: body},
	}
	if err := rollup.Enqueue(w.ix.DB, prop); err != nil {
		return ""
	}
	return fmt.Sprintf("noted from screen: %s (%d points)", note.Topic, len(note.Notes))
}

// checkIdleHelp reports a help offer when the student has gone still on a study
// page. The daemon only surfaces the *offer* — the actual help waits on the
// user saying yes, which happens in the app.
func (w *screenWatcher) checkIdleHelp(threshold float64) string {
	if !w.enabled() {
		return ""
	}
	idle, err := sources.IdleSeconds()
	if err != nil || idle < threshold {
		return ""
	}
	text, err := sources.CaptureScreenText(w.scratch)
	if err != nil || !tutor.LooksStudious(text) {
		return ""
	}
	return "you've been on this a while — `brain tutor help` if you're stuck"
}

func slugifyTopic(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return fmt.Sprintf("note-%d", time.Now().Unix())
	}
	return out
}
