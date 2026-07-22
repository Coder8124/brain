package business

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func csvFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "d.csv")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func failedNames(r VerifyReport) map[string]bool {
	m := map[string]bool{}
	for _, c := range r.Checks {
		if !c.Passed {
			m[c.Name] = true
		}
	}
	return m
}

func TestVerifyReconcilesNetColumn(t *testing.T) {
	// net = revenue - expenses, all rows consistent → should pass.
	r, err := Verify(csvFile(t, "month,revenue,expenses,net\nJan,100,60,40\nFeb,200,120,80\nMar,150,50,100\n"))
	if err != nil {
		t.Fatal(err)
	}
	if r.Failed != 0 {
		t.Errorf("clean sheet should have no failures, got %d: %+v", r.Failed, r.Checks)
	}
}

func TestVerifyCatchesBrokenArithmetic(t *testing.T) {
	// net is wrong on the last row (should be 100, says 999).
	r, _ := Verify(csvFile(t, "month,revenue,expenses,net\nJan,100,60,40\nFeb,200,120,80\nMar,150,50,999\n"))
	if r.Failed == 0 {
		t.Error("a broken net column should fail reconciliation")
	}
}

func TestVerifyCatchesTotalRowMismatch(t *testing.T) {
	r, _ := Verify(csvFile(t, "item,amount\nA,100\nB,200\nTotal,999\n"))
	if !failedNames(r)["amount total row"] {
		t.Errorf("a wrong total row should be flagged: %+v", r.Checks)
	}
}

func TestVerifyPassesCorrectTotalRow(t *testing.T) {
	r, _ := Verify(csvFile(t, "item,amount\nA,100\nB,200\nTotal,300\n"))
	if r.Failed != 0 {
		t.Errorf("a correct total row should pass, got failures: %+v", r.Checks)
	}
}

func TestVerifyFlagsSignAnomaly(t *testing.T) {
	// One negative revenue among many positives.
	r, _ := Verify(csvFile(t, "m,revenue\na,100\nb,120\nc,-50\nd,130\ne,110\nf,105\n"))
	if !failedNames(r)["revenue sign"] {
		t.Errorf("a stray negative should be flagged: %+v", r.Checks)
	}
}

func TestVerifyFlagsDuplicateRows(t *testing.T) {
	r, _ := Verify(csvFile(t, "item,amount\nA,100\nB,200\nA,100\n"))
	if !failedNames(r)["duplicate rows"] {
		t.Errorf("a duplicate row should be flagged: %+v", r.Checks)
	}
}

func TestForecastCAGRIsComputed(t *testing.T) {
	// 100 → 200 over 1 step is 100% growth; project one more → 400.
	p, err := Forecast([]float64{100, 200}, 1, "cagr")
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(p.CAGR-1.0) > 1e-9 {
		t.Errorf("cagr = %v, want 1.0", p.CAGR)
	}
	if len(p.Forecast) != 1 || math.Abs(p.Forecast[0]-400) > 0.01 {
		t.Errorf("forecast = %v, want [400]", p.Forecast)
	}
}

func TestForecastLinearIsComputed(t *testing.T) {
	// 100,150,200 → linear delta 50; two periods → 250, 300.
	p, _ := Forecast([]float64{100, 150, 200}, 2, "linear")
	if math.Abs(p.LinearDelta-50) > 1e-9 {
		t.Errorf("linear delta = %v, want 50", p.LinearDelta)
	}
	if len(p.Forecast) != 2 || p.Forecast[0] != 250 || p.Forecast[1] != 300 {
		t.Errorf("forecast = %v, want [250 300]", p.Forecast)
	}
}

func TestForecastNeedsHistory(t *testing.T) {
	if _, err := Forecast([]float64{100}, 3, "cagr"); err == nil {
		t.Error("forecasting from a single point should error")
	}
}
