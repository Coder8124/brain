# feat/tauri-widget — the app

Tauri v2. Idle RAM is a feature for a process that runs all day; Electron's
baseline is not acceptable for something that sits in the menubar forever.

`crates/core` becomes the Tauri backend directly — no rewrite, the CLI and the
app share one engine.

## Surfaces

**Menubar orb.** The permanent presence. State must be readable at a glance:

| State | Read |
|---|---|
| idle | quiet, low contrast |
| thinking | subtle pulse |
| has proposals | badge with count |
| **paused** | unmistakably different — this is a safety indicator, not decoration |

**Pull-down panel** (⌥Space). Today's timeline, ask box, review queue.
Keyboard-first: `j`/`k` to move, `enter` accept, `x` reject, `e` edit.
The panel should be usable without ever touching the mouse.

**Graph window.** Separate, on demand. See `feat/graph-view`.

## Design

Use the `ui-ux-pro-max` skill for style/palette/typography selection rather
than improvising. Starting point from its style DB: **Modern Dark** — deep
`#020203` base, `#5E6AD2` accent, 16px radii, `cubic-bezier(0.16,1,0.3,1)`
easing, hairline `rgba(255,255,255,0.08)` borders. Explicitly no pure black
(OLED smear on scroll).

Query before building each surface:
```
python3 ~/.claude/skills/ui-ux-pro-max/scripts/search.py --domain ux "menubar panel keyboard navigation"
```

Constraints the design must respect:
- Panel opens in under 100ms. It is invoked dozens of times a day; anything
  slower and the user stops reaching for it.
- Never steal focus from the active app on a proactive nudge.
- Respect `prefers-reduced-motion`.
- Full keyboard operation, real focus rings.

## Done when

Orb reflects live state, panel answers a question against the vault, and the
review queue is fully operable from the keyboard.
