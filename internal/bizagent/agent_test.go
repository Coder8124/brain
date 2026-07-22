package bizagent

import (
	"strings"
	"testing"
)

// A fake tool that records it was called, to prove the registry + dispatch work
// independently of any model.
type recorderTool struct {
	called bool
	arg    string
}

func (r *recorderTool) Name() string        { return "echo" }
func (r *recorderTool) Description() string { return "echoes its text argument" }
func (r *recorderTool) Schema() map[string]any {
	return objSchema(map[string]any{"text": strSchema("x")}, "text")
}
func (r *recorderTool) Run(env *Env, args map[string]any) (string, error) {
	r.called = true
	r.arg = strArg(args, "text")
	return "echoed: " + r.arg, nil
}

func TestRegistryRegisterAndGet(t *testing.T) {
	reg := NewRegistry()
	echo := &recorderTool{}
	reg.Register(echo)
	RegisterBuiltins(reg)

	if _, ok := reg.Get("echo"); !ok {
		t.Error("registered tool not found")
	}
	// Built-ins are present.
	for _, want := range []string{"summarize_spreadsheet", "analyze_spreadsheet", "search_vault", "list_data_sources", "query_data_source"} {
		if _, ok := reg.Get(want); !ok {
			t.Errorf("built-in %q missing", want)
		}
	}
	// Re-registering does not duplicate in List order.
	before := len(reg.List())
	reg.Register(echo)
	if len(reg.List()) != before {
		t.Error("re-registering the same tool should not add a second entry")
	}
}

func TestToolDispatchExecutesAndReturns(t *testing.T) {
	reg := NewRegistry()
	echo := &recorderTool{}
	reg.Register(echo)

	tool, ok := reg.Get("echo")
	if !ok {
		t.Fatal("tool not found")
	}
	out, err := tool.Run(&Env{}, map[string]any{"text": "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if !echo.called || echo.arg != "hello" {
		t.Error("tool did not receive its argument")
	}
	if out != "echoed: hello" {
		t.Errorf("tool output = %q", out)
	}
}

func TestSystemPromptListsTools(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg)
	r := NewRunner(&Env{}, reg)
	p := r.systemPrompt()
	for _, want := range []string{"summarize_spreadsheet", "search_vault", "JSON only"} {
		if !strings.Contains(p, want) {
			t.Errorf("system prompt missing %q", want)
		}
	}
}

func TestCleanJSONStripsFences(t *testing.T) {
	if got := cleanJSON("```json\n{\"a\":1}\n```"); got != `{"a":1}` {
		t.Errorf("cleanJSON = %q", got)
	}
}
