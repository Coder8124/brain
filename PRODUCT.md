# brain — the memory you own

A local-first memory and continuity layer for AI agents. It watches your day,
distils it into an Obsidian vault you fully own, remembers what matters about
you across every conversation, and hands that context to whichever AI you are
using — all on your machine, nothing uploaded unless you say so.

> **Thesis: memory is the product, and continuity is the proof.** Chat
> assistants forget you the moment a session ends. brain doesn't — and the test
> of that is not whether it can find an old note, it is whether an agent that
> has never seen your project can pick up where a different one stopped.

Two things follow from taking that seriously, and both are why this is not
another vault with search bolted on:

- **What failed is worth more than what succeeded.** Anyone can restate a goal.
  The expensive knowledge is the three approaches already ruled out, and it is
  exactly what dies when a session ends.
- **Memory has to know what it does not know.** Retrieval always returns its
  nearest neighbour; a memory layer that cannot say "nothing recorded" will
  eventually invent an answer and cite itself.

---

## Table of contents

1. [Who it's for](#who-its-for)
2. [What makes it different](#what-makes-it-different)
3. [The two faces of the memory](#the-two-faces-of-the-memory)
4. [The assistant](#the-assistant)
5. [The memory system](#the-memory-system)
6. [Dreaming — the nightly consolidation pass](#dreaming--the-nightly-consolidation-pass)
7. [The secretary's weekly review](#the-secretarys-weekly-review)
8. [Voice — talk to it, hear it back](#voice)
9. [Architecture](#architecture)
10. [Model tiers](#model-tiers)
11. [The command surface](#the-command-surface)
12. [Benchmarks](#benchmarks)
13. [Status & roadmap](#status--roadmap)

---

## Who it's for

- **The self-manager** who wants an assistant that actually *knows* them — their
  preferences, their people, their standing priorities — instead of an archive
  they have to interrogate.
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
6. **The memory is yours, and portable.** Your "personal context" is not locked
   in a vendor's black box — it is markdown you can read, correct, and carry. An
   on-device assistant like Apple Intelligence keeps its sense of you opaque and
   walled to its own apps; brain's memory is a file you own and can lend to any AI
   tool over MCP. It won't match a platform owner on raw signal or ubiquity, and
   doesn't try to — it wins on the one axis a walled assistant structurally can't:
   a memory you hold, inspect, and take everywhere.

---

## The two faces of the memory

brain treats its memory as a product with two directions:

### 1. Memory served outward — the MCP server *(shipped)*

`brain mcp serve` exposes your local memory over the **Model Context Protocol**,
the same protocol Claude Desktop, Claude Code, and Cursor speak — and that any
application can build on. A host doesn't just plug in beside a chat; it can sit its
own product *on top of* one private memory that follows you across every tool and
session. The same store the brain app reads is the one an external agent reads and
writes — tell one tool something, and the others know it. Nothing is uploaded.

Eleven tools in two families.

**Memory** — what do you know about X: write (`remember`, which returns a receipt
saying whether it created a fact or corroborated one it already had), read
(`recall`, `list_memories`), revise (`forget`), ask what changed (`memory_diff`),
enumerate detected projects (`list_projects`).

**Continuity** — where were we. `context(task, project, budget)` assembles
everything bearing on a task: the last checkpoint, the project dossier, the
actual prose of the relevant notes, notes reached one hop through your own
links, memories with their provenance, and open commitments — spent against a
token ceiling and cited by source. `note_progress` records a line of work as it
happens. `checkpoint` commits where you stopped to a markdown note in the vault,
including the field that matters most: what was already tried and didn't work.
`handoff` does the same and names who takes over. `resume` gives that to whoever
arrives next.

That last group is the thesis made mechanical. An agent works, checkpoints, and
shuts down; a different agent — a different *product* — calls `resume` and
continues. The AI is replaceable. The context isn't.

It speaks newline-delimited JSON-RPC 2.0 over stdio; tool failures come back as
`isError` results (not protocol errors) so the host's model can react.

### 2. Memory turned inward — the mirror *(planned)*

Memory reflected back for self-understanding: the patterns, blind spots, and
"here's what I've noticed about you" that only a system with long, private
memory can offer. Design in progress.

---

## The assistant

Memory is the product. The assistant is the surface you reach it through — one
assistant, not a set of personas.

It used to be three: Secretary, Tutor, and Business, each with its own panel and
its own domain data, sharing one engine. That was three products wearing a
trench coat, and every hour spent on a vertical was an hour not spent on the
memory. They are gone. What survives is the part that was never persona-specific
— talking to your memory and having it synthesize an answer.

- **Conversation** — grounded in three things at once: retrieval over the vault,
  the persistent memory of what it knows about *you*, and the live brief of
  what's on your plate. That combination is the difference between an assistant
  and a search box.
- **Presence** (`brain name <name>` · `brain presence`) — the assistant as an
  ambient, named voice. It opens with your brief, answers from your memory, and
  speaks up about an imminent meeting, a loop you've let slip, or a connection it
  dreamt up overnight. You give it a name; that name is how you address it. It is
  bound by one law — *augment, never override*: it proposes and reminds, but never
  decides and never rewrites a conclusion you've reached. Restraint is built in:
  one unprompted nudge at a time, non-urgent ones spaced by a cooldown, and only
  an imminent meeting may break your focus. It runs two ways off one engine: the
  `brain presence` voice loop, and ambiently inside the capture daemon (speaking
  up while you work, holding its tongue when you're heads-down in one app).
  Design in `docs/presence.md`.
- **Brief** (`brain brief`) — the proactive digest the app leads with: upcoming
  meetings, open loops (stalest first), what's gone dormant, what you usually do
  around now, and the standing preferences it keeps in mind. Pure arithmetic over
  captured data — no model runs, so it's instant and offline.
- **Open loops** (`brain loop`) — commitments extracted from your notes and chats
  ("email Sarah the deck"), tracked until you close them. An archive doesn't care
  what you promised; this tracks exactly that.
- **Weekly review** (`brain weekly`) — the Sunday executive briefing (see below).
- **Braindump** (`brain jot <thought>`) — the shortest path from a thought to the
  vault: captured, classified, and filed as a note proposal or an open loop.

Anything that would act on the outside world on your behalf is deliberately not
here. The assistant reads your memory and tells you things; it does not go and do
things. That boundary is the product.


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

### Memory diff — what changed

`brain memory diff [subject] [--since] [--until] [--days N]`. Where the timeline
lists events, the diff answers the question people actually ask — *what changed?* —
over a window, optionally about one subject ("what changed about Sarah?"). It reads
the same append-only log and sorts the changes into what was newly **learned**,
what was **dropped**, and what got **corroborated**. Pure arithmetic over the log:
no model runs, so it is instant and offline, and because it matches on the log's
text snapshots it still surfaces facts that were superseded or forgotten inside the
window. Comparing two arbitrary spans — *January vs July* — is the same machinery
pointed at two windows.

### Memory Replay — since you've been away

`brain replay [--peek]`. Open brain after a gap and it leads with a briefing, not a
blank prompt: what the memory learned and dropped, which projects moved, which
loops you closed and how many still hang, and any connections the nightly dream
left for review — all measured from the last time you caught up. It advances that
last-seen marker as you read, so each replay covers only what's new; `--peek` looks
without resetting the clock. Pure aggregation over the diff, the projects, the
loops, and the dream queue: instant and offline. Opening brain after two weeks
should feel like a briefing, not a search.

### Reflection — descriptive statistics

`brain reflect`. The plain, numeric floor beneath the interpretive mirror: how much
the assistant knows and of what kind, how sure it is (things it's certain of versus
hunches still to corroborate), how the store has grown week over week, which
memories recall leans on most, and which commitments have lingered longest. Every
figure traces to a row and no model runs, so it is instant and never editorialises —
the counting the mirror's interpretation will stand on.

### Memory health — git status for the store

`brain memory health`. Cheap, deterministic checks that say where the memory needs
tending: near-duplicates that slipped past write-time dedup, facts gone stale (old,
never recalled, faded in salience), hunches confidence never firmed up, and
memories linked to no note. It reports a single **score** — the share of memories
carrying no defect — and points duplicates and stale facts at `brain memory
consolidate`. No model; every figure is a query.

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

## Dreaming — the nightly consolidation pass *(planned)*

Every night, while you're away, brain sleeps on the day. Not a late rollup —
rollup files what happened; dreaming reorganises what it *means*. Modelled on how
a brain actually consolidates memory, it runs in two phases, and keeping them
distinct is the whole point.

Everything below obeys the two rules the rest of the system runs on: **compute,
then narrate** — what to replay, fade, and connect is decided by arithmetic, and
the model only phrases it — and **propose, don't assert** — the dream's
inferences land in your review queue, never in your vault unasked.

### NREM — stabilise

The cheap, deterministic phase. It runs first, every night, and barely touches a
model.

- **Prioritised replay** — it revisits the day's *salient* moments, not all 50k
  events, folding near-duplicate memories together and letting a newer fact
  supersede the one it replaces. What earns its keep stays sharp.
- **Gist extraction** — the one thing rollup never did: it turns specifics into a
  *rule*. Twenty evenings of clearing your commitments before Monday become one
  standing fact — *you clear loops on Sunday nights* — filed as semantic memory,
  not twenty episodic traces. Patterns backed by hard counts are recorded
  directly; anything the model had to infer is filed as a hypothesis at low
  confidence, to be proven or forgotten by whether it recurs.
- **Homeostatic downscaling** — a gentle nightly renormalisation of the whole
  memory field, so only what's genuinely reinforced stands tall. This is how the
  store stays clean by construction instead of by deletion.
- **Artifact association** — the files, commits, and pages that mattered today get
  tied to the work they belong to. brain doesn't index your code or try to become
  a copilot; it just remembers *that this artifact was in play on this project*.

### REM — recombine

The creative phase, run last, over the compressed and cleaned field NREM leaves
behind. This is the engine behind [the mirror](#the-two-faces-of-the-memory): it
looks for the non-obvious connection between distant pieces of work, the analogy
across two projects, the thread worth starting — and hands them to you in the
morning brief as *"overnight, I noticed…"*.

Recombination is also exactly where a memory system could lie to itself, so it is
fenced on three sides:

1. **Proposals only.** A dreamed-up connection is a suggestion in your queue,
   never a silent edit to your memory.
2. **It must show its work.** Every insight names the two things it connects; one
   that can't is discarded before you ever see it. No un-grounded leaps.
3. **It has to earn belief.** An accepted insight enters at low confidence and
   only firms up if reality keeps agreeing — it never outranks something you told
   brain directly.

### The command

```text
brain dream [--date YYYY-MM-DD] [--phase nrem|rem] [--dry-run]
```

It runs itself at midnight in your timezone, on the day that just ended. Until
you raise the auto-accept threshold, it reports rather than writes — you can read
exactly what a night of sleep would change before you let it change anything.

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

## Appearance

The widget ships with a palette of themes, chosen from a picker in the header and
remembered across launches. The whole UI is drawn from CSS variables, so a theme
is a swap of one variable block and nothing else:

- **Dark** (default), **Light**
- **Paper** — warm cream and sepia ink, in a book-like serif
- **Digital** — terminal green on near-black, monospace throughout
- **Blue** — deep ocean with a cyan accent
- **Red** — dark crimson with a coral accent
- **Auto** — follows the time of day, light through the day and dark at night,
  re-checked on its own so it flips at dawn and dusk without a reload

Safety signals (recording, an imminent meeting, a destructive action) stay red in
every theme.

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
| `vault` | reads notes from the Obsidian vault, and is the one door that writes to it |
| `rollup` | turns episodic events into proposed vault notes (and `jot` braindumps) |
| `routine` | finds recurring structure (habits, anomalies) in the episodic tier |
| `memory` | persistent memory — store, recall, dedup, consolidate, confidence, timeline, graph |
| `project` | auto-detects projects and assembles their dossiers |
| `graph` | the note relationship graph the widget renders (ego view, provenance) |
| `router` | decides which model does which job; probes and degrades gracefully |
| `provider` | one client for every local model runtime (Ollama/LM Studio/Jan/Msty) |
| `agent` | the conversational assistant (streams, injects memory + vault grounding) |
| `secretary` | initiative — the brief, open loops, the weekly review |
| `routine` | mines recurring patterns out of the timeline |
| `flavor` | the assistant's name, your name, and how forward it is |
| `voice` | on-device speech-to-text (whisper.cpp) and text-to-speech (Piper) |
| `session` | working notes and checkpoints — what an agent was doing, and where it stopped |
| `contextpack` | assembles and budgets everything bearing on a task, from every store |
| `mcpserver` | serves memory and continuity to MCP hosts; an adapter, not where logic lives |

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
brain replay [--peek]                          catch up on what changed since last time
brain reflect                                 descriptive stats over your memory
brain weekly                                  the Sunday executive review
brain voice | listen | say <text>             talk to it, hear it back (local STT/TTS)
brain name [<name>] | presence                 the ambient, named assistant (voice)
brain memory [add|forget|log|history|graph|diff|health]   persistent memory + timeline + graph + diff + health
brain projects | project <name>               auto-detected projects and dossiers
brain loop [add|done|drop]                     open commitments
brain jot <thought>                            braindump: capture and auto-file
brain graph [focus] [--hops N] [--similar]     the note graph around a note
brain context <task> [--project p] [--budget n] everything bearing on a task, budgeted
brain note <project> <what you did>            record progress as you work
brain checkpoint <project> [--handoff who]     commit where you stopped, into the vault
brain resume <project>                         pick up where the last agent left off
brain sessions <project>                       the checkpoint log for a project
brain mcp serve                                serve the memory layer to MCP hosts and your own apps
brain index [--watch] | rollup | prune         cache sync, note proposals, retention
brain doctor [--probe] | key                   runtimes/tiers, API keys
```

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
the **memory timeline** (git history for memory); the **memory diff** (what
changed, instant and offline); **memory health** (git status for the store); the
**memory relationship
graph**; **auto-detected projects** with scoped memory; the **weekly executive
review**; the **presence** (the ambient, named assistant — voice greeting,
grounded answers, restrained interjections);
**Memory Replay** (catch up on what changed since last time); **reflection** (`brain reflect`, descriptive stats
over memory); **on-device voice** (STT + TTS,
bundled); a palette of **themes** (light/dark/paper/digital/blue/red + auto); the
the propose-then-accept trust loop; the benchmark harness; **budgeted context**
(`brain context` — vault prose, graph-reached notes, open loops and provenance,
spent against a token ceiling); **session continuity** (`note` / `checkpoint` /
`resume` / `handoff` — working notes in the cache, checkpoints as markdown in the
vault, so one agent can hand a project to a different agent); and the **MCP
layer** serving all of it, so other apps can build on the memory.

**Next:**

- **Dreaming** — the nightly NREM/REM consolidation pass (`brain dream`): prioritised
  replay, gist extraction (episodic→semantic), homeostatic downscaling, and gated
  REM recombination that feeds the mirror. Spec in `docs/dream.md`.
- **The mirror** — memory turned inward for self-understanding, interpreting the
  numbers `brain reflect` computes.
- **Persisted conversations** — a first-class chat store, so projects gather real
  conversations (today the "conversations" facet is a proxy) and chats can be
  turned into projects directly.
- **Model-narrated weekly review** — a one-sentence summary over the computed
  numbers.

The through-line stays the same: sharpen the niche around **local, private,
persistent memory** rather than competing in the crowded agentic-work space.
