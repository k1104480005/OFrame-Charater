## Purpose

Defines the PerfectPixel filmstrip deterministic pipeline: normalizing a motion's frame sequence against the logical canvas, generating the whole filmstrip in one prompt for inter-frame consistency, deterministically slicing and correcting it, and producing PixelPerfect-previewable frames that match what the engine will consume.

## ADDED Requirements

### Requirement: Frame list normalization
The pipeline SHALL normalize a motion's frame sequence into a fixed-length, fixed-order frame list based on the logical canvas specification.

#### Scenario: Normalize frame list
- **WHEN** the pipeline starts processing a motion
- **THEN** it produces a frame list with fixed count, size, and coordinates that conform to the logical canvas

### Requirement: Filmstrip generation in a single call
The pipeline SHALL arrange all frames of the same motion side by side in frame order into one horizontal filmstrip and generate it with a single prompt.

#### Scenario: Single-call filmstrip generation
- **WHEN** a motion contains multiple frames
- **THEN** the system generates one horizontal filmstrip containing all frames in a single generation call

### Requirement: Deterministic slicing
The pipeline SHALL slice the filmstrip into independent frames at normalized coordinates, using integer-pixel-level geometric transforms without secondary interpolation blur.

#### Scenario: Slice filmstrip into frames
- **WHEN** the filmstrip is ready
- **THEN** the pipeline slices it into independent frames at normalized coordinates, with scaling and displacement at integer pixel level

#### Scenario: Slicing failure
- **WHEN** the slicing result does not match the specification (missing frames or wrong sizes)
- **THEN** the task fails with the recorded reason and no partial assets are produced

### Requirement: Deterministic correction
The pipeline SHALL automatically correct anchors to normalized coordinates, handle transparent background cleanup, and align cropping and padding to the logical canvas.

#### Scenario: Anchor correction
- **WHEN** anchors deviate from normalized coordinates after slicing
- **THEN** the pipeline corrects the anchors by rule

#### Scenario: Transparent background handling
- **WHEN** frames contain residual background
- **THEN** the pipeline cleans the transparent background, with magenta used only as the technical background in matting preview views

### Requirement: PixelPerfect preview
The pipeline SHALL provide playback of sliced frames with nearest-neighbor sampling on the canvas so that what is previewed is what the engine consumes, with optional grid overlay and anchor visualization.

#### Scenario: Pixel-perfect preview playback
- **WHEN** the user plays back frames on the acceptance tab
- **THEN** the canvas renders with nearest-neighbor sampling, and grid overlay and anchor visualization can be toggled

### Requirement: Regeneration after failed acceptance
A candidate that does not pass quality acceptance SHALL be regenerable as a new filmstrip, with the regeneration subject to generation confirmation.

#### Scenario: Regenerate candidate
- **WHEN** a candidate fails quality acceptance and the user chooses to retry
- **THEN** the system initiates a new filmstrip generation under the generation confirmation rules
