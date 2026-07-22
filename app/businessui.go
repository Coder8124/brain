package main

import (
	"github.com/pragun/brain/internal/bizagent"
	"github.com/pragun/brain/internal/business"
	"github.com/pragun/brain/internal/flavor"
	"github.com/pragun/brain/internal/memory"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Business capability bindings for the app — thin wrappers over the same engine
// the CLI uses, so the panel's buttons and the command line never diverge.

// PickSpreadsheet opens a native file picker for a spreadsheet.
func (a *App) PickSpreadsheet() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Choose a spreadsheet",
		Filters: []runtime.FileFilter{
			{DisplayName: "Spreadsheets", Pattern: "*.xlsx;*.xlsm;*.csv"},
		},
	})
}

// BizRead returns the exact computed shape of a spreadsheet (no model).
func (a *App) BizRead(path string) (string, error) {
	s, err := business.Summarize(path)
	if err != nil {
		return "", err
	}
	return s.String(), nil
}

// BizVerify runs the deterministic finance checks.
func (a *App) BizVerify(path string) (string, error) {
	rep, err := business.Verify(path)
	if err != nil {
		return "", err
	}
	return rep.String(), nil
}

// BizForecast projects a column forward, computed exactly.
func (a *App) BizForecast(path, column string, periods int, method string) (string, error) {
	if periods < 1 {
		periods = 4
	}
	if method == "" {
		method = "cagr"
	}
	p, err := business.ForecastFile(path, column, periods, method)
	if err != nil {
		return "", err
	}
	return p.String(), nil
}

// BizAnalyze narrates a spreadsheet's trends, grounded in computed figures.
func (a *App) BizAnalyze(path, question string) (string, error) {
	rt, err := a.router()
	if err != nil {
		return "", err
	}
	return business.AnalyzeFile(rt, path, question)
}

// BizExpenseReport builds a categorised report; totals computed, model formats.
func (a *App) BizExpenseReport(path string) (string, error) {
	rt, err := a.router()
	if err != nil {
		return "", err
	}
	return business.ExpenseReport(rt, path, "", "")
}

// RunAgent runs the business agent harness toward a goal, streaming each tool it
// reaches for over events so the panel can show the work. The gate is wired in,
// so any outbound action the agent proposes queues for approval rather than
// running.
func (a *App) RunAgent(goal string) {
	go func() {
		ix, err := a.open()
		if err != nil {
			runtime.EventsEmit(a.ctx, "agent:error", err.Error())
			return
		}
		defer ix.Close()
		rt, err := a.router()
		if err != nil {
			runtime.EventsEmit(a.ctx, "agent:error", err.Error())
			return
		}
		cfg, _ := flavor.Load(a.vault)

		reg := bizagent.NewRegistry()
		bizagent.RegisterBuiltins(reg)
		bizagent.RegisterTasks(reg)
		bizagent.RegisterGate(reg)
		bizagent.RegisterDefaultExecutors(a.vault)

		env := &bizagent.Env{Router: rt, Index: ix, DB: ix.DB, Vault: a.vault, MCP: cfg.MCP}
		runner := bizagent.NewRunner(env, reg)

		answer, err := runner.Run(goal, func(s bizagent.Step) {
			if s.Tool != "" {
				runtime.EventsEmit(a.ctx, "agent:step", s.Tool)
			}
		})
		if err != nil {
			runtime.EventsEmit(a.ctx, "agent:error", err.Error())
			return
		}
		runtime.EventsEmit(a.ctx, "agent:done", answer)
	}()
}

// Memories returns what the assistant has learned about the user, for the
// memory view — the visible face of persistent memory.
func (a *App) Memories() ([]memory.Memory, error) {
	ix, err := a.open()
	if err != nil {
		return nil, err
	}
	defer ix.Close()
	if err := memory.Init(ix.DB); err != nil {
		return nil, err
	}
	return memory.All(ix.DB)
}

func (a *App) ForgetMemory(id int64) error {
	ix, err := a.open()
	if err != nil {
		return err
	}
	defer ix.Close()
	return memory.Forget(ix.DB, id)
}
