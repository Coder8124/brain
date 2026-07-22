package sheet

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func writeCSV(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "data.csv")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReadsCSVWithHeaders(t *testing.T) {
	b, err := Read(writeCSV(t, "month,revenue\nJan,100\nFeb,150\nMar,225\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Tables) != 1 {
		t.Fatalf("want 1 table, got %d", len(b.Tables))
	}
	tbl := b.Tables[0]
	if len(tbl.Headers) != 2 || tbl.Headers[1] != "revenue" {
		t.Errorf("headers = %v", tbl.Headers)
	}
	if len(tbl.Rows) != 3 {
		t.Errorf("rows = %d, want 3", len(tbl.Rows))
	}
}

func TestNumericStatsAreComputedNotGuessed(t *testing.T) {
	b, _ := Read(writeCSV(t, "month,revenue\nJan,100\nFeb,150\nMar,225\n"))
	stats := b.Tables[0].Stats()

	var rev ColumnStat
	for _, s := range stats {
		if s.Name == "revenue" {
			rev = s
		}
	}
	if !rev.Numeric {
		t.Fatal("revenue should be detected as numeric")
	}
	if rev.Sum != 475 {
		t.Errorf("sum = %v, want 475", rev.Sum)
	}
	if math.Abs(rev.Mean-158.333) > 0.01 {
		t.Errorf("mean = %v, want ~158.33", rev.Mean)
	}
	if rev.Min != 100 || rev.Max != 225 {
		t.Errorf("min/max = %v/%v, want 100/225", rev.Min, rev.Max)
	}
	// Growth Jan→Mar = (225-100)/100 = 1.25
	if !rev.HasGrow || math.Abs(rev.Growth-1.25) > 1e-9 {
		t.Errorf("growth = %v, want 1.25", rev.Growth)
	}
}

func TestParsesMessyMoneyFormats(t *testing.T) {
	cases := map[string]float64{
		"$1,234.50": 1234.50,
		"1,000":     1000,
		"(500)":     -500, // accounting negative
		"12%":       0.12,
		"€2.5":      2.5,
		"  42 ":     42,
	}
	for in, want := range cases {
		got, ok := parseNumber(in)
		if !ok || math.Abs(got-want) > 1e-9 {
			t.Errorf("parseNumber(%q) = %v,%v; want %v", in, got, ok, want)
		}
	}
	if _, ok := parseNumber("not a number"); ok {
		t.Error("a label must not parse as a number")
	}
}

func TestMostlyTextColumnIsNotNumeric(t *testing.T) {
	b, _ := Read(writeCSV(t, "name,note\nAcme,paid\nBeta,42\nGamma,pending\n"))
	for _, s := range b.Tables[0].Stats() {
		if s.Name == "note" && s.Numeric {
			t.Error("a column that is mostly text must not be treated as numeric")
		}
	}
}

func TestTopByRanksDescending(t *testing.T) {
	b, _ := Read(writeCSV(t, "vendor,amount\nA,100\nB,900\nC,300\n"))
	top := b.Tables[0].TopBy("amount", 2)
	if len(top) != 2 || top[0][0] != "B" || top[1][0] != "C" {
		t.Errorf("TopBy = %v, want B then C", top)
	}
}

func TestDigestLeadsWithComputedNumbers(t *testing.T) {
	b, _ := Read(writeCSV(t, "month,revenue\nJan,100\nFeb,150\nMar,225\n"))
	d := b.Tables[0].Digest(10)
	// The digest must contain the computed sum so the model narrates it rather
	// than deriving it.
	if !contains(d, "sum=475") {
		t.Errorf("digest should include the computed sum:\n%s", d)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
