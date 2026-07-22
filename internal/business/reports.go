package business

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pragun/brain/internal/router"
	"github.com/pragun/brain/internal/sheet"
)

// The generative business deliverables — presentations, expense reports,
// competitor and stock analyses, travel itineraries. They are model-written,
// but grounded: anything numeric is computed first (in Go) and handed to the
// model as fact, and every deliverable is built only from data the user
// provided. Where a deliverable would touch money or advice, it carries the
// honest caveat rather than pretending to certainty the tool cannot have.

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

func loadContext(paths []string) (string, error) {
	var b strings.Builder
	for _, p := range paths {
		switch strings.ToLower(filepath.Ext(p)) {
		case ".xlsx", ".xlsm", ".csv":
			book, err := sheet.Read(p)
			if err != nil {
				return "", err
			}
			for _, t := range book.Tables {
				b.WriteString(t.Digest(30))
				b.WriteString("\n")
			}
		default:
			data, err := os.ReadFile(p)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(&b, "--- %s ---\n%s\n\n", filepath.Base(p), string(data))
		}
	}
	return b.String(), nil
}

// ---- presentations ----

// Slide is one slide: a title and a few bullets, with optional speaker notes.
type Slide struct {
	Title   string   `json:"title"`
	Bullets []string `json:"bullets"`
	Notes   string   `json:"notes"`
}

var deckSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"title": map[string]any{"type": "string"},
		"slides": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"title":   map[string]any{"type": "string"},
					"bullets": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"notes":   map[string]any{"type": "string"},
				},
				"required":             []string{"title", "bullets"},
				"additionalProperties": false,
			},
		},
	},
	"required":             []string{"title", "slides"},
	"additionalProperties": false,
}

// Deck is a generated presentation.
type Deck struct {
	Title  string  `json:"title"`
	Slides []Slide `json:"slides"`
}

// Presentation drafts a slide deck on a topic, grounded in any data files or
// vault context provided. Returns the deck and its markdown rendering.
func Presentation(rt *router.Router, topic string, dataPaths []string, context string) (Deck, string, error) {
	model, err := rt.ModelFor(router.T2, true)
	if err != nil {
		return Deck{}, "", err
	}

	fileCtx, err := loadContext(dataPaths)
	if err != nil {
		return Deck{}, "", err
	}

	system := "You are preparing a business presentation. Produce a tight deck (6-10 slides): a " +
		"title slide, then content slides each with a clear title and 3-5 concise bullets, ending " +
		"with next steps. Reply with JSON only. Ground every claim and figure in the provided " +
		"material; the figures given are exact — quote them, do not invent new ones."

	prompt := "Topic: " + topic
	if context != "" {
		prompt += "\n\nContext:\n" + context
	}
	if fileCtx != "" {
		prompt += "\n\nData (computed figures are exact):\n" + fileCtx
	}

	out, err := rt.Local().Chat(model, system, prompt, deckSchema)
	if err != nil {
		return Deck{}, "", err
	}
	var d Deck
	if err := json.Unmarshal([]byte(cleanJSON(out)), &d); err != nil {
		return Deck{}, "", fmt.Errorf("presentation did not parse: %w", err)
	}
	return d, d.Markdown(), nil
}

