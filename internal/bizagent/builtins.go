package bizagent

import (
	"fmt"
	"strings"

	"github.com/pragun/brain/internal/business"
	"github.com/pragun/brain/internal/router"
)

// The generic business toolset — "generic business stuff." These are the
// capabilities any business assistant needs regardless of the specific job:
// read and analyse spreadsheets, look things up in the vault, reach configured
// data sources. Specialised, task-specific tools are registered on top of these
// when the work is defined.

// RegisterBuiltins adds the generic toolset to a registry.
func RegisterBuiltins(r *Registry) {
	r.Register(funcTool{
		name: "summarize_spreadsheet",
		desc: "Read a spreadsheet file (.xlsx or .csv) and return its exact, computed shape: sheets, row/column counts, and per-column totals, means, min, max and growth. Numbers are computed, not estimated.",
		schema: objSchema(map[string]any{
			"path": strSchema("absolute path to the spreadsheet file"),
		}, "path"),
		run: func(env *Env, args map[string]any) (string, error) {
			s, err := business.Summarize(strArg(args, "path"))
			if err != nil {
				return "", err
			}
			return s.String(), nil
		},
	})

	r.Register(funcTool{
		name: "analyze_spreadsheet",
		desc: "Read a spreadsheet and return a narrated analysis of its trends and outliers, grounded in the computed figures. Use for a written read of a file; optionally pass a specific question.",
		schema: objSchema(map[string]any{
			"path":     strSchema("absolute path to the spreadsheet file"),
			"question": strSchema("optional specific question about the data"),
		}, "path"),
		run: func(env *Env, args map[string]any) (string, error) {
			return business.AnalyzeFile(env.Router, strArg(args, "path"), strArg(args, "question"))
		},
	})

	r.Register(funcTool{
		name: "search_vault",
		desc: "Search the user's notes and captured knowledge for anything relevant — people, projects, prior decisions, context. Returns the most relevant note excerpts.",
		schema: objSchema(map[string]any{
			"query": strSchema("what to look for"),
		}, "query"),
		run: func(env *Env, args map[string]any) (string, error) {
			if env.Index == nil {
				return "", fmt.Errorf("no vault index available")
			}
			embed, _ := env.Router.Model(router.T0)
			hits, err := env.Index.HybridSearch(env.Router.Local(), embed, strArg(args, "query"), 5)
			if err != nil {
				return "", err
			}
			if len(hits) == 0 {
				return "No relevant notes found.", nil
			}
			var b strings.Builder
			for _, h := range hits {
				fmt.Fprintf(&b, "## %s [%s]\n%s\n\n", h.Title, h.Slug, strings.TrimSpace(h.Body))
			}
			return b.String(), nil
		},
	})

	r.Register(funcTool{
		name:   "list_data_sources",
		desc:   "List the connected MCP data sources and the tools each exposes (spreadsheets in Drive, a database, a dashboard, etc.).",
		schema: objSchema(map[string]any{}),
		run: func(env *Env, args map[string]any) (string, error) {
			if len(env.MCP) == 0 {
				return "No data sources connected.", nil
			}
			tools, err := business.Discover(env.MCP)
			if err != nil {
				return "", err
			}
			var b strings.Builder
			for server, list := range tools {
				fmt.Fprintf(&b, "%s:\n", server)
				for _, t := range list {
					fmt.Fprintf(&b, "  %s — %s\n", t.Name, t.Description)
				}
			}
			return b.String(), nil
		},
	})

	r.Register(funcTool{
		name: "query_data_source",
		desc: "Call a tool on a connected MCP data source to pull data. Use list_data_sources first to see what is available.",
		schema: objSchema(map[string]any{
			"server": strSchema("the data source name"),
			"tool":   strSchema("the tool to call on it"),
		}, "server", "tool"),
		run: func(env *Env, args map[string]any) (string, error) {
			srcs, err := business.Gather(env.MCP, []business.ToolCall{{
				Server: strArg(args, "server"), Tool: strArg(args, "tool"),
			}})
			if err != nil {
				return "", err
			}
			var b strings.Builder
			for _, s := range srcs {
				b.WriteString(s.Text)
				b.WriteString("\n")
			}
			return b.String(), nil
		},
	})

	// --- specialised, task-specific tools plug in below ---
	// This is where the work the agent will actually be told to do gets wired.
	// Left intentionally empty until those jobs are specified, so the harness
	// ships generic and complete.
}

func objSchema(props map[string]any, required ...string) map[string]any {
	if props == nil {
		props = map[string]any{}
	}
	if required == nil {
		required = []string{}
	}
	return map[string]any{
		"type":                 "object",
		"properties":           props,
		"required":             required,
		"additionalProperties": false,
	}
}

func strSchema(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}
