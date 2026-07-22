package action

import (
	"database/sql"
	"fmt"
	"testing"

	_ "modernc.org/sqlite"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if err := Init(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestEnqueueDoesNotExecute(t *testing.T) {
	db := testDB(t)
	ran := false
	Register("test_effect", func(p map[string]string) (string, error) { ran = true; return "done", nil })

	a := &Action{Kind: "test_effect", Title: "do the thing", Preview: "would do the thing", Payload: map[string]string{"x": "1"}}
	if err := Enqueue(db, a); err != nil {
		t.Fatal(err)
	}
	if ran {
		t.Fatal("enqueue must NOT execute the effect — that is the whole point of the gate")
	}
	if a.Status != Pending {
		t.Errorf("queued action should be pending, got %s", a.Status)
	}
	if n, _ := PendingCount(db); n != 1 {
		t.Errorf("pending count = %d, want 1", n)
	}
}

func TestApproveRunsExecutorExactlyOnApproval(t *testing.T) {
	db := testDB(t)
	calls := 0
	var got map[string]string
	Register("counted", func(p map[string]string) (string, error) { calls++; got = p; return "ok", nil })

	a := &Action{Kind: "counted", Title: "x", Payload: map[string]string{"to": "sam"}}
	Enqueue(db, a)

	if calls != 0 {
		t.Fatal("executor ran before approval")
	}
	res, err := Approve(db, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || res != "ok" || got["to"] != "sam" {
		t.Errorf("executor should run once with the payload; calls=%d res=%q got=%v", calls, res, got)
	}
	// Approving again must not re-run it.
	if _, err := Approve(db, a.ID); err == nil {
		t.Error("re-approving an already-approved action should error")
	}
	if calls != 1 {
		t.Error("executor must not run twice")
	}
}

func TestRejectDoesNotRun(t *testing.T) {
	db := testDB(t)
	ran := false
	Register("rej", func(p map[string]string) (string, error) { ran = true; return "", nil })
	a := &Action{Kind: "rej", Title: "x"}
	Enqueue(db, a)

	if err := Reject(db, a.ID); err != nil {
		t.Fatal(err)
	}
	if ran {
		t.Error("a rejected action must never execute")
	}
	if n, _ := PendingCount(db); n != 0 {
		t.Errorf("rejected action should not be pending, count=%d", n)
	}
	// Retained, not deleted.
	if rejected, _ := List(db, Rejected); len(rejected) != 1 {
		t.Error("rejected actions should be retained")
	}
}

func TestApproveWithNoExecutorFailsLoudly(t *testing.T) {
	db := testDB(t)
	a := &Action{Kind: "unconnected_api", Title: "book something real"}
	Enqueue(db, a)
	if _, err := Approve(db, a.ID); err == nil {
		t.Error("approving an action with no executor must fail, not pretend success")
	}
	failed, _ := List(db, Failed)
	if len(failed) != 1 {
		t.Error("the action should be marked failed")
	}
}

func TestExecutorErrorIsRecorded(t *testing.T) {
	db := testDB(t)
	Register("flaky", func(p map[string]string) (string, error) { return "", fmt.Errorf("smtp down") })
	a := &Action{Kind: "flaky", Title: "x"}
	Enqueue(db, a)
	if _, err := Approve(db, a.ID); err == nil {
		t.Error("an executor error should surface")
	}
	failed, _ := List(db, Failed)
	if len(failed) != 1 || failed[0].Result != "smtp down" {
		t.Errorf("failure detail should be recorded, got %+v", failed)
	}
}