// Markdown renders a deck as one Markdown file, slides separated by `---` (the
// convention Marp, reveal.js and Obsidian slides all read).
func (d Deck) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "---\nmarp: true\ntitle: %s\n---\n\n# %s\n\n", d.Title, d.Title)
	for _, s := range d.Slides {
		b.WriteString("---\n\n## " + s.Title + "\n\n")
		for _, bl := range s.Bullets {
			b.WriteString("- " + bl + "\n")
		}
		if s.Notes != "" {
			b.WriteString("\n<!-- " + s.Notes + " -->\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// ---- expense reports ----

// ExpenseReport builds a categorised expense report from a spreadsheet: totals
// per category computed in Go, then a written summary. amountCol and categoryCol
// name the columns; empty lets it guess from common header names.
func ExpenseReport(rt *router.Router, path, categoryCol, amountCol string) (string, error) {
	book, err := sheet.Read(path)
	if err != nil {
		return "", err
	}
	if len(book.Tables) == 0 {
		return "", fmt.Errorf("no data in %s", path)
	}
	t := book.Tables[0]

	catIdx := columnGuess(t, categoryCol, []string{"category", "type", "class", "account", "description"})
	amtIdx := columnGuess(t, amountCol, []string{"amount", "cost", "total", "expense", "price", "usd"})
	if amtIdx < 0 {
		return "", fmt.Errorf("could not find an amount column; pass one explicitly")
	}

	// Totals per category — computed, exact.
	totals := map[string]float64{}
	order := []string{}
	var grand float64
	for _, row := range t.Rows {
		amt, ok := cellNum(row, amtIdx)
		if !ok {
			continue
		}
		cat := "uncategorised"
		if catIdx >= 0 && catIdx < len(row) && strings.TrimSpace(row[catIdx]) != "" {
			cat = strings.TrimSpace(row[catIdx])
		}
		if _, seen := totals[cat]; !seen {
			order = append(order, cat)
		}
		totals[cat] += amt
		grand += amt
	}

	var computed strings.Builder
	fmt.Fprintf(&computed, "Total: %s across %d line items\n", money(grand), len(t.Rows))
	for _, cat := range order {
		fmt.Fprintf(&computed, "  %s: %s (%.0f%%)\n", cat, money(totals[cat]), totals[cat]/grand*100)
	}

	model, err := rt.Model(router.T2)
	if err != nil {
		// Even without a model the computed report is useful.
		return "# Expense report\n\n" + computed.String(), nil
	}
	system := "You are writing an expense report from computed category totals. The totals below " +
		"are exact — quote them and do not change them. Produce a short professional report: the " +
		"grand total, the breakdown by category with the largest first, and one or two notes on " +
		"where the spend concentrated. Markdown."
	written, err := rt.Local().Chat(model, system, computed.String(), nil)
	if err != nil {
		return "# Expense report\n\n" + computed.String(), nil
	}
	return strings.TrimSpace(written), nil
}

// ---- competitor analysis ----

// CompetitorAnalysis synthesises a structured comparison from provided data —
// files and/or a free-text brief. It is explicitly grounded: it will not add
// competitors or facts that are not in the material.
func CompetitorAnalysis(rt *router.Router, brief string, dataPaths []string) (string, error) {
	model, err := rt.Model(router.T2)
	if err != nil {
		return "", err
	}
	fileCtx, err := loadContext(dataPaths)
	if err != nil {
		return "", err
	}

	system := "You are a market analyst writing a competitor analysis strictly from the material " +
		"provided. Do not introduce competitors, numbers, or claims that are not in the material — " +
		"if something is unknown, say it is not in the data. Produce: a short comparison of each " +
		"competitor's strengths and weaknesses, their positioning, and where the opportunity is. " +
		"Markdown."

	prompt := brief
	if fileCtx != "" {
		prompt += "\n\nProvided data:\n" + fileCtx
	}
	if strings.TrimSpace(prompt) == "" {
		return "", fmt.Errorf("nothing to analyse — provide a brief or data files")
	}
	out, err := rt.Local().Chat(model, system, prompt, nil)
	return strings.TrimSpace(out), err
}

// ---- stock analysis ----

// StockAnalysis reads provided financial/company data and lays out
// considerations — never a live-market call, and always with the disclaimer
// that it is not financial advice. There is no market data feed; a local model
// inventing buy/sell calls from nothing would be worse than useless, so this is
// strictly a structured read of what the user supplied.
func StockAnalysis(rt *router.Router, ticker string, dataPaths []string, notes string) (string, error) {
	model, err := rt.Model(router.T2)
	if err != nil {
		return "", err
	}
	fileCtx, err := loadContext(dataPaths)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(fileCtx) == "" && strings.TrimSpace(notes) == "" {
		return "", fmt.Errorf("provide financial data or notes — this analyses supplied data, it has no market feed")
	}

	system := "You are analysing a company from data the user supplied. Work ONLY from that data; " +
		"you have no live market prices and must not pretend to. Lay out the case for and against, " +
		"the key figures (quoting them exactly), and what a reader would want to verify. Do NOT give " +
		"a buy/sell/hold instruction. End with this exact line: 'This is analysis of supplied data, " +
		"not financial advice.' Markdown."

	prompt := "Subject: " + ticker
	if notes != "" {
		prompt += "\nNotes: " + notes
	}
	if fileCtx != "" {
		prompt += "\n\nSupplied data:\n" + fileCtx
	}
	out, err := rt.Local().Chat(model, system, prompt, nil)
	if err != nil {
		return "", err
	}
	res := strings.TrimSpace(out)
	// Enforce the disclaimer even if the model drops it.
	if !strings.Contains(strings.ToLower(res), "not financial advice") {
		res += "\n\n_This is analysis of supplied data, not financial advice._"
	}
	return res, nil
}

// ---- travel ----

// TravelItinerary drafts an itinerary and a booking brief from a request. It
// does NOT book anything — there is no booking API, and an outbound action like
// spending money belongs behind an explicit human confirmation regardless. The
// output is a plan the user (or a booking tool, later) acts on.
func TravelItinerary(rt *router.Router, request string, optionsData []string) (string, error) {
	model, err := rt.Model(router.T2)
	if err != nil {
		return "", err
	}
	fileCtx, err := loadContext(optionsData)
	if err != nil {
		return "", err
	}

	system := "You are a travel-planning assistant drafting an itinerary and a booking brief from " +
		"the request. If flight/hotel options are provided, compare them on price and convenience " +
		"using only the given figures; otherwise lay out the plan and list exactly what needs to be " +
		"booked (with placeholders for prices you do not have). Make clear this is a draft to be " +
		"confirmed and booked by the user — you are not booking anything. Markdown."

	prompt := "Request: " + request
	if fileCtx != "" {
		prompt += "\n\nOptions data:\n" + fileCtx
	}
	out, err := rt.Local().Chat(model, system, prompt, nil)
	if err != nil {
		return "", err
	}
	res := strings.TrimSpace(out)
	return res + "\n\n_Draft itinerary — nothing has been booked. Confirm details and prices before booking._", nil
}

// columnGuess resolves a column by explicit name, else by trying common header
// synonyms.
func columnGuess(t sheet.Table, explicit string, synonyms []string) int {
	if explicit != "" {
		for i, h := range t.Headers {
			if strings.EqualFold(strings.TrimSpace(h), explicit) {
				return i
			}
		}
	}
	for i, h := range t.Headers {
		low := strings.ToLower(strings.TrimSpace(h))
		for _, syn := range synonyms {
			if strings.Contains(low, syn) {
				return i
			}
		}
	}
	return -1
}

// SaveDeliverable writes a generated markdown deliverable into the vault's
// business/ folder and returns its path, so reports become part of the vault.
func SaveDeliverable(vault, kind, title, markdown string) (string, error) {
	dir := filepath.Join(vault, "business", kind)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, slugify(title)+".md")
	if err := os.WriteFile(path, []byte(markdown), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func slugify(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "untitled"
	}
	return out
}
