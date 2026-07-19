# feat/capture-macos — step 2

Populate the episodic tier. No LLM anywhere in this branch: the deliverable is
a timeline you can scroll, nothing more. If the raw timeline isn't interesting
to look at, no amount of summarisation later will save it.

## Scope

New crate `crates/capture`, plus an `events` table in the existing index.

```sql
CREATE TABLE events (
    id      INTEGER PRIMARY KEY,
    ts      INTEGER NOT NULL,      -- unix seconds, UTC
    kind    TEXT NOT NULL,         -- focus|url|file|commit|calendar|clipboard
    app     TEXT,
    title   TEXT,
    url     TEXT,
    path    TEXT,
    dur_s   INTEGER,               -- filled in on coalesce
    meta    TEXT                   -- JSON escape hatch
);
CREATE INDEX events_ts ON events(ts);
```

## Sources, in build order

1. **Frontmost app + window title** — `NSWorkspace.shared.frontmostApplication`
   for the app, AX API (`AXUIElementCopyAttributeValue` / `kAXTitleAttribute`)
   for the title. Poll 5s. Requires Accessibility permission; prompt once and
   degrade to app-name-only if denied.
2. **Browser history** — read Chrome/Arc `History` and Safari `History.db`
   SQLite directly. **Copy the file before opening** — the browser holds a
   lock and reading in place can corrupt or block. Track a high-water mark on
   `last_visit_time` so each poll is incremental.
3. **Calendar** — EventKit, read-only.
4. **Files + commits** — FSEvents on watched roots; shell out to `git log` for
   repos under those roots.
5. **Clipboard** — `NSPasteboard.changeCount` polling, text only.

## Coalescing

Raw 5s samples are useless at rest. Collapse consecutive identical
`(kind, app, title)` samples into one row with `dur_s`. Do it on write, not on
read — otherwise the table grows 17k rows/day per source for no benefit.

A focus session shorter than ~8s is almost always incidental (alt-tabbing
through). Keep it in the raw table but exclude it from rollups.

## Privacy, non-negotiable

- Per-app blocklist, seeded with password managers, Messages, Mail, banking.
- Detect secure text fields (`AXSecureTextField`) and drop the whole sample.
- Menubar state must visibly distinguish recording from paused.
- Global panic hotkey: pause capture and delete the last N minutes.
- Screenshot/OCR is **not** in this branch. It lands behind its own flag later.

## Done when

`brain timeline --today` prints a readable, correctly-coalesced day and the
retention pruner drops raw events older than 90 days.
