## Purpose

Defines motions, direction sets, and frame sequences as the structural model of animated behavior, together with the direction strategy (single direction by default, 4/8 directions with automatic mirroring) that keeps generation cost low while producing direction-consistent animation assets.

## ADDED Requirements

### Requirement: Motion and direction set structure
A motion SHALL consist of direction sets, where each direction corresponds to an independent frame sequence.

#### Scenario: Create a motion with a direction strategy
- **WHEN** the user creates a new motion and chooses a direction strategy (single / 4 / 8)
- **THEN** the system creates the motion and initializes the frame sequence list for each direction in the chosen strategy

### Requirement: Frame sequence specification
Frame sequences SHALL be organized according to the identity's logical canvas specification with a fixed frame count, fixed size, and fixed order.

#### Scenario: Frame sequence conforms to logical canvas
- **WHEN** a frame sequence is produced or edited
- **THEN** every frame conforms to the logical canvas unit size and the sequence has a defined count and order

### Requirement: Single direction by default
New motions SHALL default to a single direction — down (south / front-facing, 正面) — so only one generated direction exists.

#### Scenario: Default single-direction motion
- **WHEN** the user creates a new motion without choosing a direction strategy
- **THEN** the motion contains only the single default direction (down / south / 正面)

### Requirement: 4-direction automatic mirroring
When the user chooses 4 directions, the system SHALL issue generation calls only for the basic directions and derive the remaining direction by horizontal mirroring: right (generated) mirrors to left; up and down are generated.

#### Scenario: 4-direction configuration
- **WHEN** the user chooses the 4-direction strategy for a motion
- **THEN** the system generates the right, up, and down directions and derives the left direction as the horizontal mirror of the right direction

### Requirement: 8-direction automatic mirroring
When the user chooses 8 directions, the system SHALL issue 5 generation calls for the basic directions (right, up, down, up-right, down-left) and derive the other 3 directions (left, up-left, down-right) by horizontal mirroring. The mirror mapping is explicit and one-way (复核报告语义): right mirrors to left, up-right mirrors to up-left, and down-left mirrors to down-right — down-right is derived one-way from down-left, so down-left belongs to the basic (generated) set; up and down are self-symmetric under horizontal mirroring (they never derive a different direction).

#### Scenario: 8-direction configuration
- **WHEN** the user chooses the 8-direction strategy for a motion
- **THEN** the system generates 5 basic directions (right, up, down, up-right, down-left) and derives the remaining 3 directions (left, up-left, down-right) by horizontal mirroring

### Requirement: Mirroring can be disabled
The system SHALL support disabling automatic mirroring per motion; when disabled, ALL directions of the chosen strategy are generated independently and no direction is mirror-derived.

#### Scenario: Disable automatic mirroring
- **WHEN** the user disables automatic mirroring for a motion
- **THEN** every direction of the chosen strategy is generated independently

### Requirement: Mirror directions have independent frame sequences with converted anchors
Mirrored directions SHALL have their own independent frame sequences, and their anchors SHALL be converted according to the horizontal mirroring rule.

#### Scenario: Mirror anchor conversion
- **WHEN** the system derives a mirrored direction
- **THEN** an independent frame sequence is created for that direction and its anchors are converted by the horizontal mirror rule

#### Scenario: Manual replacement of a mirrored direction
- **WHEN** the user replaces a mirrored direction with a manually generated one during acceptance
- **THEN** the replacement is counted in the generation confirmation's expected call count and the direction set is updated with the replacement frames

### Requirement: Frame timing
Frame sequences SHALL support per-frame display duration (rhythm) metadata.

#### Scenario: Adjust frame timing
- **WHEN** the user adjusts the frame duration of a sequence
- **THEN** the durations are saved in the frame sequence metadata and preview playback follows the new rhythm
