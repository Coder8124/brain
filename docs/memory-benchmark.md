# Memory benchmark — LongMemEval

We evaluate the persistent-memory retrieval against **LongMemEval** (Wu et al.,
ICLR 2025) — the standard long-term-memory benchmark for chat assistants —
using its `longmemeval_s` split (each question carries a ~115K-token history of
~53 sessions, only a few of which hold the answer).

**Metric:** retrieval recall@k — for each question, embed every history session
and the question with the local `nomic-embed-text` model, rank by cosine, and
count a hit when an evidence session (`answer_session_ids`) lands in the top k.
This isolates the memory backend (no reranker, no LLM, no cloud). It is the same
metric other local memory systems report.

**Result** (recall@5, 90 instances stratified across all categories):

| Category                  | Recall |  n |
|---------------------------|-------:|---:|
| **OVERALL**               |  86.7% | 90 |
| knowledge-update          | 100.0% | 15 |
| multi-session             |  92.6% | 27 |
| single-session-assistant  | 100.0% |  1 |
| single-session-preference |  83.3% |  6 |
| temporal-reasoning        |  85.2% | 27 |
| single-session-user       |  64.3% | 14 |

`knowledge-update` — where a later fact supersedes an earlier one — hits 100%,
which is what the supersede/consolidate machinery is for. `single-session-user`
(one buried user statement) is the weak spot and the place a reranker would help
most.

Reproduce:

```
# dataset: huggingface.co/datasets/xiaowu0162/longmemeval-cleaned
brain bench memory longmemeval_s_cleaned.json --n 90 --k 5
```
