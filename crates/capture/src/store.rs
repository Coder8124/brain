//! Persistence for the episodic tier.
//!
//! Lives in the same SQLite file as the vault index but is conceptually
//! separate: the index is a cache derived from markdown, this is primary data
//! that exists nowhere else. It is the one part of `.brain/` that cannot be
//! rebuilt, which is why retention is a deliberate setting rather than a
//! cleanup detail.

use crate::event::{Event, Kind};
use anyhow::Result;
use rusqlite::{Connection, params};

pub const SCHEMA: &str = r#"
CREATE TABLE IF NOT EXISTS events (
    id     INTEGER PRIMARY KEY,
    ts     INTEGER NOT NULL,
    kind   TEXT    NOT NULL,
    app    TEXT,
    title  TEXT,
    url    TEXT,
    path   TEXT,
    dur_s  INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS events_ts   ON events(ts);
CREATE INDEX IF NOT EXISTS events_kind ON events(kind, ts);

-- High-water marks so pulling browser history stays incremental instead of
-- rescanning the whole file every poll.
CREATE TABLE IF NOT EXISTS source_state (
    source TEXT PRIMARY KEY,
    cursor INTEGER NOT NULL
);
"#;

pub fn init(conn: &Connection) -> Result<()> {
    conn.execute_batch(SCHEMA)?;
    Ok(())
}

pub fn insert(conn: &Connection, e: &Event) -> Result<()> {
    conn.execute(
        "INSERT INTO events (ts, kind, app, title, url, path, dur_s)
         VALUES (?1,?2,?3,?4,?5,?6,?7)",
        params![e.ts, e.kind.as_str(), e.app, e.title, e.url, e.path, e.dur_s],
    )?;
    Ok(())
}

pub fn insert_many(conn: &mut Connection, events: &[Event]) -> Result<usize> {
    let tx = conn.transaction()?;
    for e in events {
        tx.execute(
            "INSERT INTO events (ts, kind, app, title, url, path, dur_s)
             VALUES (?1,?2,?3,?4,?5,?6,?7)",
            params![e.ts, e.kind.as_str(), e.app, e.title, e.url, e.path, e.dur_s],
        )?;
    }
    tx.commit()?;
    Ok(events.len())
}

pub fn cursor(conn: &Connection, source: &str) -> i64 {
    conn.query_row("SELECT cursor FROM source_state WHERE source = ?1", params![source], |r| r.get(0))
        .unwrap_or(0)
}

pub fn set_cursor(conn: &Connection, source: &str, cursor: i64) -> Result<()> {
    conn.execute(
        "INSERT INTO source_state (source, cursor) VALUES (?1,?2)
         ON CONFLICT(source) DO UPDATE SET cursor = ?2",
        params![source, cursor],
    )?;
    Ok(())
}

/// Events in a window, oldest first.
pub fn range(conn: &Connection, from: i64, to: i64) -> Result<Vec<Event>> {
    let mut stmt = conn.prepare(
        "SELECT ts, kind, app, title, url, path, dur_s FROM events
         WHERE ts >= ?1 AND ts < ?2 ORDER BY ts",
    )?;
    let rows = stmt
        .query_map(params![from, to], |r| {
            let kind: String = r.get(1)?;
            Ok(Event {
                ts: r.get(0)?,
                kind: Kind::parse(&kind).unwrap_or(Kind::Focus),
                app: r.get(2)?,
                title: r.get(3)?,
                url: r.get(4)?,
                path: r.get(5)?,
                dur_s: r.get(6)?,
            })
        })?
        .collect::<Result<Vec<_>, _>>()?;
    Ok(rows)
}

/// Drop raw events past the retention window. Rollups must already have
/// extracted anything worth keeping — that is the whole contract of the two
/// tier design.
pub fn prune(conn: &Connection, older_than_ts: i64) -> Result<usize> {
    Ok(conn.execute("DELETE FROM events WHERE ts < ?1", params![older_than_ts])?)
}

pub fn count(conn: &Connection) -> Result<i64> {
    Ok(conn.query_row("SELECT COUNT(*) FROM events", [], |r| r.get(0))?)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn db() -> Connection {
        let c = Connection::open_in_memory().unwrap();
        init(&c).unwrap();
        c
    }

    #[test]
    fn roundtrips_events_in_a_window() {
        let c = db();
        insert(&c, &Event::new(100, Kind::Focus).app("Ghostty")).unwrap();
        insert(&c, &Event::new(200, Kind::Url).url("https://example.com")).unwrap();
        insert(&c, &Event::new(900, Kind::Focus).app("Chrome")).unwrap();

        let got = range(&c, 0, 500).unwrap();
        assert_eq!(got.len(), 2);
        assert_eq!(got[0].app.as_deref(), Some("Ghostty"));
        assert_eq!(got[1].url.as_deref(), Some("https://example.com"));
    }

    #[test]
    fn cursor_defaults_to_zero_then_persists() {
        let c = db();
        assert_eq!(cursor(&c, "chrome"), 0);
        set_cursor(&c, "chrome", 42).unwrap();
        set_cursor(&c, "chrome", 99).unwrap();
        assert_eq!(cursor(&c, "chrome"), 99);
    }

    #[test]
    fn prune_drops_only_old_rows() {
        let c = db();
        insert(&c, &Event::new(10, Kind::Focus)).unwrap();
        insert(&c, &Event::new(5000, Kind::Focus)).unwrap();
        assert_eq!(prune(&c, 1000).unwrap(), 1);
        assert_eq!(count(&c).unwrap(), 1);
    }
}
