package business

import (
	"fmt"
	"strings"

	"github.com/pragun/brain/internal/flavor"
	"github.com/pragun/brain/internal/router"
)

// Source is data pulled from one MCP tool call, ready to be summarised.
type Source struct {
	Server string
	Tool   string
	Text   string
}

// Gather runs one tool on each configured server and collects the results.
//
// Deliberately shallow: it calls the tools the user pointed at, it does not let
// a model drive an open-ended agent loop over the servers. Business mode
// summarises data the user chose to expose, which keeps both the blast radius
// and the token cost bounded.
func Gather(servers []flavor.MCPServer, calls []ToolCall) ([]Source, error) {
	var out []Source
	for _, server := range servers {
		relevant := callsFor(calls, server.Name)
		if len(relevant) == 0 {
			continue
		}

		client, err := Connect(server)
		if err != nil {
			return out, fmt.Errorf("%s: %w", server.Name, err)
		}
		for _, call := range relevant {
			text, err := client.Call(call.Tool, call.Args)
			if err != nil {
				client.Close()
				return out, fmt.Errorf("%s.%s: %w", server.Name, call.Tool, err)
			}
			out = append(out, Source{Server: server.Name, Tool: call.Tool, Text: text})
		}
		client.Close()
	}
	return out, nil
}

// ToolCall names a tool to run on a server with its arguments.
type ToolCall struct {
	Server string
	Tool   string
	Args   map[string]any
}

func callsFor(calls []ToolCall, server string) []ToolCall {
	var out []ToolCall
	for _, c := range calls {
		if c.Server == server {
			out = append(out, c)
		}
	}
	return out
}

// TrendSummary asks the model to read the gathered data and report the trends
// that matter for a business — movement, outliers, and what to watch — rather
// than restating the numbers.
//
// Uses T2: this is synthesis over potentially messy tabular text, which is
// exactly where a small model falls down. Cloud T3 is a natural upgrade when
// the data is large, gated by the same redaction preview as any other egress.
func TrendSummary(rt *router.Router, question string, sources []Source) (string, error) {
	if len(sources) == 0 {
		return "", fmt.Errorf("no data gathered — configure an MCP server and a tool to call")
	}

	model, err := rt.Model(router.T2)
	if err != nil {
		return "", err
	}

	var ctx strings.Builder
	for _, s := range sources {
		fmt.Fprintf(&ctx, "### %s · %s\n%s\n\n", s.Server, s.Tool, strings.TrimSpace(s.Text))
	}

	system := "You are a business analyst. From the data below, report the trends that matter: " +
		"what is moving and in which direction, notable outliers, and what to keep an eye on. " +
		"Lead with the single most important finding. Ground every claim in the data — cite the " +
		"figures — and say plainly when the data is too thin to support a conclusion. Be concise."

	prompt := ctx.String()
	if question != "" {
		prompt = "Question: " + question + "\n\n" + prompt
	}
	return rt.Local().Chat(model, system, prompt, nil)
}

// Discover lists the tools every configured server exposes, so the user knows
// what they can point business mode at.
func Discover(servers []flavor.MCPServer) (map[string][]Tool, error) {
	out := map[string][]Tool{}
	for _, server := range servers {
		client, err := Connect(server)
		if err != nil {
			return out, fmt.Errorf("%s: %w", server.Name, err)
		}
		tools, err := client.Tools()
		client.Close()
		if err != nil {
			return out, fmt.Errorf("%s: %w", server.Name, err)
		}
		out[server.Name] = tools
	}
	return out, nil
}
