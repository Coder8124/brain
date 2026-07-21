# Credits & inspiration

`brain` is its own codebase, but several features were inspired by prior open
work. All of the projects below are MIT-licensed; we reimplemented the *ideas*
in Go rather than copying code, and we're grateful for the thinking.

## [mempalace/mempalace](https://github.com/mempalace/mempalace) — MIT
An AI memory system that organises content spatially: people and projects are
*wings*, topics are *rooms*, content lives in *drawers*, so search can be scoped
instead of flat. Also argues for **verbatim preservation** over lossy
summarisation.

**What it inspired here:** scoped retrieval — searching within a folder/kind
(`people/`, `projects/`, `topics/`) rather than the whole vault — and keeping a
verbatim capture path (braindump) alongside the distilled notes.

## [huytieu/COG-second-brain](https://github.com/huytieu/COG-second-brain) — MIT
"Cognition + Obsidian + Git." Markdown-first, no database lock-in, with braindump
auto-classification, weekly pattern analysis and monthly consolidation, and a
verification-centric stance (sources required, confidence stamped).

**What it inspired here:** the **braindump** quick-capture command
(`brain jot`), which classifies a scrap of text and routes it into the review
queue. Our vault-as-truth / DB-as-cache stance and confidence-stamped edges are
kindred ideas we'd landed on independently.

## [henrydaum/second-brain](https://github.com/henrydaum/second-brain) — MIT
A local-first microkernel runtime that indexes local files and does **hybrid,
lexical, and semantic search** with citations, plus scheduled recurring tasks.

**What it inspired here:** upgrading retrieval from pure cosine similarity to
**hybrid search** — SQLite FTS5 lexical matching fused with vector similarity by
reciprocal rank fusion — so exact terms (names, error codes, IDs) are found as
reliably as concepts.

## [TurboLearn AI / Turbo AI](https://www.turbo.ai/) — commercial product
Turns lectures, PDFs and videos into notes, **flashcards**, and **quizzes**, with
a study-activity review flow.

**What it inspired here:** the tutor flavor's **spaced-repetition flashcards** —
generated from vault notes, scheduled with an SM-2 review algorithm — building on
the quiz generation we already had. (Reimplemented from the public feature
description; no TurboLearn code is used, and it is a closed-source product.)
