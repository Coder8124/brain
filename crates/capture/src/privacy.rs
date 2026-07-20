//! The constraints that have to hold before this runs unattended.
//!
//! Everything here fails closed: an app we cannot identify is treated as
//! blocked, because the cost of wrongly recording a password manager is far
//! higher than the cost of a gap in the timeline.

use crate::event::Event;

/// Seeded with the categories that are never worth recording. The user's own
/// additions are appended from config.
pub const DEFAULT_BLOCKLIST: &[&str] = &[
    "1Password",
    "Bitwarden",
    "KeePassXC",
    "Keychain Access",
    "Messages",
    "Signal",
    "WhatsApp",
    "Telegram",
    "Mail",
    "FaceTime",
    "System Settings",
];

/// Substrings that mean a window is showing something sensitive regardless of
/// which app it belongs to.
const BLOCKED_TITLE_HINTS: &[&str] = &["password", "passwd", "secret", "private browsing", "incognito", "sign in", "login"];

#[derive(Debug, Clone)]
pub struct Policy {
    pub blocked_apps: Vec<String>,
    pub paused: bool,
}

impl Default for Policy {
    fn default() -> Self {
        Policy {
            blocked_apps: DEFAULT_BLOCKLIST.iter().map(|s| s.to_string()).collect(),
            paused: false,
        }
    }
}

impl Policy {
    pub fn with_extra(mut self, extra: &[String]) -> Self {
        self.blocked_apps.extend(extra.iter().cloned());
        self
    }

    fn app_blocked(&self, app: &str) -> bool {
        let app = app.to_lowercase();
        self.blocked_apps.iter().any(|b| app.contains(&b.to_lowercase()))
    }

    /// Should this sample be dropped entirely? Dropped means never written —
    /// not written-then-filtered, which would leave the content on disk.
    pub fn should_drop(&self, e: &Event) -> bool {
        if self.paused {
            return true;
        }

        match e.app.as_deref() {
            Some(app) if self.app_blocked(app) => return true,
            // An unidentifiable app is blocked, not allowed.
            None if e.url.is_none() && e.path.is_none() => return true,
            _ => {}
        }

        if let Some(title) = &e.title {
            let t = title.to_lowercase();
            if BLOCKED_TITLE_HINTS.iter().any(|h| t.contains(h)) {
                return true;
            }
        }
        false
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::event::Kind;

    #[test]
    fn blocks_password_managers_by_substring() {
        let p = Policy::default();
        assert!(p.should_drop(&Event::new(0, Kind::Focus).app("1Password 8")));
        assert!(p.should_drop(&Event::new(0, Kind::Focus).app("Bitwarden")));
        assert!(!p.should_drop(&Event::new(0, Kind::Focus).app("Ghostty").title("zsh")));
    }

    #[test]
    fn blocks_sensitive_window_titles_in_any_app() {
        let p = Policy::default();
        let e = Event::new(0, Kind::Focus).app("Google Chrome").title("Sign in - Bank");
        assert!(p.should_drop(&e), "sensitive titles must be dropped regardless of app");
    }

    #[test]
    fn unidentifiable_focus_sample_fails_closed() {
        let p = Policy::default();
        assert!(p.should_drop(&Event::new(0, Kind::Focus)));
    }

    #[test]
    fn pause_drops_everything() {
        let mut p = Policy::default();
        p.paused = true;
        assert!(p.should_drop(&Event::new(0, Kind::Focus).app("Ghostty")));
    }

    #[test]
    fn user_additions_are_honoured() {
        let p = Policy::default().with_extra(&["Obsidian".to_string()]);
        assert!(p.should_drop(&Event::new(0, Kind::Focus).app("Obsidian")));
    }
}
