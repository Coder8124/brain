# feat/routine-mining — step 5

"Gleans your daily routine." Mostly **not** an LLM problem.

## The core point

`Slack → IDE at 09:40 ±12min on weekdays, 78% of the time` is a frequency
count over the events table. Prompting a model to notice it is slower, costs
more, and is less reliable than the arithmetic.

The LLM's only job here is to **name** a pattern that mining already found,
and to write it up as prose. Never to find it.

## Mining

Over coalesced sessions:
- **Periodicity** — histogram session starts by (weekday, hour). A tight
  cluster over enough weeks is a routine.
- **Sequence** — frequent app bigrams/trigrams within a window (PrefixSpan, or
  plain n-gram counting; the vault-scale data does not justify anything fancy).
- **Anomaly** — today's shape vs. the trailing 4-week baseline. This is what
  powers "you haven't touched the API repo in 9 days."

Require a minimum support (~5 occurrences over ≥3 distinct weeks) before a
pattern is ever surfaced. Two coincidences are not a routine, and a system that
claims otherwise loses trust fast.

## Output

Routines become notes in `routines/`, and they go through the **same review
queue** as everything else — no privileged write path.

```yaml
type: routine
cadence: weekdays
window: "09:30–10:00"
support: 0.78
observed: 34
relations:
  - { pred: involves, obj: "[[slack]]", conf: 0.9, src: inferred }
```

## Proactive nudges

The highest-risk surface in the whole product. A wrong nudge is worse than no
nudge, and a creepy one is worse than both.

- Never steal focus. Orb state change only, never a modal.
- Rate limit hard: at most a few per day.
- Every nudge is dismissible with "don't tell me this again", and that must
  actually stick.
- Nothing derived from a blocklisted app ever surfaces.

Ship nudges off by default. Let the user turn them on after they trust the
routines the system has already found.

## Done when

`brain routines` lists discovered patterns with support figures, and each one
is recognisably true to the user.
