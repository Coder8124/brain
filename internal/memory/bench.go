package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/pragun/brain/internal/provider"
)

// Benchmarking our memory against LongMemEval (ICLR 2025), the standard
// long-term memory benchmark for chat assistants.
//
// We measure retrieval recall — the metric that isolates the memory backend:
// given a question, did we surface the session that actually holds the answer?
// Each instance's chat history is loaded into a fresh memory store (one entry
// per session, embedded), then the question is used to recall the top-k; a hit
// is when a recalled entry comes from one of the evidence sessions. Reported
// per LongMemEval category, so the knowledge-update and temporal cases are
// visible separately.

// lmeInstance mirrors the LongMemEval JSON shape.
type lmeInstance struct {
	QuestionID       string      `json:"question_id"`
	QuestionType     string      `json:"question_type"`
	Question         string      `json:"question"`
	Answer           any         `json:"answer"`
	HaystackIDs      []string    `json:"haystack_session_ids"`
	HaystackSessions [][]lmeTurn `json:"haystack_sessions"`
	AnswerSessionIDs []string    `json:"answer_session_ids"`
}

type lmeTurn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// BenchResult is the outcome for one category (or overall).
type BenchResult struct {
	Category string
	N        int
	Hits     int
}

func (r BenchResult) Recall() float64 {
	if r.N == 0 {
		return 0
	}
	return float64(r.Hits) / float64(r.N)
}

// RunLongMemEval evaluates retrieval recall@k over up to `limit` instances of a
// LongMemEval file. progress, if non-nil, is called per instance.
func RunLongMemEval(p *provider.Provider, embedModel, path string, k, limit int, hybrid bool, progress func(done, total int)) ([]BenchResult, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var instances []lmeInstance
	if err := json.Unmarshal(raw, &instances); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	// Abstention questions reference events that never happened — LongMemEval
	// excludes them from retrieval scoring, and so do we.
	filtered := instances[:0]
	for _, in := range instances {
		if !strings.HasSuffix(in.QuestionType, "_abs") && len(in.AnswerSessionIDs) > 0 {
			filtered = append(filtered, in)
		}
	}
	instances = filtered
	// Stride-sample across the whole file rather than taking a prefix — the
	// instances are grouped by category, so a prefix would test only one type.
	// Striding gives every category proportional representation.
	if limit > 0 && limit < len(instances) {
		step := len(instances) / limit
		if step < 1 {
			step = 1
		}
		sampled := make([]lmeInstance, 0, limit)
		for i := 0; i < len(instances) && len(sampled) < limit; i += step {
			sampled = append(sampled, instances[i])
		}
		instances = sampled
	}

	byCat := map[string]*BenchResult{}
	overall := &BenchResult{Category: "OVERALL"}

	for i, in := range instances {
		hit, err := scoreInstance(p, embedModel, in, k, hybrid)
		if err != nil {
			return nil, err
		}
		cat := byCat[in.QuestionType]
		if cat == nil {
			cat = &BenchResult{Category: in.QuestionType}
			byCat[in.QuestionType] = cat
		}
		cat.N++
		overall.N++
		if hit {
			cat.Hits++
			overall.Hits++
		}
		if progress != nil {
			progress(i+1, len(instances))
		}
	}

	out := []BenchResult{*overall}
	var cats []string
	for c := range byCat {
		cats = append(cats, c)
	}
	sort.Strings(cats)
	for _, c := range cats {
		out = append(out, *byCat[c])
	}
	return out, nil
}

// scoreInstance loads one instance's sessions into a throwaway store and checks
// whether the top-k recall for its question includes an evidence session.
func scoreInstance(p *provider.Provider, embedModel string, in lmeInstance, k int, hybrid bool) (bool, error) {
	// Concatenate each session into one document, keyed by its session id — the
	// answer session ids share the id's prefix, so we normalise both to compare.
	texts := make([]string, 0, len(in.HaystackSessions))
	ids := make([]string, 0, len(in.HaystackSessions))
	for si, sess := range in.HaystackSessions {
		var b strings.Builder
		for _, turn := range sess {
			b.WriteString(turn.Role)
			b.WriteString(": ")
			b.WriteString(turn.Content)
			b.WriteString("\n")
		}
		id := ""
		if si < len(in.HaystackIDs) {
			id = in.HaystackIDs[si]
		}
		texts = append(texts, b.String())
		ids = append(ids, id)
	}

	// Embed all sessions in one batch, then the query, and rank by cosine — the
	// same retrieval our memory recall uses, run over these sessions directly so
	// the benchmark measures the backend, not the LLM extraction.
	sessVecs, err := p.Embed(embedModel, truncateAll(texts, 2000))
	if err != nil {
		return false, err
	}
	qVec, err := p.Embed(embedModel, []string{in.Question})
	if err != nil || len(qVec) == 0 {
		return false, err
	}

	// Hybrid rank: vector similarity fused with BM25 lexical, so a session that
	// shares the answer's exact terms is caught even when the embedding blurs it.
	var top []string
	if hybrid {
		cands := make([]Candidate, len(sessVecs))
		for i := range sessVecs {
			cands[i] = Candidate{ID: ids[i], Text: texts[i], Vec: sessVecs[i]}
		}
		top = HybridRank(in.Question, qVec[0], cands, k)
	} else {
		type sc struct {
			id  string
			sim float64
		}
		r := make([]sc, len(sessVecs))
		for i := range sessVecs {
			r[i] = sc{ids[i], cosine(qVec[0], sessVecs[i])}
		}
		sort.Slice(r, func(a, b int) bool { return r[a].sim > r[b].sim })
		for i := 0; i < k && i < len(r); i++ {
			top = append(top, r[i].id)
		}
	}

	evidence := map[string]bool{}
	for _, e := range in.AnswerSessionIDs {
		evidence[normalizeSessionID(e)] = true
	}
	for _, id := range top {
		if evidence[normalizeSessionID(id)] {
			return true, nil
		}
	}
	return false, nil
}

// normalizeSessionID reduces the two id conventions LongMemEval uses (a plain
// haystack id vs. an "answer_<hash>_<n>" evidence id) to a comparable core.
func normalizeSessionID(id string) string {
	id = strings.TrimPrefix(id, "answer_")
	return id
}

func truncateAll(s []string, n int) []string {
	out := make([]string, len(s))
	for i, v := range s {
		if len(v) > n {
			v = v[:n]
		}
		out[i] = v
	}
	return out
}
