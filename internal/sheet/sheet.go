// Package sheet reads spreadsheets — xlsx, csv — into a common table, and
// computes the numbers a business assistant needs *deterministically*.
//
// The division of labour matters: small local models are unreliable at
// arithmetic, and a business assistant that reports a wrong total is worse than
// none. So every figure — sums, averages, growth, per-column stats — is
// computed here in Go. The model's only job downstream is to narrate numbers it
// is handed, never to derive them.
package sheet

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// Table is a single sheet: a header row and the rows beneath it.
type Table struct {
	Name    string
	Headers []string
	Rows    [][]string
}

// Book is a workbook — one or more tables (xlsx has many sheets; csv has one).
type Book struct {
	Path   string
	Tables []Table
}

// Read loads a spreadsheet by extension. Unknown extensions are tried as CSV,
// which is the common lowest-common-denominator export.
func Read(path string) (*Book, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".xlsx", ".xlsm":
		return readXLSX(path)
	default:
		return readCSV(path)
	}
}

func readCSV(path string) (*Book, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1 // ragged rows are common in exports; tolerate them
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if len(records) == 0 {
		return &Book{Path: path, Tables: []Table{{Name: base(path)}}}, nil
	}
	return &Book{Path: path, Tables: []Table{{
		Name: base(path), Headers: records[0], Rows: records[1:],
	}}}, nil
}

func readXLSX(path string) (*Book, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	book := &Book{Path: path}
	for _, name := range f.GetSheetList() {
		rows, err := f.GetRows(name)
		if err != nil || len(rows) == 0 {
			continue
		}
		book.Tables = append(book.Tables, Table{
			Name: name, Headers: rows[0], Rows: rows[1:],
		})
	}
	if len(book.Tables) == 0 {
		return nil, fmt.Errorf("%s has no readable sheets", path)
	}
	return book, nil
}

// ColumnStat is the deterministic summary of one numeric column.
type ColumnStat struct {
	Name    string
	Numeric bool
	Count   int
	Sum     float64
	Mean    float64
	Min     float64
	Max     float64
	// Growth is the change from the first to the last non-empty value as a
	// fraction, when the column reads as a series (e.g. monthly revenue).
	Growth  float64
	HasGrow bool
}

// Stats computes per-column statistics. A column counts as numeric when most of
// its non-empty cells parse as numbers — one stray label should not disqualify
// a revenue column.
func (t Table) Stats() []ColumnStat {
	out := make([]ColumnStat, len(t.Headers))
	for c, h := range t.Headers {
		out[c] = t.columnStat(c, h)
	}
	return out
}

func (t Table) columnStat(c int, name string) ColumnStat {
	s := ColumnStat{Name: name, Min: math.Inf(1), Max: math.Inf(-1)}
	var nums []float64
	nonEmpty := 0

	for _, row := range t.Rows {
		if c >= len(row) {
			continue
		}
		cell := strings.TrimSpace(row[c])
		if cell == "" {
			continue
		}
		nonEmpty++
		if v, ok := parseNumber(cell); ok {
			nums = append(nums, v)
		}
	}

	// Majority-numeric → treat as a number column.
	if nonEmpty == 0 || len(nums)*2 < nonEmpty {
		s.Numeric = false
		s.Count = nonEmpty
		s.Min, s.Max = 0, 0
		return s
	}

	s.Numeric = true
	s.Count = len(nums)
	for _, v := range nums {
		s.Sum += v
		s.Min = math.Min(s.Min, v)
		s.Max = math.Max(s.Max, v)
	}
	s.Mean = s.Sum / float64(len(nums))
	if len(nums) >= 2 && nums[0] != 0 {
		s.Growth = (nums[len(nums)-1] - nums[0]) / math.Abs(nums[0])
		s.HasGrow = true
	}
	return s
}

// parseNumber accepts the shapes money and spreadsheets actually use:
// "$1,234.50", "1,234", "(500)" for negative, "12%", plain floats.
func parseNumber(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	neg := false
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		neg = true
		s = s[1 : len(s)-1]
	}
	pct := strings.HasSuffix(s, "%")
	s = strings.TrimSuffix(s, "%")
	s = strings.NewReplacer("$", "", ",", "", "€", "", "£", "", " ", "").Replace(s)
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	if pct {
		v /= 100
	}
	if neg {
		v = -v
	}
	return v, true
}

// Digest renders a compact, numbers-first summary of a table for a model to
// narrate. It leads with the computed figures so the model paraphrases facts
// rather than inventing them, and caps the raw rows shown.
func (t Table) Digest(maxRows int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Sheet %q: %d rows × %d columns\n\n", t.Name, len(t.Rows), len(t.Headers))

	b.WriteString("Column statistics (computed):\n")
	for _, s := range t.Stats() {
		if s.Numeric {
			line := fmt.Sprintf("  %s: sum=%s mean=%s min=%s max=%s",
				s.Name, money(s.Sum), money(s.Mean), money(s.Min), money(s.Max))
			if s.HasGrow {
				line += fmt.Sprintf(" growth=%+.1f%%", s.Growth*100)
			}
			b.WriteString(line + "\n")
		} else {
			fmt.Fprintf(&b, "  %s: %d values (text)\n", s.Name, s.Count)
		}
	}

	b.WriteString("\nHeader: " + strings.Join(t.Headers, " | ") + "\n")
	for i, row := range t.Rows {
		if i >= maxRows {
			fmt.Fprintf(&b, "… (%d more rows)\n", len(t.Rows)-maxRows)
			break
		}
		b.WriteString("  " + strings.Join(row, " | ") + "\n")
	}
	return b.String()
}

// TopBy returns the rows with the largest values in a named numeric column —
// "biggest expenses", "top accounts". Empty column or non-numeric returns nil.
func (t Table) TopBy(column string, n int) []([]string) {
	c := t.columnIndex(column)
	if c < 0 {
		return nil
	}
	type ranked struct {
		row []string
		v   float64
	}
	var rs []ranked
	for _, row := range t.Rows {
		if c >= len(row) {
			continue
		}
		if v, ok := parseNumber(row[c]); ok {
			rs = append(rs, ranked{row, v})
		}
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i].v > rs[j].v })
	var out [][]string
	for i := 0; i < n && i < len(rs); i++ {
		out = append(out, rs[i].row)
	}
	return out
}

func (t Table) columnIndex(name string) int {
	name = strings.ToLower(strings.TrimSpace(name))
	for i, h := range t.Headers {
		if strings.ToLower(strings.TrimSpace(h)) == name {
			return i
		}
	}
	return -1
}

func money(v float64) string {
	if v == math.Trunc(v) && math.Abs(v) < 1e15 {
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
	return strconv.FormatFloat(v, 'f', 2, 64)
}

func base(path string) string {
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}
