# brain — the memory you own

A local-first second brain and AI copilot. It watches your day, distills it into
an Obsidian vault you fully own, remembers what matters about you across every
conversation, and lends that memory to the AI tools you already use — all on
your machine, nothing uploaded unless you say so.

> **Thesis: memory is the product.** Chat assistants forget you the moment a
> session ends. brain is the persistent, private memory that doesn't — the one
> thing you carry between tools, models, and years.

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
   diagnostics — the answer is computed in code and the model only phrases it.
   The model never does the arithmetic.

---

## The two faces of the memory

brain treats its memory as a product with two directions:

### 1. Memory served outward — the MCP server *(shipped)*

`brain mcp serve` exposes your local memory over the **Model Context Protocol**,
the same protocol Claude Desktop, Claude Code, and Cursor speak. Any of those
hosts can plug in and gain one private memory that follows you across every tool
and session. The same store the brain app reads is the one an external agent
writes to — tell one tool something, and the others know it. Nothing is uploaded.

Four tools: `remember`, `recall`, `list_memories`, `forget`.

### 2. Memory turned inward — the mirror *(planned)*

Memory reflected back for self-understanding: the patterns, blind spots, and
"here's what I've noticed about you" that only a system with long, private
memory can offer. Design in progress.

---

## The three modes

The app ships in editions that bundle different modes. The **only** difference
between editions is which modes are offered — the entire engine is shared.

| Edition | Modes |
|---|---|
| **full** (`main`) | Secretary · Tutor · Business |
| **student** | Tutor |
| **business-secretary** | Secretary · Business |

### Secretary

A personal assistant that anticipates instead of archiving. Captures your
calendar and daily context, surfaces a **brief** of what you should know now, and
remembers the human things a good assistant just knows — how you like your
emails, who your CFO is, which colleague needs a gentle tone. Every outbound
action passes through a **confirmation gate** before it happens.

### Tutor

Turns your vault into learning. Summarizes what you've captured, generates
**smart questions** and **diagnostic quizzes** (AP Chemistry, AP Physics C, AP
Calculus BC presets), builds **spaced-repetition flashcards**, and — with a
hotkey — records a study session, transcribes the screen to text, and files it
as supporting material. When it detects studious activity on screen, it can take
notes automatically.

### Business

The analyst that replaces routine secretarial finance work. Reads **Excel files
and spreadsheets**, **verifies finances**, builds **expense reports** and
**competitor analyses**, produces **revenue/profit/margin forecasts** and
presentation prep, and pulls live data from **MCP servers** (dashboards,
databases) to summarize trends. All computation is deterministic Go; the model
only narrates. A guarded agent harness runs multi-step business tasks behind the
same confirmation gate.

---

## How the memory works

Two tiers, mirroring how people remember:

| | Episodic | Semantic |
|---|---|---|
| Store | SQLite (`.brain/index.db`) | Obsidian vault (`.md`) + memory store |
| Content | app focus, URLs, files, commits, calendar | preferences, people, context, facts |
| Lifetime | rolled up, then pruned (~90d) | permanent |

The persistent-memory loop is **extract → store → recall**:

- **Extract** durable facts from a conversation (preferences, people, standing
  context, plain facts).
- **Store** with **semantic dedup** — the model paraphrases the same fact
  differently each time, so near-identical memories (≥0.87 cosine) reinforce the
  existing one instead of piling up twins.
- **Recall** with **hybrid retrieval**: vector similarity fused with BM25 lexical
  matching via reciprocal rank fusion, blended with **effective salience** (decay
  + reinforcement) so what's relevant *and* what currently matters both surface.

Memories decay, reinforce when used, consolidate duplicates, and supersede stale
facts — memory that gets exercised persists; memory that never helps fades.

### Benchmarks

Measured against **LongMemEval** (ICLR 2025), the standard long-term-memory
benchmark for chat assistants:

- **96.0% recall@5**, **99.2% recall@10** over the full 500-question LongMemEval-S.
- Hybrid retrieval beats vector-only by **+8.9 points**; the hardest
  single-session case went from 64% → 100%.
- Extract→recall pipeline: **100% accuracy**, **zero dedup growth** on re-learn,
  ~2.7s per conversation to extract, ~30ms per query to recall.

---

## Under the hood

- **Go + Wails** — a frameless, aesthetically-tuned menubar widget app.
- **Pure-Go SQLite** (`modernc.org/sqlite`, no cgo), single-connection.
- **Tiered models**: T0 embeddings (`nomic-embed-text`), T1 fast extract/classify
  (`gemma3:4b`), T2 reasoning (`qwen3.6`), T3 optional cloud (off until confirmed).
  Tiers degrade gracefully when a model is missing.
- **Constrained JSON decoding** rather than tool-calling, for reliability on
  small local models.
- **MCP** both ways: a client (to pull business data in) and a server (to serve
  memory out).

---

## Status & roadmap

**Shipped:** two-tier memory with hybrid recall, the extract→store→recall loop,
Secretary/Tutor/Business modes, the confirmation-gate trust loop, benchmark
harness, and the **MCP memory server** across all three editions.

**Next:** the mirror — memory turned inward for self-understanding — and
continued sharpening of the niche around **local, private, persistent memory**
rather than competing in the crowded agentic-work space.
