//! Frontmost application and window title.
//!
//! Uses `osascript` rather than linking the Accessibility API directly. That is
//! a deliberate v1 tradeoff: it needs no build-time ObjC bindings, degrades to
//! app-name-only when the AX permission is denied, and costs one short-lived
//! subprocess per poll — negligible at a 5s interval. Swap for a native AX
//! binding if the poll rate ever needs to go below ~1s.

use crate::event::{Event, Kind, now};
use anyhow::Result;
use std::process::Command;
use std::time::Duration;

/// App name only. Works without Accessibility permission.
const APP_SCRIPT: &str = r#"tell application "System Events" to name of first application process whose frontmost is true"#;

/// App plus focused window title. Requires Accessibility permission; returns a
/// non-zero status or an error string without it.
const APP_AND_TITLE_SCRIPT: &str = r#"tell application "System Events"
    set p to first application process whose frontmost is true
    set n to name of p
    try
        set w to name of front window of p
    on error
        set w to ""
    end try
end tell
return n & "\n" & w"#;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Granularity {
    /// Accessibility granted: app + window title.
    AppAndTitle,
    /// Denied: app name only. Still useful, just coarser.
    AppOnly,
}

pub struct Frontmost {
    pub granularity: Granularity,
}

fn osascript(script: &str) -> Result<String> {
    let out = Command::new("osascript").arg("-e").arg(script).output()?;
    if !out.status.success() {
        anyhow::bail!("osascript failed: {}", String::from_utf8_lossy(&out.stderr).trim());
    }
    Ok(String::from_utf8_lossy(&out.stdout).trim_end().to_string())
}

impl Frontmost {
    /// Probe once at startup so the daemon can tell the user what it will and
    /// will not see, instead of silently capturing less than they expect.
    pub fn probe() -> Frontmost {
        let granularity = match osascript(APP_AND_TITLE_SCRIPT) {
            Ok(s) if s.lines().count() >= 1 && !s.is_empty() => Granularity::AppAndTitle,
            _ => Granularity::AppOnly,
        };
        Frontmost { granularity }
    }

    pub fn sample(&self) -> Result<Event> {
        let ts = now();
        match self.granularity {
            Granularity::AppOnly => Ok(Event::new(ts, Kind::Focus).app(osascript(APP_SCRIPT)?)),
            Granularity::AppAndTitle => {
                let out = osascript(APP_AND_TITLE_SCRIPT)?;
                let mut lines = out.splitn(2, '\n');
                let app = lines.next().unwrap_or_default().to_string();
                let title = lines.next().unwrap_or_default().trim().to_string();

                let mut e = Event::new(ts, Kind::Focus).app(app);
                if !title.is_empty() {
                    e = e.title(title);
                }
                Ok(e)
            }
        }
    }
}

pub const POLL_INTERVAL: Duration = Duration::from_secs(5);
