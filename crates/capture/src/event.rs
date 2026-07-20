//! The episodic record.
//!
//! These rows are high-volume and individually near-worthless; their value is
//! entirely in aggregate. They are never written to markdown directly and are
//! pruned on a retention window.

use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum Kind {
    Focus,
    Url,
    File,
    Commit,
    Calendar,
    Clipboard,
}

impl Kind {
    pub fn as_str(self) -> &'static str {
        match self {
            Kind::Focus => "focus",
            Kind::Url => "url",
            Kind::File => "file",
            Kind::Commit => "commit",
            Kind::Calendar => "calendar",
            Kind::Clipboard => "clipboard",
        }
    }

    pub fn parse(s: &str) -> Option<Kind> {
        Some(match s {
            "focus" => Kind::Focus,
            "url" => Kind::Url,
            "file" => Kind::File,
            "commit" => Kind::Commit,
            "calendar" => Kind::Calendar,
            "clipboard" => Kind::Clipboard,
            _ => return None,
        })
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Event {
    pub ts: i64,
    pub kind: Kind,
    pub app: Option<String>,
    pub title: Option<String>,
    pub url: Option<String>,
    pub path: Option<String>,
    /// Seconds this state persisted. Filled in by the coalescer, not the source.
    pub dur_s: i64,
}

impl Event {
    pub fn new(ts: i64, kind: Kind) -> Event {
        Event { ts, kind, app: None, title: None, url: None, path: None, dur_s: 0 }
    }

    pub fn app(mut self, v: impl Into<String>) -> Self {
        self.app = Some(v.into());
        self
    }

    pub fn title(mut self, v: impl Into<String>) -> Self {
        self.title = Some(v.into());
        self
    }

    pub fn url(mut self, v: impl Into<String>) -> Self {
        self.url = Some(v.into());
        self
    }

    pub fn path(mut self, v: impl Into<String>) -> Self {
        self.path = Some(v.into());
        self
    }

    /// What makes two samples "the same state" for coalescing purposes.
    pub fn identity(&self) -> (Kind, Option<&str>, Option<&str>) {
        (self.kind, self.app.as_deref(), self.title.as_deref())
    }
}

/// A focus sample shorter than this is almost always incidental — tabbing
/// through windows to get somewhere else. Kept in the raw table for
/// completeness but excluded from rollups and routine mining.
pub const INCIDENTAL_SECS: i64 = 8;

pub fn now() -> i64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_secs() as i64)
        .unwrap_or(0)
}
