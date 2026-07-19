# brain — design

A local-first second brain. Observes what you do, distills it into an Obsidian
vault, and shows you the graph of what it knows.

## Principles

1. **Markdown is truth.** Everything the system knows lives in plain `.md` files
   you can read, edit, grep, and take elsewhere. If this project dies you keep a
   vault.
2. **The database is a cache.** `.brain/index.db` holds embeddings, edges and
   FTS. Delete it, reindex, get byte-identical state. It is never authoritative.
3. **Local by default.** Nothing leaves the machine unless you name a cloud model
   and confirm the redaction preview.
4. **Propose, don't assert.** The system writes nothing to the vault that you
   haven't accepted, until you raise its auto-accept threshold yourself.

## Two-tier memory

| | Episodic | Semantic |
|---|---|---|
| Store | SQLite (`.brain/index.db`) | Obsidian vault (`.md`) |
| Content | app focus, URLs, files, commits, calendar | people, projects, topics, routines |
| Volume | ~50k rows/day | ~5 notes/day |
| Lifetime | rolled up then pruned (default 90d) | permanent |

The pipeline is `events → rollup → proposed notes → review → vault`. Raw events
never become markdown directly. A vault that grows 400 files a day is landfill.

## Vault layout

```
vault/
  daily/2026-07-18.md
  people/…  projects/…  topics/…  routines/…  sources/…
  .brain/                 # index.db, config.toml, queue.jsonl — do not sync
```

### Note frontmatter

```yaml
type: person                       # person|project|topic|routine|daily|source
aliases: [Sam, "@sameer"]
relations:
  - { pred: works_on, obj: "[[brain]]", conf: 0.82, src: inferred }
  - { pred: manages,  obj: "[[api-team]]", conf: 1.0,  src: stated }
first_seen: 2026-03-02
observations: 47
```

`conf` (0–1) and `src` (`stated` | `inferred` | `imported`) are what keep an
auto-written vault from rotting. Edges below the confidence floor render dashed
in the graph and are excluded from retrieval context.

Body links (`[[wikilinks]]`) are untyped edges at `conf: 1.0, src: stated` —
you wrote them, so they're true.

## Graph edges come from three places

| Source | Trust | Rendering |
|---|---|---|
| `[[wikilinks]]` in body | absolute | solid |
| typed `relations:` frontmatter | `conf` | solid → dashed by conf |
| embedding similarity > θ | weak | faint, toggleable, never persisted |

Similarity edges are computed at view time and never written to disk. They're a
lens, not a fact.

## Model routing

All four target runtimes (Ollama, LM Studio, Jan, Msty) expose an
OpenAI-compatible `/v1`, so there is one client plus a capability probe. Local
endpoints are auto-discovered by port scan: 11434, 1234, 1337, 10000.

| Tier | Job | Size | Detected here |
|---|---|---|---|
| T0 | embeddings | 137M | `nomic-embed-text` |
| T1 | per-event tag / classify / extract | 3–4B | `gemma3:4b` |
| T2 | daily rollup, entity resolution | 8–24B | `qwen3.6`, `gpt-oss:20b` |
| T3 | weekly synthesis, hard queries | cloud | BYOK, opt-in |

**Use constrained decoding, not tool-calling, for extraction.** Small local
models are unreliable at tool schemas but near-perfect with a JSON schema
enforced at the sampler (Ollama `format`, LM Studio structured output).

## Capture (macOS)

Default on, cheap, high signal:
- frontmost app + window title via `NSWorkspace` / AX API, sampled 5s, coalesced
- browser history read from Chrome/Arc/Safari SQLite
- EventKit calendar, FSEvents file activity, commits in watched repos
- clipboard text (blocklisted apps excluded)

Default **off**, explicit opt-in:
- periodic screenshot + Vision framework OCR (local, no network)

Always: per-app blocklist (password managers, Messages, banking), auto-pause on
secure text fields, visible menubar recording state, global panic hotkey.

Routine detection is sequence mining over the episodic table, not prompting.
The LLM's job is to *name* a discovered pattern, not to find it.

## App

Tauri v2 (idle RAM is a feature for an always-on process). Menubar orb with
state; pull-down panel with today's timeline, ask box, review-queue badge.

Graph view: layout precomputed in Rust, rendered in WebGL. Ego-mode only —
2 hops from a focus node, never the whole graph. Time scrubber to watch the
graph accrete. Click-through to Obsidian.

## Build order

1. **Vault index + retrieval + ask box.** No capture. ← *current*
2. App-focus + browser capture → episodic timeline. No LLM.
3. Rollup → proposed notes → review queue.
4. Graph view.
5. Routine mining, proactive nudges.

## Known failure modes

- **Summary inbreeding** — always re-derive rollups from episodic source rows,
  never from prior summaries.
- **Watcher feedback loop** — atomic temp+rename writes, 2s debounce on our own
  paths.
- **Duplicate notes** — resolve entities *before* creating a note.
- **The creepy moment** — visible recording state + panic hotkey.
