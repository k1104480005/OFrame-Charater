## Purpose

Defines the character identity package, the root object of the whole workflow: a versionable local directory + manifest container that holds a character's identity definition, logical canvas, anchors, and all generated content and metadata.

## ADDED Requirements

### Requirement: Create a character identity package
The system SHALL allow creating a new character identity package from the launch page as a local directory with a manifest.

#### Scenario: Create identity package from launch page
- **WHEN** the user chooses "create new identity package" on the launch page and enters a name
- **THEN** the system creates a local directory and manifest for the package and opens it in the workbench

### Requirement: Open an existing character identity package
The system SHALL allow opening an existing character identity package located in the workspace from the launch page.

#### Scenario: Open existing identity package
- **WHEN** the user selects an existing identity package on the launch page
- **THEN** the system loads the package manifest and its content and enters the workbench with that package

#### Scenario: Open corrupted identity package
- **WHEN** the selected package directory is missing its manifest or the manifest cannot be parsed
- **THEN** the system reports an error and refuses to enter the workbench without modifying the package

### Requirement: Identity package manifest
Each identity package SHALL contain a manifest that carries a format version, identity metadata, logical canvas specification, anchor definitions, asset references, and references to candidate history and operation logs.

#### Scenario: Manifest format version compatibility
- **WHEN** the user opens a package whose manifest format version is newer than the application supports
- **THEN** the system refuses to open it, shows a migration hint, and does not modify the package

### Requirement: Identity definition entry points
The identity package SHALL support text description, reference images, and existing sprites as entry points for the character identity definition.

#### Scenario: Enter text description
- **WHEN** the user enters a text description on the identity sub-page
- **THEN** the description is saved into the identity package metadata and becomes part of the identity definition

#### Scenario: Add reference image
- **WHEN** the user adds a reference image as material
- **THEN** the image is stored in the package material area and can be referenced from the identity sub-page

#### Scenario: Import existing sprite
- **WHEN** the user imports an existing sprite as the identity entry point
- **THEN** the sprite is stored as material and can be used as the basis for the identity definition

### Requirement: Logical canvas specification
The identity package SHALL define a single logical canvas specification (unit size and coordinate range) that all motions and direction sets share.

#### Scenario: Set logical canvas
- **WHEN** the user sets the logical canvas size for the identity
- **THEN** the specification is written to the manifest and becomes the reference for generation, slicing, and preview of all motions

#### Scenario: Enforce logical canvas on motions
- **WHEN** a motion or frame sequence does not conform to the logical canvas specification
- **THEN** the system marks the inconsistency and prevents it from entering acceptance or export

### Requirement: Anchor definitions and presets
The identity package SHALL support defining anchors (such as feet and hand points) with presets at the identity level, reusable by motions and direction sets.

#### Scenario: Define anchors
- **WHEN** the user defines or edits anchors for the identity
- **THEN** the anchors and their coordinate range are saved to the manifest and can be referenced by motions and direction sets
