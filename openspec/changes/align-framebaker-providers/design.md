## Context

See `proposal.md` and the five delta specifications for motivation and observable behavior. The current application has a Go Provider registry with three built-in adapters plus a generic OpenAI-compatible adapter, local JSON settings, Wails bindings, and a React settings panel. Existing image generation and generation confirmation contracts must remain stable while Provider types and future video capability are added.

## Goals / Non-Goals

**Goals:**

- Make the seven FrameBaker quick presets truthful and independently identifiable.
- Keep Provider configuration, runtime adapters, CLI, Wails bindings, and React forms on one shared model.
- Test unsaved drafts without persisting or replacing the active Provider.
- Persist separate image/video/text model catalogs and make them available to future video extraction.
- Preserve old settings, environment-key fallback, generation confirmation, retry caps, and no silent fallback.
- Provide fake-transport and deterministic tests for every new protocol path.

**Non-Goals:**

- Implement the complete video generation or video-to-filmstrip pipeline in this change.
- Add cloud synchronization, remote secret storage, or new npm/Go dependencies.
- Make every Provider support every modality; capability declarations and explicit unsupported errors are required.
- Copy FrameBaker's server architecture or its localization system into the Wails application.

## Decisions

### 1. Use an explicit protocol discriminator

Extend the Provider type with `cli`, `api`, `dashscope`, `gemini`, `minimax`, and `volcengine`. Keep `doubao`, `openai`, and `agnes` as legacy built-in identities where needed, but map each identity to an adapter type explicitly. This prevents a preset from being labeled as one vendor while silently sending another vendor's request shape.

Alternative rejected: treat every preset as generic OpenAI-compatible. This would make banana, MiniMax, and native DashScope/Ark behavior incorrect and would make future video support impossible to validate.

### 2. Store capability catalogs as arrays

Extend persisted Provider settings with `imageModels`, `videoModels`, and `textModels`, while reading legacy singular `model`, `textModel`, and any old model lists as migration fallbacks. Keep the current singular image/text fields as compatibility defaults until all generation call sites use the catalog model selection.

Alternative rejected: encode capability in model-name heuristics only. Names are insufficient for vendor-specific models and cannot express a model that supports multiple capabilities.

### 3. Separate draft RPCs from saved Provider RPCs

Add Wails methods that accept a complete non-secret draft configuration for connection testing and model discovery. The UI will call these methods with the current card values; saved-ID methods may remain as convenience wrappers for existing callers. Draft operations never write settings, change the registry, or change the active Provider.

Alternative rejected: temporarily save the draft and restore it after testing. That risks races, corrupts settings on failure, and violates the user's expectation that testing is non-mutating.

### 4. Keep protocol adapters behind the existing Provider interface

Extend the Provider abstraction with capability metadata and add protocol-specific implementations under `core/provider`. Shared HTTP helpers may handle authentication, response limits, timeout, and common error envelopes, while each adapter owns endpoint paths, request bodies, response parsing, and reference-image rules. Unsupported modalities return explicit errors before external calls.

Alternative rejected: put protocol branching throughout generation service code. That would duplicate routing logic and make GUI/CLI behavior drift.

### 5. Implement CLI through argv, never a shell

Represent custom CLI fields structurally and build an argument slice. Use the existing process execution boundary with context cancellation, output-path validation, and reference-image preflight. Legacy templates remain read-only compatibility input and are not exposed as the preferred editor.

Alternative rejected: execute one interpolated command string. It is unsafe for paths/prompts and makes quoting behavior platform-dependent.

### 6. Model selection is capability-aware and fixed before confirmation

Introduce a shared Provider/model selection view for generation forms. It filters configured Provider entries by requested capability, rejects unknown models before preparing an external task, and includes the selected Provider and model in the confirmation snapshot. Video model configuration is persisted now but video execution is gated until the video adapter/pipeline exists.

Alternative rejected: select only the active Provider and silently use its default model. That loses FrameBaker's multi-provider workflow and can cause an unintended billable call.

### 7. Migrate settings conservatively

On load, normalize missing arrays, infer only unambiguous legacy model associations, and retain unknown fields for forward compatibility where the settings format permits. Built-in IDs remain protected. Custom Provider IDs use a validated lowercase slug and collision suffixes. A failed migration leaves the original settings file untouched.

### 8. Verify at three levels

- Go unit tests cover normalization, adapter request/response contracts, validation, CLI argv construction, migration, and fake transport behavior.
- Frontend type/build tests cover card drafts, preset defaults, model filtering, and generated Wails types.
- Manual checks cover all seven presets, unsaved test/model discovery, restart recovery, unsupported capability errors, and no-key/no-network behavior.

## Risks / Trade-offs

- [Protocol drift] Vendor APIs can change or return non-standard envelopes → keep endpoint behavior isolated, cap response reads, report readable errors, and pin fake transport fixtures.
- [Video settings without execution] Users may assume a saved video model is immediately callable → label it as reserved until video support lands and block unsupported video operations before network calls.
- [Legacy settings ambiguity] Old model lists may mix image and video models → migrate only with documented heuristics and let users reclassify ambiguous entries.
- [Secret exposure] Draft test payloads contain keys → keep them in process memory only, never log request bodies, and never return keys in Provider summaries.
- [Large UI surface] Seven protocols create conditional forms → share field components and keep each protocol's required fields explicit.
- [Build reproducibility] Wails embeds `frontend/dist` and the repository tracks a stub entry → use the documented two-step real-frontend build and verify the executable does not contain the placeholder text.

## Migration Plan

1. Add protocol and capability fields with backward-compatible defaults.
2. Normalize existing settings on read; write the new shape only after a successful explicit save.
3. Add adapters and draft RPCs behind existing bindings; keep old Provider IDs and generation requests working.
4. Roll out the expanded settings UI and model selector.
5. Run automated checks, rebuild the real frontend, build portable and NSIS artifacts, and complete the manual Provider checklist.
6. Rollback is configuration-compatible: revert the new binary; existing settings retain legacy fields and are still readable by the prior version. Do not delete or rewrite user settings during rollback.

## Open Questions

- Exact vendor-specific request fields and video endpoint contracts should be finalized against official API documentation when each adapter is implemented; the adapter boundary and capability model do not depend on those details.
