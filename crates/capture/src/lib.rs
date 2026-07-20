pub mod coalesce;
pub mod event;
pub mod privacy;
pub mod sources;
pub mod store;
pub mod timeline;

pub use coalesce::Coalescer;
pub use event::{Event, Kind};
pub use privacy::Policy;

use anyhow::Result;
use rusqlite::Connection;
use std::path::Path;

/// One pass over the pull-based sources (browser history, git). The frontmost
/// sampler is push-based and lives in the daemon loop instead.
pub fn poll_once(conn: &mut Connection, scratch: &Path, repos: &[std::path::PathBuf], policy: &Policy) -> Result<usize> {
    let mut collected: Vec<Event> = Vec::new();

    for browser in sources::browser::detect() {
        let key = format!("browser:{}", browser.name);
        let since = store::cursor(conn, &key);
        match browser.visits_since(since, scratch) {
            Ok((events, high)) => {
                collected.extend(events);
                store::set_cursor(conn, &key, high)?;
            }
            // A locked or TCC-protected history file is expected, not fatal;
            // the rest of the sources should still run.
            Err(e) => eprintln!("· {} history unavailable: {e}", browser.name),
        }
    }

    for repo in repos {
        let key = format!("git:{}", repo.display());
        let since = store::cursor(conn, &key);
        if let Ok(events) = sources::git::commits_since(repo, since) {
            if let Some(high) = events.iter().map(|e| e.ts).max() {
                store::set_cursor(conn, &key, high)?;
            }
            collected.extend(events);
        }
    }

    collected.retain(|e| !policy.should_drop(e));
    collected.sort_by_key(|e| e.ts);
    store::insert_many(conn, &collected)
}
