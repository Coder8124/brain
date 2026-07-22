// Package bizagent is the business agent harness.
//
// It is a tool-using loop, not a fixed workflow: the model is given a goal and a
// set of tools, and it decides which to call, sees the result, and continues
// until it can answer. That structure is deliberately generic — the specific
// jobs this agent will do (the real "secretary" work) plug in as additional
// tools and are defined separately. What is built here is the machinery that
// runs them.
//
// Tool selection uses sampler-enforced JSON rather than native tool-calling,
// for the same reason the rest of the system does: small local models are
// unreliable at tool schemas and dependable with a constrained JSON action.
package bizagent

import (
	"database/sql"

	"github.com/pragun/brain/internal/flavor"
	"github.com/pragun/brain/internal/index"
	"github.com/pragun/brain/internal/router"
)

// Env is the capability set tools draw on. A tool never reaches outside this.
type Env struct {
	Router *router.Router
	Index  *index.Index
	DB     *sql.DB
	Vault  string
	MCP    []flavor.MCPServer
}

// Tool is one capability the agent can invoke. Schema is a JSON Schema for the
// arguments, shown to the model so it knows how to call the tool.
type Tool interface {
	Name() string
	Description() string
	Schema() map[string]any
	Run(env *Env, args map[string]any) (string, error)
}

// Registry holds the tools available to a run. New product-specific tools are
// added here without touching the loop — the extension point for the work this
// agent will be told to do.
type Registry struct {
	order []string
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: map[string]Tool{}}
}

func (r *Registry) Register(t Tool) {
	if _, dup := r.tools[t.Name()]; !dup {
		r.order = append(r.order, t.Name())
	}
	r.tools[t.Name()] = t
}

func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) List() []Tool {
	out := make([]Tool, 0, len(r.order))
	for _, n := range r.order {
		out = append(out, r.tools[n])
	}
	return out
}

// funcTool adapts a plain function into a Tool, for the generic built-ins.
type funcTool struct {
	name, desc string
	schema     map[string]any
	run        func(env *Env, args map[string]any) (string, error)
}

func (f funcTool) Name() string                                      { return f.name }
func (f funcTool) Description() string                               { return f.desc }
func (f funcTool) Schema() map[string]any                            { return f.schema }
func (f funcTool) Run(env *Env, args map[string]any) (string, error) { return f.run(env, args) }

func strArg(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}
