# feat/graph-view — step 4

The headline feature, built fourth on purpose. A graph view over an empty or
low-quality vault is a beautiful picture of nothing.

## Rendering

**Layout in Rust, render in WebGL.** Force simulation in JS falls over around
2k nodes and this vault will pass that within a couple of months of capture.

- Layout: `forceatlas2` in the backend, computed once, cached in `.brain/`,
  recomputed incrementally on vault change.
- Render: sigma.js or cosmograph. Not d3-force, not vis.js.

## Ego mode only

Never render the whole graph. Render **2 hops from a focus node**, with hop
count as a slider. The full-graph hairball is a screenshot, not a tool — it
looks impressive once and is useless every time after.

Default focus: today's daily note.

## Edge rendering by provenance

| Source | Stroke |
|---|---|
| body `[[wikilink]]` | solid, full opacity |
| typed relation, `conf` ≥ 0.8 | solid, opacity ∝ conf |
| typed relation, `conf` < 0.8 | dashed |
| embedding similarity | faint, toggle off by default, **never persisted** |

Similarity edges are computed at view time from vectors already in the index.
They are a lens, not a fact, and writing them to disk would launder a guess
into a claim.

## Time scrubber

The single most delightful thing here: drag a date range and watch the graph
accrete. Needs `first_seen` per node and per edge — worth carrying in
frontmatter from the start rather than backfilling.

## Interaction

- Click node → open in Obsidian (`obsidian://open?vault=…&file=…`)
- Hover → title, kind, degree, last touched
- Colour by `type`, size by degree (clamped — one hub node shouldn't dwarf
  everything)
- `/` to jump to a node by name

## Done when

Opens on today's note in under 300ms at 5k nodes, scrubs a month smoothly,
and click-through lands in the right Obsidian pane.
