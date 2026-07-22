package bizagent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pragun/brain/internal/provider"
	"github.com/pragun/brain/internal/router"
)

// Runner drives a tool-using loop toward a goal. Each turn the model chooses one
// action — call a tool, or finish — as constrained JSON; the harness executes
// tools and feeds results back until the model finishes or the step budget runs
// out. The step cap is the safety rail: a local model that keeps calling tools
// forever is stopped rather than spinning.
type Runner struct {
	env      *Env
	reg      *Registry
	MaxSteps int
}

func NewRunner(env *Env, reg *Registry) *Runner {
	return &Runner{env: env, reg: reg, MaxSteps: 8}
}

// Step is one turn of the loop, surfaced so a caller (CLI, app) can show the
// agent's work as it happens — which tool it reached for and what came back.
type Step struct {
	Thought string
	Tool    string
	Args    map[string]any
	Result  string
	Final   string
}

var actionSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"thought": map[string]any{"type": "string"},
		"action":  map[string]any{"type": "string", "enum": []string{"use_tool", "finish"}},
		"tool":    map[string]any{"type": "string"},
		"args":    map[string]any{"type": "object"},
		"answer":  map[string]any{"type": "string"},
	},
	"required":             []string{"thought", "action"},
	"additionalProperties": false,
}

// Run executes the loop. onStep, if non-nil, is called after each turn. The
// returned string is the agent's final answer.
func (r *Runner) Run(goal string, onStep func(Step)) (string, error) {
	model, err := r.env.Router.Model(router.T2)
	if err != nil {
		return "", err
	}

	transcript := []provider.Msg{
		{Role: "system", Content: r.systemPrompt()},
		{Role: "user", Content: "Goal: " + goal},
	}

	for step := 0; step < r.MaxSteps; step++ {
		// Force the last-turn decision even if the model is verbose by asking
		// for the action JSON explicitly each turn.
		out, err := r.env.Router.Local().Chat(model, r.systemPrompt(),
			renderTranscript(transcript, goal), actionSchema)
		if err != nil {
			return "", err
		}

		var act struct {
			Thought string         `json:"thought"`
			Action  string         `json:"action"`
			Tool    string         `json:"tool"`
			Args    map[string]any `json:"args"`
			Answer  string         `json:"answer"`
		}
		if err := json.Unmarshal([]byte(cleanJSON(out)), &act); err != nil {
			// A malformed action ends the run rather than looping on garbage.
			return "", fmt.Errorf("agent produced an unparseable action: %w", err)
		}

		if act.Action == "finish" {
			if onStep != nil {
				onStep(Step{Thought: act.Thought, Final: act.Answer})
			}
			return act.Answer, nil
		}

		tool, ok := r.reg.Get(act.Tool)
		s := Step{Thought: act.Thought, Tool: act.Tool, Args: act.Args}
		if !ok {
			s.Result = fmt.Sprintf("error: no tool named %q; available: %s", act.Tool, r.toolNames())
		} else {
			res, err := tool.Run(r.env, act.Args)
			if err != nil {
				s.Result = "error: " + err.Error()
			} else {
				s.Result = res
			}
		}
		if onStep != nil {
			onStep(s)
		}

		// Feed the observation back so the next turn can build on it.
		transcript = append(transcript,
			provider.Msg{Role: "assistant", Content: fmt.Sprintf("thought: %s\ncalling %s(%s)", act.Thought, act.Tool, jsonArgs(act.Args))},
			provider.Msg{Role: "user", Content: "result:\n" + truncate(s.Result, 3000)})
	}

	// Out of steps: ask for a best-effort answer from what was gathered.
	final, _ := r.env.Router.Local().Chat(model,
		"Summarise the best answer you can from the work so far. Be honest about what is incomplete.",
		renderTranscript(transcript, goal), nil)
	return strings.TrimSpace(final), nil
}

func (r *Runner) systemPrompt() string {
	var b strings.Builder
	b.WriteString("You are a capable business assistant working toward a goal by using tools. ")
	b.WriteString("Each turn, reply with JSON only: either use a tool, or finish with an answer. ")
	b.WriteString("Never invent figures — get them from a tool. When you have enough to answer the goal, finish.\n\n")
	b.WriteString("Available tools:\n")
	for _, t := range r.reg.List() {
		fmt.Fprintf(&b, "- %s: %s\n", t.Name(), t.Description())
	}
	return b.String()
}

func (r *Runner) toolNames() string {
	var names []string
	for _, t := range r.reg.List() {
		names = append(names, t.Name())
	}
	return strings.Join(names, ", ")
}

func renderTranscript(msgs []provider.Msg, goal string) string {
	var b strings.Builder
	for _, m := range msgs {
		if m.Role == "system" {
			continue
		}
		fmt.Fprintf(&b, "%s: %s\n", m.Role, m.Content)
	}
	b.WriteString("\nWhat is your next action? Reply with the action JSON.")
	return b.String()
}

func jsonArgs(args map[string]any) string {
	b, _ := json.Marshal(args)
	return string(b)
}

func cleanJSON(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = s[i+1:]
		}
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}
	return strings.TrimSpace(s)
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "\n… (truncated)"
	}
	return s
}
