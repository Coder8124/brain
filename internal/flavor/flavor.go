// Package flavor is the personality the assistant wears.
//
// The same engine — capture, vault, brief — serves every flavor; a flavor only
// changes what gets emphasised and which extra capabilities are switched on.
// Secretary is the base: proactive about your day. Tutor turns the vault into
// study material and takes notes off your screen. Business reaches out through
// MCP servers to summarise data.
package flavor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Flavor string

const (
	Secretary Flavor = "secretary"
	Tutor     Flavor = "tutor"
	Business  Flavor = "business"
)

// All returns the flavors this edition offers (see edition.go).
func All() []Flavor { return Offered }

func offered(f Flavor) bool {
	for _, o := range Offered {
		if o == f {
			return true
		}
	}
	return false
}

// Parse accepts only a flavor this edition actually offers, so `brain mode
// business` fails cleanly on the student build rather than half-working.
func Parse(s string) (Flavor, error) {
	f := Flavor(strings.ToLower(strings.TrimSpace(s)))
	switch f {
	case Secretary, Tutor, Business:
		if !offered(f) {
			return "", fmt.Errorf("this edition (%s) does not offer the %q flavor", EditionName, f)
		}
		return f, nil
	}
	return "", fmt.Errorf("unknown flavor %q", s)
}

func (f Flavor) Describe() string {
	switch f {
	case Tutor:
		return "turns the vault into study material and takes notes off your screen"
	case Business:
		return "reaches through MCP servers to summarise data and trends"
	default:
		return "proactive about your day: meetings, open loops, routines"
	}
}

// Config is the flavor layer's slice of .brain/config.json. Kept in its own file
// rather than folded into the model router config, so switching personas never
// risks disturbing model or key settings.
type Config struct {
	Active Flavor `json:"active"`
	// ScreenNotes gates the tutor screen-capture pipeline. Off by default and
	// only ever consulted in tutor mode — screen capture is the most invasive
	// thing the system can do, so it takes a deliberate opt-in.
	ScreenNotes bool `json:"screen_notes"`
	// MCP servers business mode may reach. Empty until the user adds one.
	MCP []MCPServer `json:"mcp,omitempty"`
}

// MCPServer is a stdio MCP server brain can launch and talk to.
type MCPServer struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

func path(vault string) string { return filepath.Join(vault, ".brain", "flavor.json") }

func Load(vault string) (*Config, error) {
	cfg := &Config{Active: Default}

	raw, err := os.ReadFile(path(vault))
	if os.IsNotExist(err) {
		return cfg, nil // absent config means this edition's default flavor
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path(vault), err)
	}
	// A saved flavor this edition no longer offers (e.g. a config carried over
	// from the full build) falls back to the default rather than showing a
	// persona the product does not have.
	if cfg.Active == "" || !offered(cfg.Active) {
		cfg.Active = Default
	}
	return cfg, nil
}

func (c *Config) Save(vault string) error {
	if err := os.MkdirAll(filepath.Dir(path(vault)), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path(vault), raw, 0o600)
}
