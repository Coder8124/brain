// Package memory is the assistant's persistent memory.
//
// The vault holds what you write down; this holds what the assistant *learns*
// about you across conversations — your preferences, the people you work with,
// standing context and priorities — the things a good human assistant just
// knows and a stateless one forgets the moment a session ends. Memories are
// embedded and recalled semantically, so the relevant ones surface into a later
// conversation without you repeating yourself. That continuity is what turns a
// chatbot into a second brain.
package memory

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/pragun/brain/internal/provider"
)

// Kind categorises a memory so it can be weighted and surfaced appropriately.
type Kind string

const (
	// Preference: how the user likes things done ("prefers short emails",
	// "no meetings before 10am"). The heart of the "emotional intelligence" a
	// secretary is expected to have.
	Preference Kind = "preference"
	// Person: a relationship fact ("Sarah is the CFO", "Alex is sensitive
	// about deadlines").
	Person Kind = "person"
	// Fact: a durable fact about the user or their work.
	Fact Kind = "fact"
	// Context: standing situational context ("launching in Q4", "hiring two
	// engineers") — what lets the assistant anticipate.
	Context Kind = "context"
)

// Memory is one remembered thing.
type Memory struct {
	ID       int64   `json:"id"`
	Text     string  `json:"text"`
	Kind     Kind    `json:"kind"`
	Salience float64 `json:"salience"` // 0..1, how much it matters
	Source   string  `json:"source"`   // where it was learned (conversation, manual)
	Created  int64   `json:"created"`
	LastUsed int64   `json:"last_used"`
	Uses     int     `json:"uses"`
	Score    float64 `json:"score"` // set on recall: relevance to the query
	vec      []byte  // embedding, loaded only for consolidation; not serialised
}

const Schema = `
CREATE TABLE IF NOT EXISTS memories (
    id          INTEGER PRIMARY KEY,
    text        TEXT NOT NULL,
    kind        TEXT NOT NULL,
    salience    REAL NOT NULL DEFAULT 0.5,
    source      TEXT,
    created     INTEGER NOT NULL,
    last_used   INTEGER NOT NULL DEFAULT 0,
    uses        INTEGER NOT NULL DEFAULT 0,
    vec         BLOB,
    fingerprint TEXT UNIQUE,
    superseded  INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS memories_kind ON memories(kind);
`

func Init(db *sql.DB) error {
	if _, err := db.Exec(Schema); err != nil {
		return err
	}
	// Migration for stores created before superseding existed.
	db.Exec("ALTER TABLE memories ADD COLUMN superseded INTEGER NOT NULL DEFAULT 0")
	return nil
}

func fingerprint(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(text), " "))
}

// Store embeds a memory and saves it, deduplicated on its normalised text so
// the same fact learned twice does not accumulate.
func Store(db *sql.DB, p *provider.Provider, embedModel string, m *Memory) (bool, error) {
	if strings.TrimSpace(m.Text) == "" {
		return false, nil
	}
	if m.Created == 0 {
		m.Created = time.Now().Unix()
	}
	if m.Salience == 0 {
		m.Salience = 0.5
	}

	var vec []byte
	var qvec []float32
	if p != nil {
		vecs, err := p.Embed(embedModel, []string{m.Text})
		if err == nil && len(vecs) == 1 {
			qvec = vecs[0]
			vec = floatsToBlob(qvec)
		}
	}

	// Semantic dedup: the model paraphrases the same fact differently every time
	// it re-extracts it, so exact-text dedup lets re-learning bloat the store.
	// If an existing memory is near-identical in meaning, reinforce it instead of
	// adding a twin — this is what keeps the store from growing on repetition.
	if qvec != nil {
		if id, ok := nearestMemory(db, qvec, DedupThreshold); ok {
			db.Exec("UPDATE memories SET salience = MIN(1.0, salience + 0.05), uses = uses + 1 WHERE id = ?", id)
			return false, nil
		}
	}

	res, err := db.Exec(
		`INSERT OR IGNORE INTO memories (text, kind, salience, source, created, vec, fingerprint)
		 VALUES (?,?,?,?,?,?,?)`,
		m.Text, string(m.Kind), m.Salience, m.Source, m.Created, vec, fingerprint(m.Text))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		m.ID, _ = res.LastInsertId()
	}
	return n > 0, nil
}

// DedupThreshold is how close a new memory must be to an existing one to be
// treated as the same fact. High, so genuinely distinct facts about the same
// subject are kept; only near-restatements collapse.
const DedupThreshold = 0.87

