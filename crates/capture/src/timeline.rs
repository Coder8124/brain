//! Rendering the episodic tier for a human.
//!
//! The deliverable of step 2. If the raw timeline is not interesting to read,
//! no amount of summarisation downstream will rescue it — so this exists to be
//! looked at, before any model is pointed at the data.

use crate::event::{Event, INCIDENTAL_SECS, Kind};

pub fn hhmm(ts: i64) -> String {
    // Local time without pulling in a date library: derive the offset once from
    // the platform, then format by arithmetic.
    let secs_of_day = (ts + local_offset()).rem_euclid(86_400);
    format!("{:02}:{:02}", secs_of_day / 3600, (secs_of_day % 3600) / 60)
}

pub fn local_offset() -> i64 {
    // `date +%z` is stable and avoids a tz crate for what is one number.
    std::process::Command::new("date")
        .arg("+%z")
        .output()
        .ok()
        .and_then(|o| {
            let s = String::from_utf8_lossy(&o.stdout).trim().to_string();
            let sign = if s.starts_with('-') { -1 } else { 1 };
            let h: i64 = s.get(1..3)?.parse().ok()?;
            let m: i64 = s.get(3..5)?.parse().ok()?;
            Some(sign * (h * 3600 + m * 60))
        })
        .unwrap_or(0)
}

pub fn dur(secs: i64) -> String {
    match secs {
        s if s < 60 => format!("{s}s"),
        s if s < 3600 => format!("{}m", s / 60),
        s => format!("{}h{:02}m", s / 3600, (s % 3600) / 60),
    }
}

/// Human-readable day. `verbose` keeps incidental sub-8s focus flickers that
/// are otherwise noise.
pub fn render(events: &[Event], verbose: bool) -> String {
    let mut out = String::new();
    let mut shown = 0;

    for e in events {
        if !verbose && e.kind == Kind::Focus && e.dur_s < INCIDENTAL_SECS {
            continue;
        }
        shown += 1;

        let label = match e.kind {
            Kind::Focus => format!(
                "{:<18} {}",
                e.app.as_deref().unwrap_or("?"),
                e.title.as_deref().unwrap_or("")
            ),
            Kind::Url => format!("{:<18} {}", e.app.as_deref().unwrap_or("web"), e.url.as_deref().unwrap_or("")),
            Kind::Commit => format!(
                "{:<18} {} — {}",
                "commit",
                e.path.as_deref().unwrap_or(""),
                e.title.as_deref().unwrap_or("")
            ),
            _ => format!("{:<18} {}", e.kind.as_str(), e.title.as_deref().unwrap_or("")),
        };

        let d = if e.dur_s > 0 { dur(e.dur_s) } else { String::new() };
        out.push_str(&format!("{}  {:>7}  {}\n", hhmm(e.ts), d, label.trim_end()));
    }

    if shown == 0 {
        out.push_str("nothing recorded\n");
    }
    out
}

/// Time per app over a window — the first thing anyone wants from a timeline.
pub fn by_app(events: &[Event]) -> Vec<(String, i64)> {
    let mut totals: std::collections::HashMap<String, i64> = std::collections::HashMap::new();
    for e in events.iter().filter(|e| e.kind == Kind::Focus && e.dur_s >= INCIDENTAL_SECS) {
        *totals.entry(e.app.clone().unwrap_or_else(|| "?".into())).or_default() += e.dur_s;
    }
    let mut v: Vec<_> = totals.into_iter().collect();
    v.sort_by(|a, b| b.1.cmp(&a.1));
    v
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn formats_durations_at_each_scale() {
        assert_eq!(dur(45), "45s");
        assert_eq!(dur(600), "10m");
        assert_eq!(dur(3700), "1h01m");
    }

    #[test]
    fn hides_incidental_flickers_unless_verbose() {
        let events = vec![
            Event { dur_s: 3, ..Event::new(0, Kind::Focus).app("Finder") },
            Event { dur_s: 600, ..Event::new(10, Kind::Focus).app("Ghostty") },
        ];
        assert!(!render(&events, false).contains("Finder"));
        assert!(render(&events, true).contains("Finder"));
    }

    #[test]
    fn by_app_totals_and_sorts_excluding_flickers() {
        let events = vec![
            Event { dur_s: 300, ..Event::new(0, Kind::Focus).app("Ghostty") },
            Event { dur_s: 900, ..Event::new(400, Kind::Focus).app("Chrome") },
            Event { dur_s: 200, ..Event::new(1400, Kind::Focus).app("Ghostty") },
            Event { dur_s: 2, ..Event::new(2400, Kind::Focus).app("Finder") },
        ];
        let totals = by_app(&events);
        assert_eq!(totals[0], ("Chrome".to_string(), 900));
        assert_eq!(totals[1], ("Ghostty".to_string(), 500));
        assert!(!totals.iter().any(|(a, _)| a == "Finder"));
    }

    #[test]
    fn empty_day_says_so() {
        assert_eq!(render(&[], false), "nothing recorded\n");
    }
}
