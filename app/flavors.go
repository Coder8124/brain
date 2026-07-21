package main

import (
	"time"

	"github.com/pragun/brain/internal/business"
	"github.com/pragun/brain/internal/capture/sources"
	"github.com/pragun/brain/internal/flavor"
	"github.com/pragun/brain/internal/rollup"
	"github.com/pragun/brain/internal/secretary"
	"github.com/pragun/brain/internal/tutor"
)

// Flavor bindings. The app reads and switches the active persona, and exposes
// the tutor and business capabilities the persona unlocks. As everywhere else,
// these delegate to the shared engine — the app adds no logic of its own.

// FlavorState is what the header needs to render the persona switcher.
type FlavorState struct {
	Active      string   `json:"active"`
	All         []string `json:"all"`
	ScreenNotes bool     `json:"screen_notes"`
}

func (a *App) Flavor() (FlavorState, error) {
	cfg, err := flavor.Load(a.vault)
	if err != nil {
		return FlavorState{}, err
	}
	all := make([]string, 0, 3)
	for _, f := range flavor.All() {
		all = append(all, string(f))
	}
	return FlavorState{Active: string(cfg.Active), All: all, ScreenNotes: cfg.ScreenNotes}, nil
}

func (a *App) SetFlavor(name string) error {
	f, err := flavor.Parse(name)
	if err != nil {
		return err
	}
	cfg, err := flavor.Load(a.vault)
	if err != nil {
		return err
	}
	cfg.Active = f
	return cfg.Save(a.vault)
}

func (a *App) SetScreenNotes(on bool) error {
	cfg, err := flavor.Load(a.vault)
	if err != nil {
		return err
	}
	cfg.ScreenNotes = on
	return cfg.Save(a.vault)
}

// ---- tutor ----

func (a *App) Quiz(topic string) ([]tutor.Card, error) {
	ix, err := a.open()
	if err != nil {
		return nil, err
	}
	defer ix.Close()
	rt, err := a.router()
	if err != nil {
		return nil, err
	}
	return tutor.Questions(ix, rt, topic, 5)
}

// AddCards generates questions on a topic and files them into the SRS deck,
// returning how many were added. Powers the "make flashcards" action.
func (a *App) AddCards(topic string) (int, error) {
	ix, err := a.open()
	if err != nil {
		return 0, err
	}
	defer ix.Close()
	if err := tutor.InitDeck(ix.DB); err != nil {
		return 0, err
	}
	rt, err := a.router()
	if err != nil {
		return 0, err
	}
	cards, err := tutor.Questions(ix, rt, topic, 6)
	if err != nil {
		return 0, err
	}
	added := 0
	for _, c := range cards {
		if ok, _ := tutor.AddCard(ix.DB, c); ok {
			added++
		}
	}
	return added, nil
}

// DueCards returns cards ready for spaced-repetition review right now.
func (a *App) DueCards() ([]tutor.DueCard, error) {
	ix, err := a.open()
	if err != nil {
		return nil, err
	}
	defer ix.Close()
	if err := tutor.InitDeck(ix.DB); err != nil {
		return nil, err
	}
	return tutor.Due(ix.DB, time.Now(), 30)
}

// GradeCard records a review outcome (1 again, 2 good, 3 easy) and reschedules.
func (a *App) GradeCard(id int64, grade int) error {
	ix, err := a.open()
	if err != nil {
		return err
	}
	defer ix.Close()
	if err := tutor.InitDeck(ix.DB); err != nil {
		return err
	}
	g := tutor.Good
	switch grade {
	case 1:
		g = tutor.Again
	case 3:
		g = tutor.Easy
	}
	return tutor.Review(ix.DB, id, g, time.Now())
}

// Jot is braindump from the app: capture a thought and let it be filed.
func (a *App) Jot(text string) (string, error) {
	ix, err := a.open()
	if err != nil {
		return "", err
	}
	defer ix.Close()
	if err := rollup.InitQueue(ix.DB); err != nil {
		return "", err
	}
	if err := secretary.Init(ix.DB); err != nil {
		return "", err
	}
	rt, err := a.router()
	if err != nil {
		return "", err
	}
	prop, kind, err := rollup.Braindump(ix.DB, rt, text)
	if err != nil {
		return "", err
	}
	if kind == "task" {
		secretary.Add(ix.DB, &secretary.Commitment{Text: text})
		return "filed as an open loop", nil
	}
	return "filed as " + kind + " → " + prop.Target, nil
}

func (a *App) StudyDigest(topic string) (string, error) {
	ix, err := a.open()
	if err != nil {
		return "", err
	}
	defer ix.Close()
	rt, err := a.router()
	if err != nil {
		return "", err
	}
	digest, _, err := tutor.Summarize(ix, rt, topic)
	return digest, err
}

// ---- idle help: the "you look stuck, want a hand?" flow ----

// IdleHelpDefaultThreshold is how long a student sits still before the offer
// appears. Twelve seconds — long enough that it is not a normal reading pause,
// short enough to catch someone genuinely stuck.
const IdleHelpDefaultThreshold = 12.0

// ShouldOfferHelp is polled by the app while in tutor mode. It returns true only
// when the user has gone still on a study page — the app then shows the
// yes/no offer, and calls HelpNow only if they say yes.
func (a *App) ShouldOfferHelp() bool {
	cfg, err := flavor.Load(a.vault)
	if err != nil || cfg.Active != flavor.Tutor {
		return false
	}
	idle, err := sources.IdleSeconds()
	if err != nil {
		return false
	}
	if idle < IdleHelpDefaultThreshold {
		return false
	}
	text, err := sources.CaptureScreenText(a.vault + "/.brain/scratch")
	if err != nil {
		return false
	}
	return tutor.LooksStudious(text)
}

// HelpNow captures the screen and returns coaching. Called only after the user
// clicks yes on the offer — the consent gate is in the UI, and the screen is
// never read for help without it.
func (a *App) HelpNow() (string, error) {
	rt, err := a.router()
	if err != nil {
		return "", err
	}
	text, err := sources.CaptureScreenText(a.vault + "/.brain/scratch")
	if err != nil {
		return "", err
	}
	return tutor.Help(rt, text)
}

// ---- business ----

func (a *App) BusinessTrends(question string) (string, error) {
	cfg, err := flavor.Load(a.vault)
	if err != nil {
		return "", err
	}
	rt, err := a.router()
	if err != nil {
		return "", err
	}

	discovered, err := business.Discover(cfg.MCP)
	if err != nil {
		return "", err
	}
	var calls []business.ToolCall
	for server, list := range discovered {
		for _, tl := range list {
			calls = append(calls, business.ToolCall{Server: server, Tool: tl.Name})
		}
	}
	srcs, err := business.Gather(cfg.MCP, calls)
	if err != nil {
		return "", err
	}
	return business.TrendSummary(rt, question, srcs)
}
