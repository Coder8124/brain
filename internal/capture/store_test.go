package capture

import (
	"database/sql"
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
	if err := InitStore(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestRoundtripsEventsInAWindow(t *testing.T) {
	db := testDB(t)
	for _, e := range []Event{
		{TS: 100, Kind: Focus, App: "Ghostty"},
		{TS: 200, Kind: URL, URL: "https://example.com"},
		{TS: 900, Kind: Focus, App: "Chrome"},
	} {
		if err := Insert(db, e); err != nil {
			t.Fatal(err)
		}
	}

	got, err := Range(db, 0, 500)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d events, want 2", len(got))
	}
	if got[0].App != "Ghostty" || got[1].URL != "https://example.com" {
		t.Errorf("unexpected rows: %+v", got)
	}
}

func TestCursorDefaultsToZeroThenPersists(t *testing.T) {
	db := testDB(t)
	if c := Cursor(db, "chrome"); c != 0 {
		t.Errorf("fresh cursor = %d, want 0", c)
	}
	SetCursor(db, "chrome", 42)
	SetCursor(db, "chrome", 99)
	if c := Cursor(db, "chrome"); c != 99 {
		t.Errorf("cursor = %d, want 99", c)
	}
}

func TestPruneDropsOnlyOldRows(t *testing.T) {
	db := testDB(t)
	Insert(db, Event{TS: 10, Kind: Focus, App: "a"})
	Insert(db, Event{TS: 5000, Kind: Focus, App: "b"})

	n, err := Prune(db, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("pruned %d, want 1", n)
	}
	if c, _ := Count(db); c != 1 {
		t.Errorf("remaining = %d, want 1", c)
	}
}

func TestInsertManyIsAtomicAndCounted(t *testing.T) {
	db := testDB(t)
	n, err := InsertMany(db, []Event{
		{TS: 1, Kind: Focus, App: "a"},
		{TS: 2, Kind: Focus, App: "b"},
	})
	if err != nil || n != 2 {
		t.Fatalf("InsertMany = %d, %v", n, err)
	}
	if c, _ := Count(db); c != 2 {
		t.Errorf("count = %d, want 2", c)
	}
}
