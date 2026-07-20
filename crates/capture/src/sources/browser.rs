//! Browser history.
//!
//! Read from the browsers' own SQLite files rather than an extension: no
//! install per browser, no permissions dance, and full history including
//! sessions that predate this tool.
//!
//! **Always copy the file before opening it.** Chrome holds an exclusive lock
//! while running, so opening in place either fails or — worse, with some
//! journal modes — can disturb the browser's own state. The copy is cheap and
//! the correctness is not optional.

use crate::event::{Event, Kind};
use anyhow::{Context, Result};
use rusqlite::{Connection, OpenFlags, params};
use std::path::{Path, PathBuf};

/// Chromium stores microseconds since 1601-01-01.
const CHROME_EPOCH_OFFSET_S: i64 = 11_644_473_600;
/// WebKit stores seconds since 2001-01-01.
const SAFARI_EPOCH_OFFSET_S: i64 = 978_307_200;

#[derive(Debug, Clone, Copy)]
pub enum Flavor {
    Chromium,
    Safari,
}

#[derive(Debug, Clone)]
pub struct Browser {
    pub name: &'static str,
    pub flavor: Flavor,
    pub db: PathBuf,
}

fn home() -> PathBuf {
    std::env::var("HOME").map(PathBuf::from).unwrap_or_default()
}

/// Only browsers whose history file actually exists on this machine.
pub fn detect() -> Vec<Browser> {
    let h = home();
    let candidates: Vec<(&'static str, Flavor, PathBuf)> = vec![
        ("chrome", Flavor::Chromium, h.join("Library/Application Support/Google/Chrome/Default/History")),
        ("arc", Flavor::Chromium, h.join("Library/Application Support/Arc/User Data/Default/History")),
        ("brave", Flavor::Chromium, h.join("Library/Application Support/BraveSoftware/Brave-Browser/Default/History")),
        ("edge", Flavor::Chromium, h.join("Library/Application Support/Microsoft Edge/Default/History")),
        ("safari", Flavor::Safari, h.join("Library/Safari/History.db")),
    ];

    candidates
        .into_iter()
        .filter(|(_, _, p)| p.exists())
        .map(|(name, flavor, db)| Browser { name, flavor, db })
        .collect()
}

/// Copy the live history file somewhere we can safely open it. WAL sidecars
/// must come along or recent visits are missing from the copy.
fn snapshot(db: &Path, scratch: &Path) -> Result<PathBuf> {
    std::fs::create_dir_all(scratch)?;
    let dst = scratch.join("history-snapshot.db");

    std::fs::copy(db, &dst).with_context(|| format!("copying {}", db.display()))?;
    for ext in ["-wal", "-shm"] {
        let side = PathBuf::from(format!("{}{ext}", db.display()));
        if side.exists() {
            let _ = std::fs::copy(&side, format!("{}{ext}", dst.display()));
        }
    }
    Ok(dst)
}

impl Browser {
    /// Visits newer than `since` (unix seconds). Caller persists the returned
    /// high-water mark so the next poll is incremental.
    pub fn visits_since(&self, since: i64, scratch: &Path) -> Result<(Vec<Event>, i64)> {
        let snap = snapshot(&self.db, scratch)?;
        let conn = Connection::open_with_flags(&snap, OpenFlags::SQLITE_OPEN_READ_ONLY)?;

        let (sql, offset) = match self.flavor {
            Flavor::Chromium => (
                "SELECT v.visit_time, u.url, u.title
                 FROM visits v JOIN urls u ON u.id = v.url
                 WHERE v.visit_time > ?1 ORDER BY v.visit_time",
                CHROME_EPOCH_OFFSET_S,
            ),
            Flavor::Safari => (
                "SELECT v.visit_time, i.url, v.title
                 FROM history_visits v JOIN history_items i ON i.id = v.history_item
                 WHERE v.visit_time > ?1 ORDER BY v.visit_time",
                SAFARI_EPOCH_OFFSET_S,
            ),
        };

        // Convert the cursor into the browser's own time base rather than
        // converting every row into ours — keeps the comparison indexed.
        let cursor_native = match self.flavor {
            Flavor::Chromium => (since + offset) * 1_000_000,
            Flavor::Safari => since - offset,
        };

        let mut stmt = conn.prepare(sql)?;
        let rows = stmt.query_map(params![cursor_native], |r| {
            let native: i64 = r.get(0)?;
            let url: String = r.get(1)?;
            let title: Option<String> = r.get(2)?;
            Ok((native, url, title))
        })?;

        let mut events = Vec::new();
        let mut high = cursor_native;

        for row in rows {
            let (native, url, title) = row?;
            high = high.max(native);

            let ts = match self.flavor {
                Flavor::Chromium => native / 1_000_000 - offset,
                Flavor::Safari => native as i64 + offset,
            };

            let mut e = Event::new(ts, Kind::Url).url(url).app(self.name);
            if let Some(t) = title.filter(|t| !t.is_empty()) {
                e = e.title(t);
            }
            events.push(e);
        }

        let _ = std::fs::remove_file(&snap);

        let high_unix = match self.flavor {
            Flavor::Chromium => high / 1_000_000 - offset,
            Flavor::Safari => high + offset,
        };
        Ok((events, high_unix.max(since)))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn chrome_epoch_conversion_roundtrips() {
        let unix = 1_752_883_200_i64;
        let native = (unix + CHROME_EPOCH_OFFSET_S) * 1_000_000;
        assert_eq!(native / 1_000_000 - CHROME_EPOCH_OFFSET_S, unix);
    }

    #[test]
    fn safari_epoch_conversion_roundtrips() {
        let unix = 1_752_883_200_i64;
        let native = unix - SAFARI_EPOCH_OFFSET_S;
        assert_eq!(native + SAFARI_EPOCH_OFFSET_S, unix);
    }

    #[test]
    fn detect_returns_only_existing_files() {
        for b in detect() {
            assert!(b.db.exists(), "{} reported without a history file", b.name);
        }
    }
}
