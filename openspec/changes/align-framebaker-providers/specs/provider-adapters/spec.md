## Purpose

为 OFrame 提供可扩展且协议明确的图像、视频和文本生成 Provider，使设置中的每个预设都对应真实可验证的连接、模型和请求行为。

## ADDED Requirements

### Requirement: Provider presets represent supported protocols
系统 SHALL 在设置中提供 OpenAI、百炼、banana/Gemini、MiniMax、火山方舟/豆包、自定义 CLI 和自定义 API 预设，并为每个预设显示其协议类型和配置状态。系统 SHALL 额外提供 Agnes 专项预设（多模态：图像 + 文本；端点为占位，接入真实服务前不可用）。

#### Scenario: Add a protocol preset
- **WHEN** 用户点击一个快捷预设
- **THEN** 系统创建一张带有该协议默认字段、说明和模型能力分类的 Provider 卡片，且不要求立即填写密钥。

### Requirement: Protocol-specific generation routing
系统 SHALL 根据 Provider 类型使用对应的认证方式、请求路径、请求体和响应解析规则，不得把一种协议静默当作另一种协议调用。

#### Scenario: Route a supported API request
- **WHEN** 用户确认使用某个已配置 Provider 生成资产
- **THEN** 系统使用该 Provider 类型对应的协议发送请求，并将协议错误以可读原因返回。

### Requirement: Provider connectivity and model discovery
系统 SHALL 支持对当前编辑中的 Provider 配置执行连接测试和模型列表获取，并在成功时返回延迟、可用模型及错误状态。

#### Scenario: Test unsaved provider values
- **WHEN** 用户修改 Base URL 或密钥后点击“测试连接”而未保存
- **THEN** 系统使用当前表单值测试，不读取旧配置，不修改持久化配置。

#### Scenario: Fetch models from current values
- **WHEN** 用户点击“获取模型”
- **THEN** 系统使用当前表单值请求模型列表，并允许用户将返回模型按能力加入草稿。

### Requirement: Provider configuration privacy and persistence
Provider 密钥 SHALL 只保存在本机设置中；Provider 的类型、显示名、接口地址、模型能力和非敏感选项 SHALL 在保存后可在重启中恢复，密钥列表展示不得回显完整密钥。

#### Scenario: Restore provider after restart
- **WHEN** 用户保存 Provider 后重启应用并打开设置
- **THEN** Provider 卡片、类型、模型分类和非敏感配置保持一致，密钥仅显示状态而非原文。
