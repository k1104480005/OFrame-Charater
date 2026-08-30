## 1. Core Domain And API

- [x] 1.1 Add versioned base-character candidate data and identity adoption semantics, preserving old identity packages; verify with Go compatibility and adoption tests
- [x] 1.2 Add shared-service preparation and execution for base-character generation using existing provider capability, confirmation, budget, task, and deterministic pipeline rules; verify cancellation, no-provider, and successful-plan tests
- [x] 1.3 Expose bindings and frontend API views for base-character candidates, adoption, current work context, and generation progress; verify binding tests and generated frontend types/build

## 2. Shared Workbench Context

- [x] 2.1 Add a session-level current context for identity, motion, direction, candidate, accepted asset, and preview controls; verify context survives view changes and package reload
- [x] 2.2 Replace duplicated motion/direction selection with context-driven selectors while preserving deep links from acceptance, edit, and export; verify one selection updates all views
- [x] 2.3 Preserve unsaved drafts and clearly expose dirty state during view switching and session restore; verify draft recovery tests and GUI behavior

## 3. Character Creation And Generation UX

- [x] 3.1 Restore the perfectpixel-style base-character creation panel with description, style, reference upload/drop, multiple candidate tasks, preview, and adopt action; verify empty, loading, error, and success states in the frontend build
- [x] 3.2 Connect animation generation to the shared context and automatically focus the generated candidate in the preview/acceptance surface; verify task completion updates without manual re-selection
- [x] 3.3 Preserve generation confirmation details for both base-character and animation generation, including provider/model, outbound materials, directions, costs, prompt snapshot, and cancellation; verify no external call on cancellation

## 4. Preview, Acceptance, And Editing

- [x] 4.1 Recompose the workbench into a compact list-plus-preview layout using the existing PixelCanvas, task drawer, and identity summary; verify responsive layout and frontend build
- [x] 4.2 Integrate verified perfectpixel interactions: playback, pause/replay, frame scrubber, FPS, zoom, direction grid, frame selection, and feedback regeneration; verify preview controls with representative frame fixtures
- [x] 4.3 Keep candidate history, quality scores, warnings, accept/reject, replacement, rollback, and operation log attached to the current context; verify Go and GUI acceptance scenarios
- [x] 4.4 Make lightweight frame/sequence/anchor editing operate on the selected context object and refresh preview without losing selection; verify edit and rollback tests

## 5. Export And Compatibility

- [x] 5.1 Drive export inspection from the shared accepted-asset context and preserve the accepted-current-version-only rule; verify export validation rejects pending/rejected assets
- [x] 5.2 Verify old identity packages, existing generated assets, CLI generation, task recovery, and provider settings remain usable; run `go test -count=1 ./...`

## 6. Visual And System Verification

- [ ] 6.1 Flatten navigation into identity/action/acceptance/export stages, align theme tokens and controls with the workbench visual direction while retaining FrameBaker branding and accessible labels; verify screenshots at desktop and narrow viewport sizes
- [x] 6.2 Run frontend production build and package build, then verify the exact packaged GUI URL/build behavior after refresh
- [ ] 6.3 Complete real GUI acceptance for base-character creation, animation generation, no-provider boundary, unsaved drafts, context switching, candidate decisions, editing, rollback, and export; record results in the change checklist
- [x] 6.4 Review changed files and local git diff; do not push or commit unless the user explicitly requests it
- [x] 6.5 Add immutable mutually exclusive base-character source selection (AI vs local import), legacy inference, backend enforcement, explicit lock confirmation, source-specific UI state, and regression tests
