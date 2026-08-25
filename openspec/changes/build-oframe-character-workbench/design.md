# Design: build-oframe-character-workbench

## Context

This is a greenfield product: there is no existing codebase to integrate with. The scope and motivation come from `PLAN.md` and `CONTEXT.md` at the repository root (see proposal.md — Why for the product motivation). The mandated stack is fixed by the plan:

- **Go core library** holds all domain logic (`identity`, `motion`, `pipeline`, `provider`, `task`, `edit`, `version`, `export` modules).
- **Wails v2** desktop shell (Go), **React + TypeScript** UI, **PixiJS** preview/editing canvas.
- **oframe CLI** (Go, same core) as the scriptable entry point.
- Windows 10/11 x64 first release; local-first, offline-capable core editing, providers only when online.

The domain language (角色身份包, 方向集, 锚点, 生成候选, 候选接受, 身份版本, 生成确认, 逻辑画布, 持久化会话) is defined in `CONTEXT.md` and must be used consistently across specs, design, and implementation. The behavioral contract is fully specified in the delta specs under `specs/` — this document only explains *how*.

## Goals / Non-Goals

**Goals:**

- One core, two entry points: GUI and CLI share the Go core library so behavior cannot drift.
- Frontend stays stateless: React renders and interacts; Go executes all persistence and business rules; Wails bindings are the only communication channel.
- Provider pluggability: a single generation interface with vendor adapters, runtime switchable, with generation confirmation and local call accounting.
- Determinism: PerfectPixel filmstrip pipeline produces integer-pixel-exact frames with no secondary interpolation, so preview == engine output.
- Recoverability: task queue and session state persist across app restarts; interrupted work resumes with one action.
- Versionability: identity package as directory + manifest, immutable identity versions, append-only operation log with rollback.

**Non-Goals:**

- No 3D, skeletal/mesh animation, physics simulation, generic image editor, cloud collaboration, model training, or game-engine runtime (P3 items such as macOS/Linux and batch CLI features are also out of the first-release design).
- No cross-platform packaging in this design (Windows-only first release; platform-neutral core is a design property, not a packaging target).
- No final decision on UI component library details or exact visual token values — the visual system is defined at the theme-token level and fleshed out during implementation.

## Decisions

### D1: Go core library as the single source of truth ("single core, multi entry")
All domain logic lives in the Go core; Wails GUI and oframe CLI are thin entry points over it.
- *Rationale*: guarantees identical behavior between GUI and CLI; centralizes identity package, direction-set, versioning, and export rules in one place; testable headlessly.
- *Alternatives considered*: business logic in the frontend (rejected: two implementations drift; CLI would need a duplicate); a local service daemon (rejected: breaks local-first simplicity and process model).

### D2: Wails v2 as the desktop shell
Go + system integration, mature Windows packaging. React frontend is served by the Wails window; Go bindings expose core operations to the UI.
- *Rationale*: the plan targets Windows first; Wails keeps the whole stack in Go/JS without a second runtime; packaging (NSIS and portable) is well-trodden on Windows.
- *Alternatives considered*: Electron (rejected: heavier, no Go-native core reuse), Tauri (rejected: Rust core would duplicate domain logic).

### D3: Identity package = local directory + versioned manifest
The identity package is a directory plus a manifest carrying format version, identity metadata, logical canvas, anchors, asset references, candidate history, and operation-log references. Format version + migrators handle evolution; a format freeze point is set for the end of P1.
- *Rationale*: directories are backup-friendly, diffable, and archive-friendly; the manifest with a version number gives a migration story (PLAN §12).
- *Alternatives considered*: single binary file (rejected: poor external backup/version management), embedded database (rejected: opaque to users, hard to archive).

### D4: PerfectPixel filmstrip pipeline (normalize → generate → slice → correct → preview)
One prompt generates a horizontal filmstrip of all frames of a motion, guaranteeing inter-frame style/body/rhythm consistency; deterministic integer-pixel slicing and correction produce the final frames.
- *Rationale*: PLAN §4.2 — inter-frame consistency is the core quality problem; a single filmstrip generation avoids per-frame drift; deterministic post-processing keeps results engine-exact.
- *Alternatives considered*: per-frame generation (rejected: consistency poor), video generation then splitting (rejected: less controllable geometry).

### D5: Direction strategy — single default + automatic mirroring
New motions default to one direction; 4/8 directions generate only basic directions and derive the rest by horizontal mirroring, with independent frame sequences and anchor mirror conversion. Mirrored directions may be replaced manually during acceptance (counted in generation confirmation).
- *Rationale*: minimizes billable calls while producing direction-consistent assets; asymmetric details remain user-controlled via replacement.
- *Alternatives considered*: generate all directions independently (rejected: cost multiplies without consistency guarantee).

### D6: Generation confirmation + local accounting + hard retry cap
Any operation that may call a provider first shows expected call count and max retries and waits for confirmation; keys/credits/call statistics are local; retries use backoff up to a hard cap; successful results are cached per task (idempotency).
- *Rationale*: PLAN §12 — cost runaway is a top risk; confirmation and accounting make spend explicit; caching prevents duplicate billing.
- *Alternatives considered*: fire-and-forget with global daily cap (rejected: opaque spend surprises users).

