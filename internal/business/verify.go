package business

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"github.com/pragun/brain/internal/sheet"
)

// Finance verification: the deterministic checks a bookkeeper runs before
// trusting a sheet. Every check is arithmetic done in Go — the whole value is
// that these findings are facts, not a model's impression. The model, if used
// at all downstream, only explains the findings.

// Check is one verification result.
type Check struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

// VerifyReport is the outcome of verifying a workbook.
type VerifyReport struct {
	Path     string  `json:"path"`
	Checks   []Check `json:"checks"`
	Failed   int     `json:"failed"`
	Warnings int     `json:"warnings"`
}

// Verify runs consistency checks over every sheet in a file: arithmetic
// relationships between columns, outliers, sign anomalies, blanks in numeric
// columns, duplicate rows, and column-sum vs a total row.
func Verify(path string) (VerifyReport, error) {
	book, err := sheet.Read(path)
	if err != nil {
		return VerifyReport{}, err
	}

	rep := VerifyReport{Path: path}
	add := func(c Check) {
		rep.Checks = append(rep.Checks, c)
		if !c.Passed {
			rep.Failed++
		}
	}

	for _, t := range book.Tables {
		checkColumnArithmetic(t, add)
		checkBlanksAndSigns(t, add)
		checkOutliers(t, add)
		checkDuplicateRows(t, add)
		checkTotalRow(t, add)
	}
	if len(rep.Checks) == 0 {
		add(Check{Name: "structure", Passed: true, Detail: "no numeric columns to verify"})
	}
	return rep, nil
}

// checkColumnArithmetic looks for a column whose name implies it is derived
// (net, total, profit, balance) and verifies it equals the sum or difference of
// the other numeric columns, row by row.
func checkColumnArithmetic(t sheet.Table, add func(Check)) {
	numeric := numericCols(t)
	if len(numeric) < 3 {
		return // need a derived column plus at least two inputs
	}

	derivedNames := []string{"net", "total", "profit", "balance", "gross", "sum"}
	for _, d := range numeric {
		low := strings.ToLower(t.Headers[d])
		isDerived := false
		for _, dn := range derivedNames {
			if strings.Contains(low, dn) {
				isDerived = true
			}
		}
		if !isDerived {
			continue
		}

		inputs := []int{}
		for _, c := range numeric {
			if c != d {
				inputs = append(inputs, c)
			}
		}

		sumOK, diffOK, checked := 0, 0, 0
		for _, row := range t.Rows {
			dv, ok := cellNum(row, d)
			if !ok {
				continue
			}
			var sum, first float64
			haveFirst := false
			valid := true
			for _, c := range inputs {
				v, ok := cellNum(row, c)
				if !ok {
					valid = false
					break
				}
				sum += v
				if !haveFirst {
					first = v
					haveFirst = true
				}
			}
			if !valid || !haveFirst {
				continue
			}
			checked++
			diff := first - (sum - first) // first minus the rest
			if approx(dv, sum) {
				sumOK++
			}
			if approx(dv, diff) {
				diffOK++
			}
		}
		if checked == 0 {
			continue
		}
		if sumOK == checked {
			add(Check{Name: t.Headers[d] + " = sum of columns", Passed: true,
				Detail: fmt.Sprintf("all %d rows reconcile as a sum", checked)})
		} else if diffOK == checked {
			add(Check{Name: t.Headers[d] + " = first − rest", Passed: true,
				Detail: fmt.Sprintf("all %d rows reconcile as a difference", checked)})
		} else if sumOK > 0 || diffOK > 0 {
			best := sumOK
			if diffOK > best {
				best = diffOK
			}
			add(Check{Name: t.Headers[d] + " reconciliation", Passed: false,
				Detail: fmt.Sprintf("%d of %d rows do not reconcile against the other columns", checked-best, checked)})
		}
	}
}

