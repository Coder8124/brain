package router

import (
	"fmt"
	"regexp"
	"strings"
)

// The redaction gate is the precondition for offering cloud at all. Nothing
// reaches a third party because a config default was wrong: T3 is off until the
// user has seen exactly what would be sent.

type Finding struct {
	Kind  string // email, phone, key, path, url
	Value string
	Start int
	End   int
}

var detectors = []struct {
	kind string
	re   *regexp.Regexp
}{
	{"email", regexp.MustCompile(`[\w.+-]+@[\w-]+\.[\w.]{2,}`)},
	// Long random-looking tokens: API keys, bearer tokens, private key bodies.
	{"key", regexp.MustCompile(`\b(?:sk|pk|ghp|gho|xox[baprs])-[A-Za-z0-9_\-]{16,}\b`)},
	{"key", regexp.MustCompile(`\b[A-Za-z0-9_\-]{40,}\b`)},
	{"phone", regexp.MustCompile(`\+?\d[\d\s().-]{8,}\d`)},
	{"path", regexp.MustCompile(`/Users/[\w.-]+`)},
	{"url", regexp.MustCompile(`https?://[^\s<>"]+`)},
}

// Scan finds candidates worth showing the user before egress. It deliberately
// over-reports: a false positive costs one glance, a false negative leaks.
func Scan(text string) []Finding {
	var out []Finding
	seen := map[string]bool{}

	for _, d := range detectors {
		for _, loc := range d.re.FindAllStringIndex(text, -1) {
			val := text[loc[0]:loc[1]]
			key := d.kind + "\x00" + val
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, Finding{Kind: d.kind, Value: val, Start: loc[0], End: loc[1]})
		}
	}
	return out
}

// Redact replaces the given findings with typed placeholders. Placeholders are
// stable per value, so a note mentioning one person twice stays coherent to the
// model rather than reading as two different people.
func Redact(text string, findings []Finding) string {
	counters := map[string]int{}
	assigned := map[string]string{}

	out := text
	for _, f := range findings {
		if _, ok := assigned[f.Value]; !ok {
			counters[f.Kind]++
			assigned[f.Value] = fmt.Sprintf("[%s_%d]", strings.ToUpper(f.Kind), counters[f.Kind])
		}
		out = strings.ReplaceAll(out, f.Value, assigned[f.Value])
	}
	return out
}

// Preview renders what the user sees before approving egress for a tier.
func Preview(text string, findings []Finding, maxChars int) string {
	var b strings.Builder

	fmt.Fprintf(&b, "About to send %d characters to a third party.\n", len(text))
	if len(findings) == 0 {
		b.WriteString("No sensitive patterns detected — but read it anyway.\n\n")
	} else {
		fmt.Fprintf(&b, "%d sensitive pattern(s) detected:\n", len(findings))
		byKind := map[string]int{}
		for _, f := range findings {
			byKind[f.Kind]++
		}
		for kind, n := range byKind {
			fmt.Fprintf(&b, "  %-6s %d\n", kind, n)
		}
		b.WriteString("\n")
	}

	body := Redact(text, findings)
	if len(body) > maxChars {
		body = body[:maxChars] + "\n… truncated"
	}
	b.WriteString("─── payload after redaction ───\n")
	b.WriteString(body)
	b.WriteString("\n")
	return b.String()
}
