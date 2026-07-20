//! Commits in watched repositories.
//!
//! The highest signal-per-row source available: a commit is an explicit,
//! self-described unit of work, already written by the user. Worth mining
//! before anything inferred from window titles.

use crate::event::{Event, Kind};
use anyhow::Result;
use std::path::Path;
use std::process::Command;

/// Commits authored after `since`. Scoped to the current user's commits —
/// pulling a colleague's merged work into your own timeline would be wrong.
pub fn commits_since(repo: &Path, since: i64) -> Result<Vec<Event>> {
    if !repo.join(".git").exists() {
        return Ok(vec![]);
    }

    let email = Command::new("git")
        .arg("-C")
        .arg(repo)
        .args(["config", "user.email"])
        .output()
        .ok()
        .map(|o| String::from_utf8_lossy(&o.stdout).trim().to_string())
        .unwrap_or_default();

    let out = Command::new("git")
        .arg("-C")
        .arg(repo)
        .args([
            "log",
            "--all",
            "--no-merges",
            &format!("--since={since}"),
            &format!("--author={email}"),
            "--pretty=format:%ct%x1f%s",
        ])
        .output()?;

    if !out.status.success() {
        return Ok(vec![]);
    }

    let repo_name = repo.file_name().map(|s| s.to_string_lossy().into_owned()).unwrap_or_default();

    Ok(String::from_utf8_lossy(&out.stdout)
        .lines()
        .filter_map(|line| {
            let (ts, subject) = line.split_once('\u{1f}')?;
            let ts: i64 = ts.trim().parse().ok()?;
            // `--since` is inclusive-ish and drifts; enforce the bound here so
            // repeated polls cannot re-emit the same commit.
            (ts > since).then(|| {
                Event::new(ts, Kind::Commit)
                    .app("git")
                    .title(subject.to_string())
                    .path(repo_name.clone())
            })
        })
        .collect())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn non_repo_yields_nothing() {
        assert!(commits_since(Path::new("/tmp"), 0).unwrap().is_empty());
    }

    #[test]
    fn reads_this_repo_and_respects_the_since_bound() {
        let here = std::env::current_dir().unwrap();
        if !here.join(".git").exists() {
            return; // not a repo in this context; nothing to assert
        }
        let all = commits_since(&here, 0).unwrap();
        if all.is_empty() {
            return; // no commits by this author
        }
        let newest = all.iter().map(|e| e.ts).max().unwrap();
        assert!(
            commits_since(&here, newest).unwrap().is_empty(),
            "polling from the newest commit must not re-emit it"
        );
    }
}
