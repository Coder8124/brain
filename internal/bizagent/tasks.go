package bizagent

import (
	"strconv"
	"strings"

	"github.com/pragun/brain/internal/business"
)

// The task-specific tools — the work the agent was told to do. These are the
// specialised capabilities that plug into the generic harness: verifying
// finances, forecasting, and producing the deliverables a business assistant
// turns out. Numbers flow through the deterministic engines in internal/business
// before any model narration, so the agent cannot report a figure it did not
// compute.

// RegisterTasks adds the specialised business tools to a registry.
func RegisterTasks(r *Registry) {
	r.Register(funcTool{
		name: "verify_finances",
		desc: "Run deterministic consistency checks on a financial spreadsheet: column arithmetic (does net = revenue − expenses?), total-row reconciliation, outliers, sign anomalies, blanks, and duplicate rows. Returns concrete pass/fail findings.",
		schema: objSchema(map[string]any{
			"path": strSchema("path to the spreadsheet to verify"),
		}, "path"),
		run: func(env *Env, args map[string]any) (string, error) {
			rep, err := business.Verify(strArg(args, "path"))
			if err != nil {
				return "", err
			}
			return rep.String(), nil
		},
	})

	r.Register(funcTool{
		name: "forecast",
		desc: "Forecast a numeric column (revenue, margin, profit) forward N periods. Growth is computed exactly (CAGR or linear) from the history; the projection is not a model guess. Args: path, column, periods, method (cagr|linear).",
		schema: objSchema(map[string]any{
			"path":    strSchema("spreadsheet path"),
			"column":  strSchema("the column header to forecast"),
			"periods": strSchema("how many periods ahead (number)"),
			"method":  strSchema("cagr or linear"),
		}, "path", "column"),
		run: func(env *Env, args map[string]any) (string, error) {
			periods := 4
			if n, err := strconv.Atoi(strArg(args, "periods")); err == nil && n > 0 {
				periods = n
			}
			method := strArg(args, "method")
			if method == "" {
				method = "cagr"
			}
			p, err := business.ForecastFile(strArg(args, "path"), strArg(args, "column"), periods, method)
			if err != nil {
				return "", err
			}
			return p.String(), nil
		},
	})

	r.Register(funcTool{
		name: "expense_report",
		desc: "Build a categorised expense report from a spreadsheet. Category totals are computed exactly; a written summary is generated from them. Args: path, and optional category_column / amount_column.",
		schema: objSchema(map[string]any{
			"path":            strSchema("expense spreadsheet path"),
			"category_column": strSchema("optional: the category column header"),
			"amount_column":   strSchema("optional: the amount column header"),
		}, "path"),
		run: func(env *Env, args map[string]any) (string, error) {
			return business.ExpenseReport(env.Router, strArg(args, "path"),
				strArg(args, "category_column"), strArg(args, "amount_column"))
		},
	})

	r.Register(funcTool{
		name: "make_presentation",
		desc: "Draft a slide deck on a topic, grounded in any data files provided. Returns markdown (Marp/reveal-compatible). Args: topic, and optional data (comma-separated file paths).",
		schema: objSchema(map[string]any{
			"topic": strSchema("what the presentation is about"),
			"data":  strSchema("optional comma-separated paths to supporting spreadsheets/files"),
		}, "topic"),
		run: func(env *Env, args map[string]any) (string, error) {
			_, md, err := business.Presentation(env.Router, strArg(args, "topic"), splitPaths(strArg(args, "data")), "")
			return md, err
		},
	})

	r.Register(funcTool{
		name: "competitor_analysis",
		desc: "Write a competitor analysis strictly from provided material (a brief and/or data files). Will not invent competitors or figures. Args: brief, and optional data (comma-separated paths).",
		schema: objSchema(map[string]any{
			"brief": strSchema("what to analyse and any context"),
			"data":  strSchema("optional comma-separated paths to data about the competitors"),
		}, "brief"),
		run: func(env *Env, args map[string]any) (string, error) {
			return business.CompetitorAnalysis(env.Router, strArg(args, "brief"), splitPaths(strArg(args, "data")))
		},
	})

	r.Register(funcTool{
		name: "stock_analysis",
		desc: "Analyse a company from SUPPLIED financial data only (no live market feed). Lays out the case for and against; never gives a buy/sell call; always ends with a not-financial-advice note. Args: ticker, optional data paths, optional notes.",
		schema: objSchema(map[string]any{
			"ticker": strSchema("the company or ticker"),
			"data":   strSchema("optional comma-separated paths to financial data"),
			"notes":  strSchema("optional context you already know"),
		}, "ticker"),
		run: func(env *Env, args map[string]any) (string, error) {
			return business.StockAnalysis(env.Router, strArg(args, "ticker"), splitPaths(strArg(args, "data")), strArg(args, "notes"))
		},
	})

	r.Register(funcTool{
		name: "plan_travel",
		desc: "Draft a travel itinerary and booking brief from a request. Does NOT book anything — outbound actions need human confirmation. Compares options if option data is provided. Args: request, optional options (comma-separated paths).",
		schema: objSchema(map[string]any{
			"request": strSchema("the trip: who, from/to, dates, preferences"),
			"options": strSchema("optional comma-separated paths to flight/hotel option data"),
		}, "request"),
		run: func(env *Env, args map[string]any) (string, error) {
			return business.TravelItinerary(env.Router, strArg(args, "request"), splitPaths(strArg(args, "options")))
		},
	})
}

func splitPaths(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
