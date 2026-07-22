package business

import (
	"fmt"
	"math"
	"strings"

	"github.com/pragun/brain/internal/router"
	"github.com/pragun/brain/internal/sheet"
)

// Forecasting margin, revenue, and profit. The projection is computed in Go —
// the model is never asked to extrapolate a number, because a model that
// invents next quarter's revenue is worse than a blank cell. Two standard
// methods are computed from the history and the caller picks; the model, if
// used, only explains the projection and its assumptions.

// Projection is a computed forecast of a series.
type Projection struct {
	Column   string    `json:"column"`
	History  []float64 `json:"history"`
	Periods  int       `json:"periods"`
	Method   string    `json:"method"` // "cagr" or "linear"
	Rate     float64   `json:"rate"`   // per-period growth (cagr) or delta (linear)
	Forecast []float64 `json:"forecast"`
	// CAGR and LinearDelta are both computed so a caller can compare methods.
	CAGR        float64 `json:"cagr"`
	LinearDelta float64 `json:"linear_delta"`
}

// Forecast projects a numeric series forward `periods` steps. method is "cagr"
// (compound growth — the right default for revenue/profit that grow by a rate)
// or "linear" (constant absolute change — right for steady, additive series).
func Forecast(history []float64, periods int, method string) (Projection, error) {
	if len(history) < 2 {
		return Projection{}, fmt.Errorf("need at least two historical points to forecast, got %d", len(history))
	}
	if periods < 1 {
		periods = 1
	}

	p := Projection{History: history, Periods: periods, Method: method}
	last := history[len(history)-1]
	first := history[0]
	n := float64(len(history) - 1)

	// CAGR: (last/first)^(1/n) − 1, defined only for same-signed positive series.
	if first > 0 && last > 0 {
		p.CAGR = math.Pow(last/first, 1/n) - 1
	}
	// Linear: average step.
	p.LinearDelta = (last - first) / n

	switch method {
	case "linear":
		p.Rate = p.LinearDelta
		v := last
		for i := 0; i < periods; i++ {
			v += p.LinearDelta
			p.Forecast = append(p.Forecast, round2(v))
		}
	default: // cagr
		p.Method = "cagr"
		p.Rate = p.CAGR
		v := last
		for i := 0; i < periods; i++ {
			v *= 1 + p.CAGR
			p.Forecast = append(p.Forecast, round2(v))
		}
	}
	return p, nil
}

// ForecastFile forecasts a named column from a spreadsheet.
func ForecastFile(path, column string, periods int, method string) (Projection, error) {
	book, err := sheet.Read(path)
	if err != nil {
		return Projection{}, err
	}
	for _, t := range book.Tables {
		if series, ok := t.NumericSeries(column); ok {
			p, err := Forecast(series, periods, method)
			p.Column = column
			return p, err
		}
	}
	return Projection{}, fmt.Errorf("no numeric column %q found in %s", column, path)
}

func (p Projection) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Forecast of %s (%s, %d periods)\n", p.Column, p.Method, p.Periods)
	fmt.Fprintf(&b, "  history: %s\n", floatsStr(p.History))
	fmt.Fprintf(&b, "  cagr %+.1f%%/period · linear %+.2f/period\n", p.CAGR*100, p.LinearDelta)
	fmt.Fprintf(&b, "  forecast: %s\n", floatsStr(p.Forecast))
	return b.String()
}

// Narrate asks the model to explain a computed projection — its trajectory,
// the assumption behind the method, and the risk. It is handed the numbers and
// told not to change them.
func (p Projection) Narrate(rt *router.Router) (string, error) {
	model, err := rt.Model(router.T2)
	if err != nil {
		return "", err
	}
	system := "You are a financial analyst explaining a computed forecast. The history, growth " +
		"rate, and projected values below were calculated exactly — quote them and do not change " +
		"or recompute any figure. Explain the trajectory, state the assumption the chosen method " +
		"makes, and name the main risk to the forecast. A forecast is a projection, not a promise — " +
		"say so. Be concise."
	return rt.Local().Chat(model, system, p.String(), nil)
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

func floatsStr(v []float64) string {
	parts := make([]string, len(v))
	for i, x := range v {
		parts[i] = money(x)
	}
	return strings.Join(parts, ", ")
}

func money(v float64) string {
	if v == math.Trunc(v) && math.Abs(v) < 1e15 {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.2f", v)
}
