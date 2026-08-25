## Purpose

Defines the oframe CLI: a scriptable pipeline that shares the same Go core library as the GUI, supporting batch generation, validation, and export for CI and advanced users, keeping behavior identical between the two entry points.

## ADDED Requirements

### Requirement: Shared core with the GUI
The oframe CLI SHALL reuse the same Go core library as the GUI for all pipeline behavior.

#### Scenario: Consistent behavior between CLI and GUI
- **WHEN** the same input is processed by the CLI and by the GUI
- **THEN** both produce consistent results, verified by running the same test cases on both entry points

### Requirement: Batch generation
The oframe CLI SHALL support scriptable batch generation.

#### Scenario: Trigger generation from the command line
- **WHEN** the user triggers generation with a CLI command and parameters
- **THEN** the pipeline executes accordingly and outputs the result or task status

### Requirement: Validation
The oframe CLI SHALL support validation of identity packages and export packages.

#### Scenario: Validate an identity package
- **WHEN** the user runs the validate command against an identity package
- **THEN** the CLI outputs the validation result (format and completeness)

### Requirement: Export generation
The oframe CLI SHALL support generating export packages.

#### Scenario: Trigger export from the command line
- **WHEN** the user triggers export with a CLI command
- **THEN** the export package is generated and its path and validation result are output
