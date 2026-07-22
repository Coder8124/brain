// Package action is the confirmation gate for things the assistant would do to
// the outside world.
//
// The rest of the trust loop governs what the assistant writes into your vault;
// this governs what it does *out* of it — sending an email, booking a trip,
// exporting a file, calling a system that changes state. None of that runs
// unattended. An outbound action is queued with a preview of exactly what would
// happen, and only executes when you approve it — the same propose-then-accept
// shape as the note review queue, applied to actions instead of notes.
package action

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type Status string

const (
	Pending  Status = "pending"
	Approved Status = "approved"
	Rejected Status = "rejected"
	Failed   Status = "failed"
)

// Action is a proposed outbound effect awaiting confirmation.
type Action struct {
	ID      int64             `json:"id"`
	Kind    string            `json:"kind"`    // "send_email", "book_travel", "export_file", …
	Title   string            `json:"title"`   // one-line summary for the queue
	Preview string            `json:"preview"` // exactly what would happen, for the user to read
	Payload map[string]string `json:"payload"` // structured detail the executor needs
	Status  Status            `json:"status"`
	Created int64             `json:"created"`
	Result  string            `json:"result"` // outcome once executed
}

const Schema = `
CREATE TABLE IF NOT EXISTS actions (
    id      INTEGER PRIMARY KEY,
    kind    TEXT NOT NULL,
    title   TEXT NOT NULL,
    preview TEXT NOT NULL,
    payload TEXT NOT NULL,
    status  TEXT NOT NULL DEFAULT 'pending',
    created INTEGER NOT NULL,
    result  TEXT
);
CREATE INDEX IF NOT EXISTS actions_status ON actions(status, created);
`

func Init(db *sql.DB) error {
	_, err := db.Exec(Schema)
	return err
}

// Executor performs one kind of action once it has been approved. Real
// integrations (SMTP, a booking API via MCP, a filesystem export) register one
// each. The gate guarantees an executor never runs without approval.
type Executor func(payload map[string]string) (string, error)

var (
	mu        sync.RWMutex
	executors = map[string]Executor{}
)

// Register wires an executor for a kind. Registering is how a capability opts
// into being gated: the tool enqueues, the executor runs on approval.
func Register(kind string, e Executor) {
	mu.Lock()
	defer mu.Unlock()
	executors[kind] = e
}

func executorFor(kind string) (Executor, bool) {
	mu.RLock()
	defer mu.RUnlock()
	e, ok := executors[kind]
	return e, ok
}

// Enqueue records an outbound action for confirmation. It never executes here —
// that is the whole point.
func Enqueue(db *sql.DB, a *Action) error {
	if a.Kind == "" || a.Title == "" {
		return fmt.Errorf("an action needs a kind and a title")
	}
	payload, _ := json.Marshal(a.Payload)
	if a.Created == 0 {
		a.Created = time.Now().Unix()
	}
	a.Status = Pending
	res, err := db.Exec(
		`INSERT INTO actions (kind, title, preview, payload, status, created) VALUES (?,?,?,?,?,?)`,
		a.Kind, a.Title, a.Preview, string(payload), string(a.Status), a.Created)
	if err != nil {
		return err
	}
	a.ID, _ = res.LastInsertId()
	return nil
}

func List(db *sql.DB, status Status) ([]Action, error) {
	rows, err := db.Query(
		`SELECT id, kind, title, preview, payload, status, created, COALESCE(result,'')
		 FROM actions WHERE status = ? ORDER BY created`, string(status))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Action
	for rows.Next() {
		var a Action
		var payload, st string
		if err := rows.Scan(&a.ID, &a.Kind, &a.Title, &a.Preview, &payload, &st, &a.Created, &a.Result); err != nil {
			return nil, err
		}
		a.Status = Status(st)
		json.Unmarshal([]byte(payload), &a.Payload)
		out = append(out, a)
	}
	return out, rows.Err()
}

func PendingCount(db *sql.DB) (int, error) {
	var n int
	err := db.QueryRow("SELECT COUNT(*) FROM actions WHERE status = 'pending'").Scan(&n)
	return n, err
}

func get(db *sql.DB, id int64) (*Action, error) {
	var a Action
	var payload, st string
	err := db.QueryRow(
		`SELECT id, kind, title, preview, payload, status, created, COALESCE(result,'') FROM actions WHERE id = ?`, id).
		Scan(&a.ID, &a.Kind, &a.Title, &a.Preview, &payload, &st, &a.Created, &a.Result)
	if err != nil {
		return nil, err
	}
	a.Status = Status(st)
	json.Unmarshal([]byte(payload), &a.Payload)
	return &a, nil
}

// Approve runs the action's executor and records the outcome. This is the only
// path that executes an outbound effect, and it exists only behind an explicit
// user decision.
func Approve(db *sql.DB, id int64) (string, error) {
	a, err := get(db, id)
	if err != nil {
		return "", err
	}
	if a.Status != Pending {
		return "", fmt.Errorf("action %d is already %s", id, a.Status)
	}

	exec, ok := executorFor(a.Kind)
	if !ok {
		// No executor means the effect cannot actually be carried out here (e.g.
		// a real booking API is not connected). Fail loudly rather than
		// pretending it happened.
		setStatus(db, id, Failed, "no executor registered for "+a.Kind)
		return "", fmt.Errorf("nothing can perform %q here — it needs an integration", a.Kind)
	}

	result, err := exec(a.Payload)
	if err != nil {
		setStatus(db, id, Failed, err.Error())
		return "", err
	}
	setStatus(db, id, Approved, result)
	return result, nil
}

// Reject discards an action without running it. Kept, not deleted, so the same
// request is not re-proposed and re-surfaced.
func Reject(db *sql.DB, id int64) error {
	return setStatus(db, id, Rejected, "")
}

func setStatus(db *sql.DB, id int64, s Status, result string) error {
	_, err := db.Exec("UPDATE actions SET status = ?, result = ? WHERE id = ?", string(s), result, id)
	return err
}