// nearestMemory returns the id of the most-similar active memory if it clears
// the threshold — the write-time guard against near-duplicate accumulation.
func nearestMemory(db *sql.DB, query []float32, threshold float64) (int64, bool) {
	rows, err := db.Query(`SELECT id, vec FROM memories WHERE superseded = 0 AND vec IS NOT NULL`)
	if err != nil {
		return 0, false
	}
	defer rows.Close()
	var bestID int64
	best := threshold
	for rows.Next() {
		var id int64
		var vec []byte
		if rows.Scan(&id, &vec) != nil || len(vec) == 0 {
			continue
		}
		if sim := cosine(query, blobToFloats(vec)); sim >= best {
			best = sim
			bestID = id
		}
	}
	return bestID, bestID != 0
}

// Recall returns the memories most relevant to a query, by embedding similarity.
// The used memories have their reinforcement bumped, so what proves useful
// stays fresh — memory that is exercised persists, memory that never helps fades
// in the ranking.
func Recall(db *sql.DB, p *provider.Provider, embedModel, query string, k int) ([]Memory, error) {
	if p == nil {
		return All(db) // no embedding backend: fall back to everything, salience-first
	}
	vecs, err := p.Embed(embedModel, []string{query})
	if err != nil || len(vecs) == 0 {
		return All(db)
	}
	mems, err := recallByVec(db, vecs[0], k, query)
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	for _, m := range mems {
		db.Exec("UPDATE memories SET last_used = ?, uses = uses + 1 WHERE id = ?", now, m.ID)
	}
	return mems, nil
}

// recallByVec ranks stored memories using hybrid retrieval (vector + BM25 fused)
// blended with effective salience. queryText drives the lexical arm; pass "" to
// fall back to pure vector. The +8.9-point lift this gave on LongMemEval is why
// live recall runs it too, not just the benchmark.
func recallByVec(db *sql.DB, query []float32, k int, queryText string) ([]Memory, error) {
	rows, err := db.Query(`SELECT id, text, kind, salience, source, created, last_used, uses, vec FROM memories WHERE superseded = 0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var mems []Memory
	var cands []Candidate
	for rows.Next() {
		var m Memory
		var kind string
		var vec []byte
		if err := rows.Scan(&m.ID, &m.Text, &kind, &m.Salience, &m.Source, &m.Created, &m.LastUsed, &m.Uses, &vec); err != nil {
			return nil, err
		}
		m.Kind = Kind(kind)
		if len(vec) == 0 {
			continue
		}
		mems = append(mems, m)
		cands = append(cands, Candidate{ID: fmt.Sprint(m.ID), Text: m.Text, Vec: blobToFloats(vec)})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	fused := Fuse(queryText, query, cands) // normalised 0..1 relevance per candidate
	now := time.Now().Unix()
	for i := range mems {
		rel := 0.0
		if i < len(fused) {
			rel = fused[i]
		}
		// Relevance dominates; effective salience (decay + reinforcement) breaks
		// ties toward what currently matters.
		mems[i].Score = rel*0.85 + EffectiveSalience(mems[i], now)*0.15
	}
	sortByScore(mems)
	if len(mems) > k {
		mems = mems[:k]
	}
	return mems, nil
}

// All returns every memory, most salient first — the fallback and the CLI view.
func All(db *sql.DB) ([]Memory, error) {
	rows, err := db.Query(
		`SELECT id, text, kind, salience, source, created, last_used, uses FROM memories ORDER BY salience DESC, created DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Memory
	for rows.Next() {
		var m Memory
		var kind string
		if err := rows.Scan(&m.ID, &m.Text, &kind, &m.Salience, &m.Source, &m.Created, &m.LastUsed, &m.Uses); err != nil {
			return nil, err
		}
		m.Kind = Kind(kind)
		out = append(out, m)
	}
	return out, rows.Err()
}

func Count(db *sql.DB) (int, error) {
	var n int
	err := db.QueryRow("SELECT COUNT(*) FROM memories").Scan(&n)
	return n, err
}

func Forget(db *sql.DB, id int64) error {
	_, err := db.Exec("DELETE FROM memories WHERE id = ?", id)
	return err
}

// --- vector helpers (shared shape with the index) ---

func cosine(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		x, y := float64(a[i]), float64(b[i])
		dot += x * y
		na += x * x
		nb += y * y
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

func floatsToBlob(v []float32) []byte {
	b := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[4*i:], math.Float32bits(f))
	}
	return b
}

func blobToFloats(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[4*i:]))
	}
	return v
}

func sortByScore(m []Memory) {
	for i := 1; i < len(m); i++ {
		for j := i; j > 0 && m[j].Score > m[j-1].Score; j-- {
			m[j], m[j-1] = m[j-1], m[j]
		}
	}
}
