package main

import (
	"github.com/pragun/brain/internal/record"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Screen recording from the app: start/stop a study-session capture, then turn
// it into notes. Triggered by an in-panel button and by a global hotkey (wired
// in main.go), so it works even while the frameless panel is hidden.

var recorder *record.Recorder

// RecordingActive reports whether a session is in progress, for the orb/button.
func (a *App) RecordingActive() bool {
	return recorder != nil && recorder.Running()
}

// ToggleRecording starts a session if idle, or stops and processes one if
// running. Returns the new state ("recording" or "processing"/"idle") so the UI
// can update immediately; processing continues over events.
func (a *App) ToggleRecording(name string) string {
	if recorder != nil && recorder.Running() {
		go a.finishRecording(name)
		return "processing"
	}

	recorder = record.NewRecorder(a.vault + "/.brain/scratch")
	if err := recorder.Start(record.FFmpegAvailable()); err != nil {
		runtime.EventsEmit(a.ctx, "record:error", err.Error())
		return "idle"
	}
	runtime.EventsEmit(a.ctx, "record:started", "")
	return "recording"
}

// finishRecording stops the session and processes it, emitting the result. It
// runs off the UI path because the summarising model call takes tens of seconds.
func (a *App) finishRecording(name string) {
	session := recorder.Stop()
	runtime.EventsEmit(a.ctx, "record:processing", "")

	ix, err := a.open()
	if err != nil {
		runtime.EventsEmit(a.ctx, "record:error", err.Error())
		return
	}
	defer ix.Close()

	rt, err := a.router()
	if err != nil {
		runtime.EventsEmit(a.ctx, "record:error", err.Error())
		return
	}

	res, err := record.Process(rt, ix.DB, a.vault, session, name)
	if err != nil {
		runtime.EventsEmit(a.ctx, "record:error", err.Error())
		return
	}
	runtime.EventsEmit(a.ctx, "record:done", map[string]any{
		"title": res.Title,
		"note":  res.NotePath,
		"video": res.VideoPath,
		"cards": res.Cards,
	})
}
