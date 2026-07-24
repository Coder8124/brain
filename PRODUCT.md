# brain — the memory you own

A local-first second brain and AI copilot. It watches your day, distills it into
an Obsidian vault you fully own, remembers what matters about you across every
conversation, and lends that memory to the AI tools you already use — all on
your machine, nothing uploaded unless you say so.

> **Thesis: memory is the product.** Chat assistants forget you the moment a
> session ends. brain is the persistent, private memory that doesn't — the one
> thing you carry between tools, models, and years.

---

## Table of contents

1. [Who it's for](#who-its-for)
2. [What makes it different](#what-makes-it-different)
3. [The two faces of the memory](#the-two-faces-of-the-memory)
4. [The three modes](#the-three-modes)
5. [The memory system](#the-memory-system)
6. [The secretary's weekly review](#the-secretarys-weekly-review)
7. [Voice — talk to it, hear it back](#voice)
8. [Architecture](#architecture)
9. [Model tiers](#model-tiers)
10. [The command surface](#the-command-surface)
11. [Editions](#editions)
12. [Benchmarks](#benchmarks)
13. [Status & roadmap](#status--roadmap)

---

## Who it's for

- **The self-manager** who wants a secretary that actually *knows* them — their
  preferences, their people, their standing priorities — instead of an archive
  they have to interrogate.
- **The student** who wants study material turned into understanding: notes,
  smart questions, diagnostics, and spaced review, generated from their own
  vault.
- **The operator** who needs a business analyst that reads the spreadsheet,
  checks the math, and tells them the trend — without the numbers leaving the
  building.
- **Anyone using Claude, Cursor, or other AI tools** who wants one private memory
  those tools can share, instead of re-explaining themselves in every app.

## What makes it different

1. **Markdown is truth.** Everything the system knows lives in plain `.md` files
   you can read, edit, grep, and take elsewhere. If this project dies, you keep a
   vault.
2. **The database is a cache.** `.brain/index.db` holds embeddings and indexes.
   Delete it, reindex, get identical state. It is never authoritative.
3. **Local by default.** Nothing leaves the machine unless you name a cloud model
   and confirm the redaction preview. BYOK, paid, or fully local models —
   optimized for Ollama, LM Studio, Jan, and Msty.
4. **Propose, don't assert.** The system writes nothing to your vault and sends
   nothing outward that you haven't accepted — until you raise the auto-accept
   threshold yourself.
5. **Compute, then narrate.** For anything with numbers — finances, forecasts,
   diagnostics, the weekly review — the answer is computed in code and the model
   only phrases it. The model never does the arithmetic.

---

## The two faces of the memory

brain treats its memory as a product with two directions:

### 1. Memory served outward — the MCP server *(shipped)*

`brain mcp serve` exposes your local memory over the **Model Context Protocol**,
the same protocol Claude Desktop, Claude Code, and Cursor speak. Any of those
hosts can plug in and gain one private memory that follows you across every tool
and session. The same store the brain app reads is the one an external agent
writes to — tell one tool something, and the others know it. Nothing is uploaded.

Four tools: `remember`, `recall`, `list_memories`, `forget`. It speaks
newline-delimited JSON-RPC 2.0 over stdio; tool failures come back as `isError`
results (not protocol errors) so the host's model can react.

### 2. Memory turned inward — the mirror *(planned)*

Memory reflected back for self-understanding: the patterns, blind spots, and
"here's what I've noticed about you" that only a system with long, private
memory can offer. Design in progress.

---

## The three modes

The app ships in editions that bundle different modes. The **only** difference
between editions is which modes are offered — the entire engine is shared.

### Secretary

A personal assistant that anticipates instead of archiving.

- **Brief** (`brain brief`) — the proactive digest the app leads with: upcoming
  meetings, open loops (stalest first), what's gone dormant, what you usually do
  around now, and the standing preferences it keeps in mind. Pure arithmetic over
  captured data — no model runs, so it's instant and offline.
- **Open loops** (`brain loop`) — commitments extracted from your notes and chats
  ("email Sarah the deck"), tracked until you close them. An archive doesn't care
  what you promised; a secretary tracks exactly that.
- **Weekly Review** (`brain weekly`) — the Sunday executive briefing (see below).
- **Emotional & context intelligence** — the persistent memory of preferences and
  people is what lets it draft in the right tone and stop asking what it already
  knows.
- **Confirmation gate** — every outbound action is previewed and requires approval
  before it runs.

### Tutor

Turns your vault into learning.

- **Diagnostics** (`brain tutor diagnostic <subject>`) — Khan-style placement
  quizzes with hand-verified question banks for AP Chemistry, AP Physics C, and
  AP Calculus BC.
- **Study material** (`brain tutor study|quiz <topic>`) — summaries and smart
  questions generated from what's in your vault.
- **Spaced repetition** (`brain tutor cards|review`) — an SRS flashcard system.
- **Screen-aware note-taking** (`brain tutor screen on`) — when it detects
  studious activity on screen it takes notes automatically.
- **Session recording** (`brain record`) — a hotkey records a study session,
  transcribes the screen to text, and files it as supporting material.

### Business

The analyst that replaces routine secretarial finance work.

- **Spreadsheets** — reads Excel and CSV into a common table (`internal/sheet`)
  with deterministic stats.
- **Verify, forecast, report** — verifies finances, builds expense reports and
  competitor analyses, produces revenue/profit/margin forecasts and presentation
  prep. All computation is deterministic Go; the model only narrates.
- **MCP client** — pulls live data from external MCP servers (dashboards,
  databases) and summarizes the trends.
- **Agent harness** (`internal/bizagent`) — runs multi-step business tasks behind
  the same confirmation gate.

---

## The memory system

The heart of the product. Two tiers, mirroring how people remember:

| | Episodic | Semantic |
|---|---|---|
| Store | SQLite (`.brain/index.db`) | Obsidian vault (`.md`) + memory store |
| Content | app focus, URLs, files, commits, calendar, clipboard, screen | preferences, people, context, facts |
| Volume | ~50k rows/day | a handful/day |
| Lifetime | rolled up, then pruned (~90d) | permanent |

The pipeline is `events → rollup → proposed notes → review → vault`. Raw events
never become markdown directly — a vault that grows 400 files a day is landfill.

### Persistent memory: extract → store → recall

- **Extract** durable facts from a conversation (preferences, people, standing
  context, plain facts) — a narrow classification, not the passing chat content.
- **Store** with **semantic dedup**: the model paraphrases the same fact
  differently each time, so near-identical memories (≥0.87 cosine) reinforce the
  existing one instead of piling up twins.
- **Recall** with **hybrid retrieval**: vector similarity fused with BM25 lexical
  matching via reciprocal rank fusion (k=60), blended with **effective salience ×
  confidence** so what's relevant, what currently matters, and what's trustworthy
  all shape the ranking.

Memories **decay** (90-day half-life), **reinforce** when recalled or
corroborated, **consolidate** duplicates, and **supersede** stale facts ("moved
to Boston" replaces "lives in NYC") so the current truth is what surfaces.

### Confidence ratings

Every memory carries a **confidence** (0..1) distinct from salience — *how sure
we are the fact is true and current*, versus *how much it matters*. Seeded by
source (a fact you typed is near-certain; one inferred from a passing remark is a
hypothesis), raised by corroboration, and folded into recall so a shaky fact
never outranks a certain one at equal relevance. Shown as a bar in
`brain memory`.

### The memory timeline — git history for memory

`brain memory log` / `brain memory history <id>`. An append-only `memory_log`
records every lifecycle event — **created, reinforced, superseded, merged,
forgotten** — with a text snapshot, so the history stays legible even after a
memory is deleted. You can always answer "when did it start believing this, and
why does it believe it now?"

### The memory relationship graph

`brain memory graph [--similar] [--mermaid] [--json]`. Where the note graph shows
how written notes link, this shows how what the assistant has *learned* connects —
to itself and to the vault. Three edge kinds:

- **mentions** — a memory names a person/project/topic note (`.md` file), matched
  by title or alias on word boundaries, tying the learned layer to the written
  one.
- **supersedes** — a memory replaced an older one, carrying the timeline's history
  into the graph.
- **similar** *(opt-in)* — embedding proximity, surfacing clusters of related
  knowledge.

It renders as a text summary with hubs, a portable **Mermaid** diagram, or JSON
for the widget.

### Auto-detected projects with scoped memory

`brain projects` / `brain project <name>` / `brain projects sync`. The system
surfaces the pieces of work a life is organised around **without the user filing
anything** — the rollup already distils captured activity into project notes, and
each becomes a project with an assembled dossier:

- **notes** and **people** connected in the graph
- **files** and **recent progress** from a bounded scan of the event log
- **goals** from the project note's Goals section plus standing-context memories
- its own **project-scoped memory**

`brain projects sync` auto-tags each memory that names exactly one project, so
**project-scoped recall works with zero classification** — an assistant helping
with one project recalls that project's context, not your whole life.

---

## The secretary's weekly review

`brain weekly` — the Sunday executive-assistant briefing. Where the daily brief
says "what now?", this says "here's your week", computed entirely from captured
data (instant, offline, every number traceable):

- **Accomplished** — closed loops, git commits, notes created
- **Still open** — open loops, stalest first
- **People** — loop counterparts and people named in meeting titles (matched to
  real person-notes, not raw strings)
- **Deadlines** — due hints plus next week's calendar, soonest first
- **Where your time went** — focus hours by app, browsing by domain
- **Habits** — stable routines mined over a 120-day baseline
- **Recommendations** — deterministic, each traceable to a number above

---

<a id="voice"></a>

## Voice — talk to it, hear it back

The assistant has ears and a mouth, entirely on-device. Both engines are
self-contained native binaries the product **bundles** alongside their model
files and runs as subprocesses — no cgo in the Go core, no cloud, no per-word
API. Your voice never leaves the machine.

- **Speech-to-text** — [whisper.cpp](https://github.com/ggml-org/whisper.cpp)
  transcribes a mic turn captured via `ffmpeg` (16 kHz mono). Swap the GGML model
  for the speed/accuracy you want; Moonshine or another CLI can be dropped in via
  an env override.
- **Text-to-speech** — [Piper](https://github.com/rhasspy/piper), a small, fast,
  fully-local neural voice. If Piper isn't bundled yet, TTS falls back to the OS
  voice (macOS `say`) so the assistant can always talk.

Each binary and model resolves in order: an **environment override**, then the
**bundled resources directory** shipped with the app (in a packaged macOS app,
`Contents/Resources/voice/…`), then a plain **PATH** lookup for developers who
installed the tools themselves. `scripts/fetch-voice.sh` pulls the binaries and
models into `resources/voice/` at build time; `brain doctor` shows exactly what
resolved.

**Commands:**

- `brain say <text>` — speak text aloud
- `brain listen [--seconds N]` — transcribe a mic turn to text
- `brain voice [--seconds N]` — a hands-on-keyboard voice Q&A loop over the
  vault: press Enter to talk, hear the answer, repeat

A streaming speaker (`SpeakStream`) also lets the app speak a model reply
sentence by sentence as it generates, so speech starts before the text finishes.

## Architecture

**Stack:** Go + Wails v2 (a frameless, aesthetically-tuned menubar widget app),
pure-Go SQLite (`modernc.org/sqlite`, no cgo) on a single-connection pool.

**Data model:** one SQLite file (`.brain/index.db`) holds the rebuildable cache —
`notes`, `aliases`, `edges`, `embeddings`, full-text search — alongside the
primary episodic `events` and the `memories` / `memory_log` / `commitments`
tables. The vault's `.md` files are the source of truth; the DB is derived and
disposable.

**Packages** (`internal/`):

| Package | Role |
|---|---|
| `event` | the episodic record shared by capture and its sources |
| `capture` | episodic tier — sampling, coalescing, storage, privacy, retention |
| `index` | the rebuildable cache: notes, edges, embeddings, FTS |
| `vault` | reads notes from the Obsidian vault |
| `rollup` | turns episodic events into proposed vault notes (and `jot` braindumps) |
| `routine` | finds recurring structure (habits, anomalies) in the episodic tier |
| `memory` | persistent memory — store, recall, dedup, consolidate, confidence, timeline, graph |
| `project` | auto-detects projects and assembles their dossiers |
| `graph` | the note relationship graph the widget renders (ego view, provenance) |
| `router` | decides which model does which job; probes and degrades gracefully |
| `provider` | one client for every local model runtime (Ollama/LM Studio/Jan/Msty) |
| `agent` | the conversational assistant (streams, injects memory + vault grounding) |
| `secretary` | initiative — the brief, open loops, the weekly review |
| `action` | the confirmation gate for anything the assistant would do outward |
| `flavor` | the personality/edition the assistant wears |
| `tutor` | turns the vault into study material (diagnostics, SRS, screen notes) |
| `business` | reaches outward via MCP to summarize external data |
| `sheet` | reads spreadsheets (xlsx, csv) into a common table |
| `bizagent` | the business agent harness |
| `record` | captures a study session and turns it into notes |
| `voice` | on-device speech-to-text (whisper.cpp) and text-to-speech (Piper) |
| `mcpserver` | exposes brain's persistent memory as an MCP server |

**Trust loop:** every inference is a **proposal** with cited evidence; nothing
touches the vault or the outside world until accepted. The `action` package is
the only execution path — `Enqueue` never runs anything, `Approve` runs a
registered executor, and outbound actions are previewed first.

**Reliability discipline:**

- **Single-connection safety** — the SQLite pool is capped at one connection, so
  no code nests a query inside an open cursor (it would deadlock). Loaders read
  each table fully and close the cursor before any follow-up.
- **Constrained JSON decoding** — extraction and classification use a response
  schema rather than tool-calling, for reliability on small local models.
- **Compute-then-narrate** — numbers are computed in Go; the model only phrases
  them.

## Model tiers

The router resolves a *tier* to whatever model is actually installed, degrading
to a lower tier with a warning rather than failing the pipeline.

| Tier | Job | Default model |
|---|---|---|
| **T0** | embeddings (768-dim) | `nomic-embed-text` |
| **T1** | fast extract / classify | `gemma3:4b` |
| **T2** | reasoning | `qwen3.6` |
| **T3** | optional cloud | off until you confirm egress |

A model that lists but won't load, or ignores JSON schemas, is caught by a probe
at startup rather than surfacing as a pipeline error later.

## The command surface

```text
brain ask <q> | search <q> | timeline        query what it knows
brain brief                                   the proactive daily digest
brain weekly                                  the Sunday executive review
brain voice | listen | say <text>             talk to it, hear it back (local STT/TTS)
brain memory [add|forget|log|history|graph]   persistent memory + timeline + graph
brain projects | project <name>               auto-detected projects and dossiers
brain loop [add|done|drop]                     open commitments
brain jot <thought>                            braindump: capture and auto-file
brain tutor [diagnostic|study|quiz|cards|review|screen]
brain business [read|analyze|verify|forecast|agent|…]
brain record [--name X] [--no-video]           record a study session into notes
brain graph [focus] [--hops N] [--similar]     the note graph around a note
brain mcp serve                                serve memory to MCP hosts
brain index [--watch] | rollup | prune         cache sync, note proposals, retention
brain doctor [--probe] | key | mode            runtimes/tiers, API keys, edition
```

## Editions

The **only** file that differs between editions is `internal/flavor/edition.go`.

| Edition | Modes |
|---|---|
| **full** (`main`) | Secretary · Tutor · Business |
| **student** | Tutor |
| **business-secretary** | Secretary · Business |

## Benchmarks

Measured against **LongMemEval** (ICLR 2025), the standard long-term-memory
benchmark for chat assistants:

- **96.0% recall@5**, **99.2% recall@10** over the full 500-question LongMemEval-S.
- Hybrid retrieval beats vector-only by **+8.9 points**; the hardest
  single-session case went from 64% → 100%.
- Extract→recall pipeline: **100% accuracy**, **zero dedup growth** on re-learn,
  ~2.7s per conversation to extract, ~30ms per query to recall.

## Status & roadmap

**Shipped:** two-tier memory with hybrid recall; the extract→store→recall loop
with decay, reinforcement, consolidation and supersession; **confidence ratings**;
the **memory timeline** (git history for memory); the **memory relationship
graph**; **auto-detected projects** with scoped memory; Secretary / Tutor /
Business modes; the **weekly executive review**; **on-device voice** (STT + TTS,
bundled); the confirmation-gate trust loop; the benchmark harness; and the **MCP
memory server** — across all three editions.

**Next:**

- **The mirror** — memory turned inward for self-understanding.
- **Persisted conversations** — a first-class chat store, so projects gather real
  conversations (today the "conversations" facet is a proxy) and chats can be
  turned into projects directly.
- **Model-narrated weekly review** — a one-sentence summary over the computed
  numbers.

The through-line stays the same: sharpen the niche around **local, private,
persistent memory** rather than competing in the crowded agentic-work space.
