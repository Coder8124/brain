package capture

import "strings"

// DefaultBlocklist is seeded with the categories never worth recording. The
// user's own additions are appended from config.
var DefaultBlocklist = []string{
	"1Password", "Bitwarden", "KeePassXC", "Keychain Access",
	"Messages", "Signal", "WhatsApp", "Telegram",
	"Mail", "FaceTime", "System Settings",
}

// Window titles meaning something sensitive is on screen, regardless of app.
var blockedTitleHints = []string{
	"password", "passwd", "secret", "private browsing", "incognito", "sign in", "login",
}

// Policy fails closed everywhere: an app we cannot identify is treated as
// blocked, because wrongly recording a password manager costs far more than a
// gap in the timeline.
type Policy struct {
	BlockedApps []string
	Paused      bool
}

func DefaultPolicy() *Policy {
	return &Policy{BlockedApps: append([]string(nil), DefaultBlocklist...)}
}

func (p *Policy) WithExtra(extra []string) *Policy {
	p.BlockedApps = append(p.BlockedApps, extra...)
	return p
}

func (p *Policy) appBlocked(app string) bool {
	a := strings.ToLower(app)
	for _, b := range p.BlockedApps {
		if strings.Contains(a, strings.ToLower(b)) {
			return true
		}
	}
	return false
}

// ShouldDrop reports whether a sample must never be written. Dropped means
// never persisted — not written-then-filtered, which would leave the content
// on disk.
func (p *Policy) ShouldDrop(e Event) bool {
	if p.Paused {
		return true
	}
	if e.App != "" && p.appBlocked(e.App) {
		return true
	}
	// An unidentifiable sample with nothing else to go on is blocked, not allowed.
	if e.App == "" && e.URL == "" && e.Path == "" {
		return true
	}
	if e.Title != "" {
		t := strings.ToLower(e.Title)
		for _, h := range blockedTitleHints {
			if strings.Contains(t, h) {
				return true
			}
		}
	}
	return false
}
