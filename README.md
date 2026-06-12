# Digital Loved One 💛

A **memory workbench for preserving a person's voice, personality, and history** —
upload their messages, transcripts, and notes, and the system builds a grounded
persona you can talk to. Built around one hard constraint: **it must say
"I don't know" rather than make things up.** The UI is in Chinese.

> A device-first persona runtime. Not a chatbot that improvises — a retrieval system
> that refuses to answer when the memory isn't there.

## Why it's interesting

- **Grounding-before-inference invariant** — every response passes through a grounding
  gate (`grounding.Layer.ValidateForInference`). If retrieval is insufficient or
  sources conflict, the system is *blocked* from generating and returns "I don't know."
  This is the core honesty guarantee, enforced in code, not prompt-only.
- **Evidence stays separate from interpretation** — raw `SourceExcerpt`s are stored
  verbatim and linked to `TopicNode`s; conflicts between sources are detected and
  surfaced rather than silently averaged.
- **Versioned memory** — topic nodes are archived on each update (`_v1`, `_v2`, …),
  so the persona's knowledge has history, not just current state.
- **Swappable persistence** — all code works through a `memory.Store` interface;
  the default writes JSON files, with an ObjectBox engine behind a build tag.

## Architecture

```
Upload / Chat UI  →  /api/upload, /api/chat, /api/remember
     ↓
cmd/server           HTTP shell (no framework)
     ↓
ingestion.Pipeline   parse → extract excerpts → detect conflicts → update topics
     ↓
memory.Store         interface boundary (default: GraphStore JSON files)
     ↓
grounding.Layer      validates sufficiency, surfaces conflicts, annotates citations
     ↓
response             grounded text (LLM generation gated on the grounding result)
```

### Key packages

| Package | Role |
|---|---|
| `schema` | Shared types: `PersonaProfile`, `TopicNode`, `SourceExcerpt`, `ConflictMarker` |
| `memory` | `Store` interface + `GraphStore` (one JSON file per entity) |
| `ingestion` | Parses WhatsApp / WeChat / audio / text into excerpts; conflict detection |
| `grounding` | Pre-inference gate: retrieval sufficiency, conflicts, confidence score |
| `conflict` | Detects factual / belief contradictions between excerpts |
| `inference` | Voice gateway (STT / LLM / TTS) |
| `cmd/server` | HTTP handlers |

## Tech stack

| Layer | Stack |
|---|---|
| Backend | Go (standard library HTTP, no framework) |
| Storage | JSON GraphStore (default) or ObjectBox (build tag) |
| Frontend | React + Vite |
| LLM | Anthropic Claude (via `ANTHROPIC_API_KEY`) |

## Run locally

```bash
# Backend (default JSON store, data written to ./data)
cd backend
cp ../.env.example .env        # add ANTHROPIC_API_KEY
STORE_PATH=./data go run ./cmd/server      # http://localhost:8080

# Frontend (separate terminal)
cd frontend
npm install
npm run dev                    # connects to http://localhost:8080 (VITE_API_BASE)
```

ObjectBox store (optional, requires the build tag + native runtime):

```bash
cd backend
STORE_ENGINE=objectbox STORE_PATH=./objectbox-data go run -tags objectbox ./cmd/server
```

## Tests

```bash
cd backend
go test ./...
go test ./memory/...           # single package
```

## Notes

- Local memory under `data/` / `objectbox-data/` is gitignored — the repo ships the
  engine, not anyone's personal data.
- No API keys are committed; `.env` is ignored and `.env.example` documents what's needed.
- LLM response generation is gated on the grounding layer; template responses are used
  where generation is not yet wired.
