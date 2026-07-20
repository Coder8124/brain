package capture

import "testing"

func TestBlocksPasswordManagersBySubstring(t *testing.T) {
	p := DefaultPolicy()
	for _, app := range []string{"1Password 8", "Bitwarden", "Keychain Access"} {
		if !p.ShouldDrop(Event{Kind: Focus, App: app}) {
			t.Errorf("%q should be blocked", app)
		}
	}
	if p.ShouldDrop(Event{Kind: Focus, App: "Ghostty", Title: "zsh"}) {
		t.Error("Ghostty should not be blocked")
	}
}

func TestBlocksSensitiveTitlesInAnyApp(t *testing.T) {
	p := DefaultPolicy()
	e := Event{Kind: Focus, App: "Google Chrome", Title: "Sign in - Bank"}
	if !p.ShouldDrop(e) {
		t.Error("sensitive titles must be dropped regardless of app")
	}
}

func TestUnidentifiableSampleFailsClosed(t *testing.T) {
	if !DefaultPolicy().ShouldDrop(Event{Kind: Focus}) {
		t.Error("a sample with no app, url or path must be dropped")
	}
}

func TestPauseDropsEverything(t *testing.T) {
	p := DefaultPolicy()
	p.Paused = true
	if !p.ShouldDrop(Event{Kind: Focus, App: "Ghostty"}) {
		t.Error("pause must drop everything")
	}
}

func TestUserAdditionsAreHonoured(t *testing.T) {
	p := DefaultPolicy().WithExtra([]string{"Obsidian"})
	if !p.ShouldDrop(Event{Kind: Focus, App: "Obsidian"}) {
		t.Error("user blocklist additions must apply")
	}
}