func checkBlanksAndSigns(t sheet.Table, add func(Check)) {
	for _, c := range numericCols(t) {
		blanks, neg, pos, total := 0, 0, 0, 0
		for _, row := range t.Rows {
			if c >= len(row) || strings.TrimSpace(row[c]) == "" {
				blanks++
				continue
			}
			v, ok := cellNum(row, c)
			if !ok {
				continue
			}
			total++
			if v < 0 {
				neg++
			} else if v > 0 {
				pos++
			}
		}
		if blanks > 0 {
			add(Check{Name: t.Headers[c] + " completeness", Passed: false,
				Detail: fmt.Sprintf("%d blank cell(s) in a numeric column", blanks)})
		}
		// A mostly-positive column with a minority of negatives is a classic
		// sign-entry error worth flagging (negatives no more than a third of
		// the positives).
		if total >= 5 && neg > 0 && pos >= 3*neg {
			add(Check{Name: t.Headers[c] + " sign", Passed: false,
				Detail: fmt.Sprintf("%d negative value(s) in an otherwise positive column", neg)})
		}
	}
}

func checkOutliers(t sheet.Table, add func(Check)) {
	for _, c := range numericCols(t) {
		var vals []float64
		for _, row := range t.Rows {
			if v, ok := cellNum(row, c); ok {
				vals = append(vals, v)
			}
		}
		if len(vals) < 5 {
			continue
		}
		mean, sd := meanStd(vals)
		if sd == 0 {
			continue
		}
		out := 0
		for _, v := range vals {
			if math.Abs(v-mean) > 3*sd {
				out++
			}
		}
		if out > 0 {
			add(Check{Name: t.Headers[c] + " outliers", Passed: false,
				Detail: fmt.Sprintf("%d value(s) beyond 3σ — verify they are not typos", out)})
		}
	}
}

func checkDuplicateRows(t sheet.Table, add func(Check)) {
	seen := map[string]int{}
	dupes := 0
	for _, row := range t.Rows {
		key := strings.Join(row, "\x00")
		seen[key]++
		if seen[key] == 2 {
			dupes++
		}
	}
	if dupes > 0 {
		add(Check{Name: "duplicate rows", Passed: false,
			Detail: fmt.Sprintf("%d exactly-duplicated row(s) — possible double entry", dupes)})
	}
}

// checkTotalRow verifies a trailing "total" row equals the column sums above it.
func checkTotalRow(t sheet.Table, add func(Check)) {
	if len(t.Rows) < 2 {
		return
	}
	last := t.Rows[len(t.Rows)-1]
	isTotal := false
	for _, cell := range last {
		l := strings.ToLower(strings.TrimSpace(cell))
		if l == "total" || l == "totals" || l == "sum" {
			isTotal = true
		}
	}
	if !isTotal {
		return
	}
	for _, c := range numericCols(t) {
		stated, ok := cellNum(last, c)
		if !ok {
			continue
		}
		var sum float64
		for _, row := range t.Rows[:len(t.Rows)-1] {
			if v, ok := cellNum(row, c); ok {
				sum += v
			}
		}
		if approx(stated, sum) {
			add(Check{Name: t.Headers[c] + " total row", Passed: true,
				Detail: fmt.Sprintf("stated total %.2f matches the column sum", stated)})
		} else {
			add(Check{Name: t.Headers[c] + " total row", Passed: false,
				Detail: fmt.Sprintf("stated total %.2f ≠ column sum %.2f (off by %.2f)", stated, sum, stated-sum)})
		}
	}
}

func (r VerifyReport) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Verified %s — %d check(s), %d failing\n\n", filepath.Base(r.Path), len(r.Checks), r.Failed)
	for _, c := range r.Checks {
		mark := "✓"
		if !c.Passed {
			mark = "✗"
		}
		fmt.Fprintf(&b, "  %s %s — %s\n", mark, c.Name, c.Detail)
	}
	return b.String()
}

// --- helpers ---

func numericCols(t sheet.Table) []int {
	var out []int
	for i, s := range t.Stats() {
		if s.Numeric {
			out = append(out, i)
		}
	}
	return out
}

func cellNum(row []string, c int) (float64, bool) {
	if c >= len(row) {
		return 0, false
	}
	return sheet.ParseNumber(row[c])
}

func approx(a, b float64) bool {
	return math.Abs(a-b) <= 0.01+0.001*math.Abs(b)
}

func meanStd(v []float64) (float64, float64) {
	if len(v) == 0 {
		return 0, 0
	}
	var sum float64
	for _, x := range v {
		sum += x
	}
	mean := sum / float64(len(v))
	var ss float64
	for _, x := range v {
		ss += (x - mean) * (x - mean)
	}
	return mean, math.Sqrt(ss / float64(len(v)))
}