### D7: Recoverable task queue persisted locally
Every generation/correction/export is a task in a local persisted queue (provider params, call counts, status, progress, errors, retry counts). A state machine (queued/running/failed/abandoned) plus persisted session supports one-click resume after crash/shutdown/network failure.
- *Rationale*: PLAN §6 — the product promise is "interrupt and resume in one session"; persistence is the enabler.
- *Alternatives considered*: in-memory queue (rejected: violates the resume promise), remote queue (rejected: local-first).

### D8: Edits as replayable append-only instructions
Lightweight editing (frame/sequence/anchor/batch) records instructions in the append-only log; the current state is derived by replay. Non-destructive by construction; edited results can re-enter quality scoring.
- *Rationale*: ties into versioning (rollback to any historical point) and keeps original data intact.
- *Alternatives considered*: destructive pixel writes with undo stack (rejected: cannot roll back identity package to arbitrary history points).

### D9: Versioning — immutable identity versions + candidate acceptance + operation log
Appearance revisions form immutable identity versions; candidate acceptance is the explicit business action making a candidate the current asset; all changes append to the operation log; rollback restores any historical point while preserving later log entries.
- *Rationale*: PLAN §8 — distinguishes "candidate history" from "current assets" and gives a full audit trail.
- *Alternatives considered*: single mutable state with backups (rejected: no explicit identity version semantics).

### D10: Export as engine-target formats with validation
Export packages target Godot / Unity / generic sprite sheet from a selected identity version; generation is followed by format validation (frames, anchors, manifest) so engines can consume directly; export history is retained.
- *Rationale*: PLAN §1.3/§10 — "directly consumable by engines" is a product-level success criterion; validation makes the guarantee checkable.
- *Alternatives considered*: unvalidated zip dumps (rejected: fails the acceptance criterion).

### D11: UI — stateless React over Wails bindings, PixiJS canvas, theme-token visuals
Three-tab shell (Make/Acceptance/Export) + launch page + settings + task drawer; PixiJS handles playback, anchors, pixel-perfect preview, and the lightweight edit canvas; visual system implemented as CSS/theme tokens (warm-white/dark-ink, 8px grid, pixel borders, status colors, magenta reserved for matting, pixel fonts as accents).
- *Rationale*: frontend statelessness (PLAN §2.2) keeps business rules on the Go side; token-based theming decouples visuals from function (PLAN §10) and enables future re-skinning.
- *Performance considerations*: texture atlas, nearest-neighbor rendering, on-demand frame loading, and degraded scaling modes mitigate PixiJS multi-frame playback cost (PLAN §12).

### D12: oframe CLI over the same core
CLI commands (generate, validate, export) call the same core modules as the GUI; P2 regression runs the same test cases on both entry points.
- *Rationale*: single-core design makes the CLI nearly free and guarantees parity (PLAN §12 drift risk).
- *Alternatives considered*: separate CLI implementation (rejected: drift risk).

## Risks / Trade-offs

- **Provider generation inconsistency** (same character drifts across frames/directions) → filmstrip single-call generation for intra-sequence consistency; automatic mirroring minimizes calls; optional AI coarse consistency score as reference; candidate history preserves retry options.
- **Mirrored directions look unnatural** (weapons, asymmetric clothing) → per-direction manual replacement during acceptance, counted into generation confirmation; mirroring semantics documented.
- **Generation cost runaway** → generation confirmation with expected call count and max retries; local call statistics; idempotent caching; hard retry cap.
- **Wails Windows packaging/integration pitfalls** → minimal packaging smoke test early in P0; NSIS and portable builds validated in parallel.
- **PixiJS performance on many large frames** → texture atlas, nearest-neighbor rendering, on-demand frame loading, degraded scaling modes.
- **Quality scoring subjectivity** → scores are advisory only; human review in the acceptance gate is authoritative; metrics favor quantifiable structural items.
- **Identity package format evolution** → manifest format version + migrators; explicit format freeze point at end of P1.
- **CLI/GUI behavior drift** → shared Go core; P2 runs identical test cases on both entry points.

## Migration Plan

- Greenfield: no data migration from prior systems. The first identity package format version is defined in P0; a migration framework (manifest version + migrators) ships from the start so later format changes are upgrades, not breaks.
- Rollback: because the operation log and immutable identity versions are the source of truth, both data-level rollback (identity package to a historical point) and release-level rollback (older app build reads older manifest versions) are supported; forward-incompatible manifests are refused with a migration hint rather than touched.
- Packaging: NSIS installer and portable build are both produced and smoke-tested from P0; the installer path is the primary, portable the fallback.

## Open Questions

- **Filmstrip size limits**: maximum frame count / filmstrip width per generation call is bounded by the provider's output size and context limits; the concrete limits will be discovered during adapter integration and surfaced as task parameters — they do not change the pipeline design or specs.
- **Provider API specifics**: exact API parameters, rate limits, and pricing structures of Doubao / gpt-image-2 / Agnes will be confirmed during adapter implementation; the provider interface already isolates these.
- **Agnes availability**: whether the Agnes adapter can be wired in practice is validated during P1 integration; it is optional and does not block the default Doubao path.
- **Default logical canvas and frame timing values**: sensible defaults (unit size, per-frame durations) are product-level details settled during UI implementation without affecting specs or the task breakdown.
