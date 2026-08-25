## Purpose

Defines export packages: engine-targeted deliverables (Godot, Unity, generic sprite sheets) generated from a specified identity version, validated so engines can consume them directly, with an export history retained.

## ADDED Requirements

### Requirement: Engine target selection
The system SHALL support Godot, Unity, and generic sprite-sheet target formats for export packages.

#### Scenario: Select an engine target
- **WHEN** the user selects an engine target on the export tab
- **THEN** the export package is generated in that target format

### Requirement: Export package content and asset inspection
An export package SHALL contain the animation assets (frame sequences plus anchor lists), and the export tab SHALL let the user inspect animation assets before exporting.

#### Scenario: Inspect animation assets
- **WHEN** the user views animation assets on the export tab
- **THEN** the frame sequences and anchor lists of accepted assets are displayed

### Requirement: Export generation and validation
The export package SHALL be generated from the selected identity version and validated so it can be consumed directly by the target engine.

#### Scenario: Validate generated export package
- **WHEN** an export package is generated
- **THEN** the system validates format completeness (frames, anchors, manifest); if validation fails the export task fails with the reason stated

### Requirement: Export history
The system SHALL record export operations in an export history.

#### Scenario: View export history
- **WHEN** the user views the export history
- **THEN** previous export records (target, identity version, time, and result) are shown
