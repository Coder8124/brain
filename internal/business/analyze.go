package business

import (
	"fmt"
	"strings"

	"github.com/pragun/brain/internal/router"
	"github.com/pragun/brain/internal/sheet"
)

// Spreadsheet analysis. The figures are computed in internal/sheet (Go); this
// layer only asks the model to explain what those figures mean for the
// business. The prompt is emphatic that every number comes from the provided
// statistics — a finance assistant that invents a total is a liability, so the
// same discipline as the diagnostic applies: compute, then narrate.

// AnalyzeFile reads a spreadsheet and returns a narrated analysis grounded in
// the deterministically-computed statistics. question is optional; empty asks
// for a general read.
func AnalyzeFile(rt *router.Router, path, question string) (string, error) {
	book, err := sheet.Read(path)
	if err != nil {
		return "", err
	}

	var digest strings.Builder
	for _, t := range book.Tables {
		digest.WriteString(t.Digest(25))
		digest.WriteString("\n")
	}

	model, err := rt.Model(router.T2)
	if err != nil {
		return "", err
	}

	system := "You are a finance and operations analyst reading a spreadsheet. The column " +
		"statistics below were computed exactly; treat them as ground truth and quote them. " +
		"Do NOT compute new figures or estimate — if a number is not in the statistics, say it is " +
		"not available rather than guessing. Report what matters: totals, trends and their " +
		"direction, notable outliers, and anything that needs attention. Lead with the single most " +
		"important finding. Be concise."

	prompt := digest.String()
	if question != "" {
		prompt = "Question: " + question + "\n\n" + prompt
	}
	return rt.Local().Chat(model, system, prompt, nil)
}

// FileSummary is the structured, model-free read of a workbook, for the agent
// and the UI. Every field is computed, nothing inferred.
type FileSummary struct {
	Path   string         `json:"path"`
	Sheets []SheetSummary `json:"sheets"`
}

type SheetSummary struct {
	Name    string          `json:"name"`
	Rows    int             `json:"rows"`
	Columns int             `json:"columns"`
	Numeric []NumericColumn `json:"numeric"`
}

type NumericColumn struct {
	Name   string  `json:"name"`
	Sum    float64 `json:"sum"`
	Mean   float64 `json:"mean"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Growth float64 `json:"growth"`
}

// Summarize returns the computed shape of a workbook without any model call —
// instant, exact, and what the agent harness inspects before deciding what to
// do with a file.
func Summarize(path string) (FileSummary, error) {
	book, err := sheet.Read(path)
	if err != nil {
		return FileSummary{}, err
	}
	out := FileSummary{Path: path}
	for _, t := range book.Tables {
		ss := SheetSummary{Name: t.Name, Rows: len(t.Rows), Columns: len(t.Headers)}
		for _, s := range t.Stats() {
			if s.Numeric {
				ss.Numeric = append(ss.Numeric, NumericColumn{
					Name: s.Name, Sum: s.Sum, Mean: s.Mean, Min: s.Min, Max: s.Max, Growth: s.Growth,
				})
			}
		}
		out.Sheets = append(out.Sheets, ss)
	}
	return out, nil
}

func (f FileSummary) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", f.Path)
	for _, s := range f.Sheets {
		fmt.Fprintf(&b, "  sheet %q: %d rows × %d cols\n", s.Name, s.Rows, s.Columns)
		for _, n := range s.Numeric {
			fmt.Fprintf(&b, "    %s: sum=%.2f mean=%.2f min=%.2f max=%.2f growth=%+.1f%%\n",
				n.Name, n.Sum, n.Mean, n.Min, n.Max, n.Growth*100)
		}
	}
	return b.String()
}
