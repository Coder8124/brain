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

| Category                  | Vector-only | **Hybrid** |  n |
|---------------------------|------------:|-----------:|---:|
| **OVERALL**               |       86.7% |  **95.6%** | 90 |
| single-session-user       |       64.3% | **100.0%** | 14 |
| multi-session             |       92.6% |     100.0% | 27 |
| temporal-reasoning        |       85.2% |      92.6% | 27 |
| knowledge-update          |      100.0% |      93.3% | 15 |
| single-session-preference |       83.3% |      83.3% |  6 |
| single-session-assistant  |      100.0% |     100.0% |  1 |

Hybrid = the vector ranking fused with an in-process **BM25** lexical ranking by
reciprocal rank fusion. The big win is `single-session-user` (64% → 100%): a
buried one-off statement is found by its exact terms, which the embedding blurs.
Overall recall@5 rises **+8.9 points to 95.6%**. (knowledge-update slips by one
instance of 15 — within noise.) Live persistent recall uses the same hybrid path.

Reproduce:

```
# dataset: huggingface.co/datasets/xiaowu0162/longmemeval-cleaned
brain bench memory longmemeval_s_cleaned.json --n 90 --k 5            # hybrid
brain bench memory longmemeval_s_cleaned.json --n 90 --k 5 --vector   # vector-only baseline
```

## Extract → recall pipeline

Beyond retrieval, we test the whole loop the assistant runs live: read a
conversation, extract what is worth remembering (a model call), store it, and
recall it later from a natural question. `brain bench pipeline` seeds a fresh
test vault with sample conversations spanning preferences, people, context and
plain facts, then measures correctness and efficiency.

| Metric | Result |
|---|---|
| extract→recall accuracy | **100%** (6/6 facts survived the round trip) |
| dedup growth on re-learn | **0** (re-learning the same conversations adds nothing) |
| extraction | ~2.7s per conversation (gemma3:4b, T1) |
| recall | ~30ms per query (hybrid over the store) |

The pipeline test earned its keep: the first run exposed that exact-text dedup
let re-learning bloat the store (the model paraphrases each extraction). The fix
is **semantic dedup at write time** — a new memory within `DedupThreshold`
(0.87 cosine) of an existing one reinforces it instead of adding a twin — which
takes re-learn growth to zero while keeping distinct facts.

Reproduce: `brain bench pipeline`
