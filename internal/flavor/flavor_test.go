package flavor

import "testing"

func TestParseAndDefault(t *testing.T) {
	for _, s := range []string{"secretary", "TUTOR", " business "} {
		if _, err := Parse(s); err != nil {
			t.Errorf("Parse(%q): %v", s, err)
		}
	}
	if _, err := Parse("wizard"); err == nil {
		t.Error("unknown flavor should error")
	}
}

func TestConfigDefaultsToSecretary(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Active != Secretary {
		t.Errorf("default flavor = %q, want secretary", cfg.Active)
	}
	if cfg.ScreenNotes {
		t.Error("screen notes must be off by default — it is the most invasive capability")
	}
}

func TestRoundTripPreservesMCPAndScreen(t *testing.T) {
	dir := t.TempDir()
	c := &Config{Active: Business, ScreenNotes: true,
		MCP: []MCPServer{{Name: "drive", Command: "npx", Args: []string{"-y", "server"}}}}
	if err := c.Save(dir); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Active != Business || !got.ScreenNotes || len(got.MCP) != 1 || got.MCP[0].Name != "drive" {
		t.Errorf("round trip lost data: %+v", got)
	}
}
