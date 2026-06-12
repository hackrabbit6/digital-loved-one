# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

### Backend (Go)
```bash
# Run dev server (default JSON store, data written to ./data)
cd backend && STORE_PATH=./data go run ./cmd/server

# Run with ObjectBox store (requires build tag + native runtime)
cd backend && STORE_ENGINE=objectbox STORE_PATH=./objectbox-data go run -tags objectbox ./cmd/server

# Run tests
cd backend && go test ./...

# Run a single package's tests
cd backend && go test ./memory/...
```

### Frontend (React + Vite)
```bash
cd frontend && npm install
cd frontend && npm run dev      # dev server on 0.0.0.0
cd frontend && npm run build    # outputs to frontend/dist/
```

The frontend connects to `http://localhost:8080` by default (overridable via `VITE_API_BASE` env var or the in-app "API 地址" field).

## Architecture

The system is a **device-first persona runtime** — a memory workbench for preserving and querying a deceased person's voice, personality, and history. The UI is in Chinese.

### Data flow
```
Upload / Chat UI  →  /api/upload, /api/chat, /api/remember
     ↓
cmd/server/main.go   (HTTP shell, no framework)
     ↓
ingestion.Pipeline   (parse → extract excerpts → detect conflicts → update topics)
     ↓
memory.Store         (interface boundary; default: GraphStore JSON files)
     ↓
grounding.Layer      (validates sufficiency, surfaces conflicts, annotates citations)
     ↓
response text        (LLM generation is a TODO; currently template responses in Chinese)
```

### Key packages

| Package | Role |
|---|---|
| `schema` | All shared types: `PersonaProfile`, `TopicNode`, `SourceExcerpt`, `ConflictMarker`, `ValidationResult` |
| `memory` | `Store` interface + `GraphStore` (JSON files, one file per entity in `data/{personas,excerpts,topics}/`) |
| `ingestion` | Parses WhatsApp, WeChat, audio, plain text into `SourceExcerpt` records; runs conflict detection; groups into `TopicNode`s |
| `grounding` | Pre-inference gate: checks retrieval sufficiency, surfaces unresolved conflicts, calculates confidence score |
| `conflict` | Detects factual/belief contradictions between excerpts |
| `persona` | Chat engine stub — will become the Agent controller |
| `inference` | Voice gateway placeholder for STT/LLM/TTS |
| `cmd/server` | HTTP handlers; response generation lives here until `persona.Engine` is ready |

### Memory model
- `SourceExcerpt` — verbatim text or transcript chunk, linked to one or more `TopicNode`s
- `TopicNode` — topic area (e.g., `family`, `work`) that aggregates excerpts; archived on each update (`_v1`, `_v2`, …)
- `PersonaProfile` — root entity; holds voice and speech pattern descriptors

### Persistence boundary
All code outside `memory/` works exclusively through `memory.Store`. The default `GraphStore` writes JSON to `./data/{personas,excerpts,topics}/`. The ObjectBox path (tagged files `objectbox_entities.go`, `objectbox_store.go`, `factory_objectbox.go`) requires generated bindings and is not active by default.

### Grounding layer invariant
`grounding.Layer.ValidateForInference` must be called before any LLM call. If `ValidationResult.Blocked == true`, the caller must return "I don't know" rather than hallucinate. This is the core honesty constraint of the system.
