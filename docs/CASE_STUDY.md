# Case study: making an AI refuse to make things up

A short write-up of the core design decision behind `digital-loved-one`: how to
build a memory-companion AI whose defining feature is that it *won't* fabricate.

## Background

The product preserves a person's voice, personality, and history so their family
can talk to a grounded persona. The failure mode that kills this kind of product
is not a clumsy sentence — it is **a confident, fabricated memory**: the model
inventing an event, opinion, or relationship that never existed. For a grief- and
memory-adjacent product, that is not a bug, it is a betrayal of trust.

So the central requirement was inverted from a normal chatbot: the system must be
*willing to say "I don't know"* far more often than it guesses.

## The problem

"Don't hallucinate" is easy to say and hard to enforce. Prompt instructions
("only answer from the provided context") reduce fabrication but do not *guarantee*
it — the moment retrieval is thin or sources disagree, an LLM will still produce a
fluent, plausible, wrong answer. I wanted the honesty constraint to live in code,
not in a prompt that the model can drift away from.

## Decisions

**A grounding gate before every inference.** `grounding.Layer.ValidateForInference`
must run before any LLM call. It checks retrieval sufficiency and surfaces
unresolved conflicts, producing a `ValidationResult`. If `Blocked == true`, the
caller is required to return "I don't know" — generation never happens. The
honesty guarantee is a control-flow invariant, not a suggestion.

**Keep evidence separate from interpretation.** Source material is stored as
verbatim `SourceExcerpt` records linked to `TopicNode`s. A dedicated `conflict`
package detects factual/belief contradictions between excerpts and *surfaces* them
rather than silently averaging two incompatible memories into one smooth lie.

**Make memory auditable and versioned.** Topic nodes are archived on every update
(`_v1`, `_v2`, …), so the persona's knowledge has a history you can inspect — you
can see what it knew and when, instead of an opaque current state.

**Isolate persistence behind an interface.** Everything outside `memory/` works
through a `memory.Store` interface. The default `GraphStore` writes one JSON file
per entity (easy to read, diff, and back up); an ObjectBox engine sits behind a
build tag for when scale matters. Storage choices never leak into analysis code.

**No framework.** The HTTP layer is Go standard library only. For a system whose
value is in its data model and its honesty constraint, a framework would add
surface area without buying anything.

## Result

- An AI companion where the anti-hallucination property is **enforced structurally**:
  insufficient or conflicting evidence blocks generation, by construction.
- Clean separation of concerns — `ingestion`, `memory`, `grounding`, `conflict`,
  `inference` — each replaceable in isolation.
- A persistence boundary that let me start with plain JSON files and keep a path to
  a real embedded database without touching the rest of the system.

## What I'd do next

Wire the gated LLM generation behind the grounding layer (it currently returns
template responses where generation isn't yet connected), and add retrieval-quality
metrics so "sufficiency" is measured, not just thresholded.
