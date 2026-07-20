//! Collapsing a stream of identical samples into durable sessions.
//!
//! Polling frontmost-app every 5s produces ~17k rows per day per source, almost
//! all of them repeats. Coalescing happens on write, not on read: storing the
//! raw stream and aggregating at query time costs disk and query latency
//! forever in exchange for nothing.

use crate::event::{Event, now};

/// Accumulates samples and emits a completed session whenever the state
/// changes. Pure and synchronous so it can be tested without a clock.
#[derive(Debug, Default)]
pub struct Coalescer {
    open: Option<Event>,
    /// If a sample arrives after a gap much longer than the poll interval, the
    /// machine was probably asleep. Closing the session at its last known
    /// sample avoids inventing an 8-hour "session" over a lunch break.
    max_gap_s: i64,
}

impl Coalescer {
    pub fn new(max_gap_s: i64) -> Coalescer {
        Coalescer { open: None, max_gap_s }
    }

    /// Feed one sample. Returns a completed session if this sample ended one.
    pub fn push(&mut self, sample: Event) -> Option<Event> {
        let Some(open) = self.open.as_mut() else {
            self.open = Some(sample);
            return None;
        };

        let gap = sample.ts - (open.ts + open.dur_s);
        let same = open.identity() == sample.identity();

        if same && gap <= self.max_gap_s {
            open.dur_s = sample.ts - open.ts;
            return None;
        }

        // Either the state changed or we lost time; close the open session.
        let mut done = self.open.take().expect("checked above");
        if !same && gap <= self.max_gap_s {
            // State changed cleanly — the old session ran right up to now.
            done.dur_s = sample.ts - done.ts;
        }
        self.open = Some(sample);
        Some(done)
    }

    /// Close the current session, e.g. on shutdown.
    pub fn flush(&mut self) -> Option<Event> {
        let mut done = self.open.take()?;
        if done.dur_s == 0 {
            done.dur_s = (now() - done.ts).max(0);
        }
        Some(done)
    }

    pub fn open_event(&self) -> Option<&Event> {
        self.open.as_ref()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::event::Kind;

    fn focus(ts: i64, app: &str) -> Event {
        Event::new(ts, Kind::Focus).app(app).title("w")
    }

    #[test]
    fn identical_samples_extend_rather_than_duplicate() {
        let mut c = Coalescer::new(60);
        assert!(c.push(focus(0, "Ghostty")).is_none());
        assert!(c.push(focus(5, "Ghostty")).is_none());
        assert!(c.push(focus(10, "Ghostty")).is_none());
        assert_eq!(c.open_event().unwrap().dur_s, 10);
    }

    #[test]
    fn state_change_emits_completed_session() {
        let mut c = Coalescer::new(60);
        c.push(focus(0, "Ghostty"));
        c.push(focus(30, "Ghostty"));

        let done = c.push(focus(45, "Chrome")).expect("session should close");
        assert_eq!(done.app.as_deref(), Some("Ghostty"));
        // Ran right up to the moment Chrome took over, not just to the last
        // sample at t=30.
        assert_eq!(done.dur_s, 45);
    }

    #[test]
    fn long_gap_does_not_invent_a_session_over_sleep() {
        let mut c = Coalescer::new(60);
        c.push(focus(0, "Ghostty"));
        c.push(focus(20, "Ghostty"));

        // Machine slept for two hours, then the same app is frontmost again.
        let done = c.push(focus(7220, "Ghostty")).expect("gap should close it");
        assert_eq!(done.dur_s, 20, "must not absorb the sleep window");

        // And the post-sleep sample opens a fresh session.
        assert_eq!(c.open_event().unwrap().ts, 7220);
    }

    #[test]
    fn flush_closes_the_open_session() {
        let mut c = Coalescer::new(60);
        c.push(focus(0, "Ghostty"));
        c.push(focus(12, "Ghostty"));
        assert_eq!(c.flush().unwrap().dur_s, 12);
        assert!(c.flush().is_none());
    }

    #[test]
    fn title_change_within_one_app_is_a_new_session() {
        let mut c = Coalescer::new(60);
        c.push(Event::new(0, Kind::Focus).app("Chrome").title("inbox"));
        let done = c
            .push(Event::new(10, Kind::Focus).app("Chrome").title("calendar"))
            .expect("title change closes session");
        assert_eq!(done.title.as_deref(), Some("inbox"));
    }
}
