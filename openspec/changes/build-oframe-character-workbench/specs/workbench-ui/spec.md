## Purpose

Defines the desktop workbench interface: a launch page that only selects or creates identity packages, a fixed three-tab main screen (Make / Acceptance / Export), global settings and a cross-tab task drawer, and a warm-white/dark-ink dual-theme pixel-art visual system.

## ADDED Requirements

### Requirement: Launch page as identity package entry
The launch page SHALL only select or create character identity packages and SHALL NOT carry any editing capability.

#### Scenario: Launch page entry points
- **WHEN** the application starts
- **THEN** the launch page is shown offering only the operations of selecting an existing identity package or creating a new one

### Requirement: Three-tab main screen
The main screen SHALL have a fixed top-level of three tabs: Make (actions and production), Acceptance (quality acceptance), and Export (assets and export packages).

#### Scenario: Switch tabs without losing state
- **WHEN** the user switches between the three tabs
- **THEN** unfinished motions are not lost and all tabs share the same identity package instance

### Requirement: Make tab sub-pages
The Make tab SHALL allow switching among three sub-pages: Identity, Motion, and Edit.

#### Scenario: Switch Make sub-pages
- **WHEN** the user switches sub-pages inside the Make tab
- **THEN** the corresponding Identity, Motion, or Edit content is shown

### Requirement: Acceptance tab content
The Acceptance tab SHALL show quality scores, PixelPerfect preview playback and confirmation, candidate accept/reject, and direction replacement.

#### Scenario: Perform acceptance operations
- **WHEN** the user works on the Acceptance tab
- **THEN** quality scores are displayed, preview confirmation is supported, candidates can be accepted or rejected, and directions can be replaced

### Requirement: Export tab content
The Export tab SHALL show animation asset inspection, engine target selection, export package generation with validation, and export history.

#### Scenario: Perform export flow
- **WHEN** the user works on the Export tab
- **THEN** asset inspection, engine target selection, export generation with validation, and history viewing are available

### Requirement: Global settings
The system SHALL provide a settings entry (gear icon) carrying global configuration: provider selection, keys/credits, and appearance theme.

#### Scenario: Open settings
- **WHEN** the user clicks the gear entry
- **THEN** global settings are shown and can be modified

### Requirement: Cross-tab task drawer
The system SHALL provide a global task drawer visible across tabs, showing running, queued, and failed tasks.

#### Scenario: View tasks from any tab
- **WHEN** the user opens the task drawer from any tab
- **THEN** task statuses are shown, and failed tasks can show their reason and support retry or abandon

### Requirement: Dual-theme pixel visual system
The interface SHALL implement a warm-white / dark-ink dual theme with pixel borders and separators, an 8px spacing grid, status colors reserved for status expression, magenta reserved exclusively for matting technical backgrounds, and pixel fonts used sparingly for accents.

#### Scenario: Switch theme
- **WHEN** the user switches the appearance theme
- **THEN** the interface is redrawn in the warm-white or dark-ink theme accordingly

#### Scenario: Magenta usage restriction
- **WHEN** the interface displays matting-related technical backgrounds (such as background-removal preview or alpha-check views)
- **THEN** magenta is used only for those technical backgrounds and never in regular interface elements
