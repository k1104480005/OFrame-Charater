## Purpose

Defines lightweight editing as the minimal closed loop for deterministic correction: frame-level, sequence-level, anchor-level, and batch capabilities, with all edits recorded as replayable edit instructions for non-destructive storage.

## ADDED Requirements

### Requirement: Frame-level editing
The system SHALL support frame-level editing: cropping, transparent background cleanup (edge/remnant removal), and pixel-level replacement (eraser, color picker, single-pixel brush).

#### Scenario: Pixel-level replacement
- **WHEN** the user edits a frame with the eraser or brush
- **THEN** the edit is recorded as an instruction and the original frame data is not destroyed

#### Scenario: Transparent background cleanup
- **WHEN** the user runs background cleanup on a frame
- **THEN** the background is cleaned per the instruction and an alpha-check view is available for inspection

### Requirement: Sequence-level editing
The system SHALL support sequence-level editing: frame order adjustment, inserting and deleting frames, and frame duration (rhythm) adjustment.

#### Scenario: Adjust frame order
- **WHEN** the user changes the order of frames in a sequence
- **THEN** the sequence is saved in the new order with its per-frame durations preserved

### Requirement: Anchor-level editing
The system SHALL support dragging anchors to adjust them, applied to a single frame or to a whole direction set.

#### Scenario: Apply anchor change to a direction set
- **WHEN** the user applies an anchor adjustment to the whole direction set
- **THEN** the anchors of all corresponding frames in the set are updated by the rule

### Requirement: Batch editing
The system SHALL support applying the same correction (such as uniform background removal) to all frames within a direction set.

#### Scenario: Batch background removal
- **WHEN** the user applies a uniform background removal to all frames of a direction set
- **THEN** all frames receive the same instruction

### Requirement: Replayable edit instructions
All edits SHALL be recorded as replayable edit instructions, and edited results SHALL be able to re-enter quality scoring.

#### Scenario: Re-enter quality scoring after edit
- **WHEN** editing completes
- **THEN** the edited result can be re-submitted to quality scoring

#### Scenario: Replay edit history
- **WHEN** the edit history needs to be replayed
- **THEN** the current result is reproduced by replaying the append-only instruction sequence
