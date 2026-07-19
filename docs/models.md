# feat/model-router — BYOK, tiering, redaction

`Provider` already speaks one OpenAI-compatible dialect to every runtime. This
branch adds the routing policy on top.

## Tiers

| Tier | Job | Size | Default here |
|---|---|---|---|
| T0 | embeddings | 137M | `nomic-embed-text` |
| T1 | per-event classify / extract | 3–4B | `gemma3:4b` |
| T2 | rollup, entity resolution, ask | 8–24B | `qwen3.6` |
| T3 | weekly synthesis, hard queries | cloud | BYOK, opt-in |

Config in `.brain/config.toml`; per-tier override; missing model falls back to
the next tier down with a warning rather than failing the pipeline.

## Discovery

Port scan already implemented (11434 / 1234 / 1337 / 10000). Extend to:
- probe capabilities, not just presence: does the endpoint honour
  `response_format: json_schema`? what context length does it report?
- cache the probe, re-run on connection failure
- first-run greeting names what it found — "Ollama, 8 models, qwen3.6 for
  reasoning" beats an empty endpoint config box

Note: `gpt-oss:20b` currently fails to load in this Ollama library
(`tensor "blk.0.ffn_down_exps.weight" size overflow`). Capability probe should
catch a model that lists but won't run, and quietly drop it from the tier.

## BYOK

Anthropic, OpenAI, OpenRouter, or any base URL + key. Keys in the macOS
Keychain, never in `config.toml`, never in the vault.

## Redaction gate

Cloud is opt-in **per tier**, and the first time a given tier crosses the
network the user sees exactly what would be sent, with detected entities
highlighted and a per-entity redact toggle. Choice is remembered per tier.

This gate is the whole reason a local-first tool can offer cloud at all. It
must be impossible for vault content to reach a third party because a config
default was wrong.

## Cost and latency

Track tokens and wall time per tier; surface in the panel. A local T2 rollup
taking 40s is fine nightly and unacceptable interactively — the router should
know the difference and pick accordingly.

## Done when

Tiers are configurable, a downed runtime degrades instead of crashing, and no
byte leaves the machine without an explicit per-tier confirmation.
