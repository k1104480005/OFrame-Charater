## Purpose

扩展现有生成契约，使 Provider 类型、任务能力、模型选择和视频模型配置在生成前可验证，同时保留确认、预算、重试和零静默回退原则。

## MODIFIED Requirements

### Requirement: Unified provider interface
The system SHALL expose generation capability (text and image generation) through a unified provider interface, with per-vendor adapters that can be switched at runtime. Provider adapters SHALL additionally declare their supported image, video, and text capabilities, and generation requests SHALL select a model compatible with the requested capability.

#### Scenario: Switch provider at runtime
- **WHEN** the user switches the active provider in settings
- **THEN** subsequent generation calls use the new provider while the previous provider's configuration is preserved

#### Scenario: Select a model by task capability
- **WHEN** a user starts an image, video, or text-capable operation
- **THEN** the provider and model choices contain only configured compatible capabilities, and an unavailable capability prevents the external call with a readable error

### Requirement: Built-in provider adapters
The system SHALL ship adapters for Doubao (default primary), gpt-image-2 (high-quality fallback), and Agnes (specialized supplementary). The system SHALL also support explicitly configured protocol-specific adapters for the FrameBaker-aligned presets without silently substituting a different protocol. No provider is pre-seeded in a fresh installation (人工验收更新: 不固定显示 3 个内置 Provider): users configure providers from the presets, EVERY provider can be removed, a removed built-in identity can be added again under the same id, and generation without any configured provider fails before any external call with a readable error.

#### Scenario: Select an alternative provider
- **WHEN** the user selects a configured protocol-specific Provider in settings
- **THEN** generation calls are routed through that Provider's adapter and its configured model, or fail before the external call if the protocol is incomplete

### Requirement: Generation confirmation before external calls
The system SHALL, before any operation that may cause external calls, present the expected call count and maximum retry count and execute only after the user confirms. Provider and model selection, including a configured video model for future video extraction, SHALL be fixed before confirmation.

#### Scenario: Confirmation includes provider and model
- **WHEN** generation is prepared
- **THEN** the confirmation view identifies the selected Provider, model, capability, expected call count and maximum retry count before any external call

#### Scenario: Cancel confirmation
- **WHEN** the user cancels the generation confirmation
- **THEN** no external call is made and the operation is aborted
