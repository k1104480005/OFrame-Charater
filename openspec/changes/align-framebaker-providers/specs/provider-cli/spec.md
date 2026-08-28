## Purpose

为需要本地工具或脚本的用户提供与 FrameBaker 一致的自定义 CLI Provider 配置，同时确保提示词、输出、模型和引用图参数以安全且可预测的方式传递。

## ADDED Requirements

### Requirement: Configure a custom CLI provider
系统 SHALL 允许用户配置 CLI 可执行文件、提示词参数、输出参数、模型参数、引用图参数和固定额外参数，并显示配置是否完整。

#### Scenario: Add a CLI preset
- **WHEN** 用户添加“自定义 CLI”预设并填写命令和参数
- **THEN** 系统保存结构化 CLI 配置，并在生成 Provider 选择中显示该 Provider。

### Requirement: Execute CLI without shell interpolation
系统 SHALL 将 CLI 配置转换为独立参数列表执行，不得将用户输入拼接成未经保护的 shell 命令字符串。

#### Scenario: Execute a configured CLI
- **WHEN** 用户确认使用自定义 CLI 生成
- **THEN** 系统以独立参数传递提示词、输出路径、模型和引用图，命令中的空格或特殊字符不会改变参数边界。

### Requirement: Validate CLI reference support
系统 SHALL 在生成前检查引用图参数或模板占位是否存在；配置不支持引用图时，系统 SHALL 在外部调用前拒绝该任务并说明如何修正。

#### Scenario: CLI lacks reference support
- **WHEN** 用户选择了引用图但 CLI Provider 未配置引用图参数
- **THEN** 系统不执行 CLI，并显示缺少引用图参数的原因。

### Requirement: Report CLI failures
系统 SHALL 将 CLI 不存在、退出码非零、输出文件缺失和输出格式无效分别报告为可读错误，并保留任务失败状态。

#### Scenario: CLI exits without output
- **WHEN** CLI 退出成功但没有生成有效输出文件
- **THEN** 系统将任务标记为失败，并提示输出文件缺失或格式无效。
