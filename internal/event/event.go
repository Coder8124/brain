// Package event defines the episodic record shared by capture and its sources.
//
// These rows are high-volume and individually near-worthless; their value is
// entirely in aggregate. They are never written to markdown directly and are
// pruned on a retention window.
package event

import "time"

type Kind string

const (
	Focus     Kind = "focus"
	URL       Kind = "url"
	File      Kind = "file"
	Commit    Kind = "commit"
	Calendar  Kind = "calendar"
	Clipboard Kind = "clipboard"
)

type Event struct {
	// ID is the rowid once persisted, zero before. Proposals cite these, so
	// every inference can be traced back to the observations behind it.
	ID    int64
	TS    int64
	Kind  Kind
	App   string
	Title string
	URL   string
	Path  string
	// DurS is how long this state persisted, filled in by the coalescer rather
	// than by the source.
	DurS int64
}

// Identity is what makes two samples "the same state" for coalescing.
type Identity struct {
	Kind       Kind
	App, Title string
}

func (e Event) Identity() Identity {
	return Identity{e.Kind, e.App, e.Title}
}

// IncidentalSecs: a focus session shorter than this is almost always
// incidental — tabbing through windows to get somewhere else. Kept in the raw
// table for completeness but excluded from rollups and routine mining.
const IncidentalSecs int64 = 8

func Now() int64 { return time.Now().Unix() }
