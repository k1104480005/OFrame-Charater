## MODIFIED Requirements

### Requirement: Global settings
The system SHALL provide a settings entry (gear icon) carrying global configuration: provider selection, keys/credits, and appearance theme. The settings view SHALL provide FrameBaker-aligned Provider management with the seven quick presets (OpenAI, 百炼, banana/Gemini, MiniMax, 火山方舟/豆包, custom CLI, custom API) plus the Agnes specialist preset (multimodal image+text), protocol-specific fields, separate image/video/text model catalogs, current-form connection testing and model discovery, and clear configured/unconfigured states.

#### Scenario: Open settings
- **WHEN** the user clicks the gear entry
- **THEN** global settings are shown and can be modified

#### Scenario: Add and edit a provider preset
- **WHEN** the user adds one of the seven quick presets
- **THEN** a Provider card appears with the correct protocol fields and editable image/video/text model values, while existing Provider cards remain unchanged

#### Scenario: Test a draft provider
- **WHEN** the user edits a Provider card and clicks connection test
- **THEN** the test uses the current unsaved form values and shows success, latency, discovered models, or a readable error inline

#### Scenario: Manage model capabilities
- **WHEN** the user fetches models and assigns them to image, video, or text capability
- **THEN** the selected classifications remain in the draft until save and are restored after reopening settings

### Requirement: Three-tab main screen
The main screen SHALL have a fixed top-level of three tabs: Make (actions and production), Acceptance (quality acceptance), and Export (assets and export packages). Provider and model choices used by production tasks SHALL be preserved when switching tabs.

#### Scenario: Switch tabs without losing state
- **WHEN** the user switches between the three tabs
- **THEN** unfinished motions are not lost and all tabs share the same identity package instance
