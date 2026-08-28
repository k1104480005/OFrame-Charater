## Why

当前 OFrame 的 Provider 设置只覆盖少量内置适配器和一个通用 OpenAI 兼容适配器，虽然已有卡片式管理，但与 FrameBaker 的 Provider 工作流仍有明显差距：预设不完整、协议类型没有显式区分、图片/视频/文本模型没有按能力管理，且测试连接和获取模型不能使用用户当前未保存的表单值。随着 OFrame 即将加入视频抽帧动作方案，必须先建立可扩展且清晰的 Provider 配置模型，避免后续再次迁移设置结构。

本变更将以 FrameBaker 的 Provider 体验为参考，完整补齐配置层、设置 UI、协议适配、模型能力管理和后续视频接入所需的基础，同时保持现有 OFrame 的本地优先、生成确认和无静默回退原则。

## What Changes

- 将快捷预设对齐为 FrameBaker 的完整入口：OpenAI、百炼、banana/Gemini、MiniMax、火山方舟/豆包、自定义 CLI、自定义 API，并附加 Agnes 专项多模态（图像+文本）预设（人工验收反馈）。
- 扩展 Provider 类型模型，显式区分 CLI、OpenAI 兼容 API、百炼、Gemini、MiniMax 和火山方舟协议；每种类型使用对应的请求、认证、模型列表和错误处理方式。
- 将 Provider 模型按图片、视频、文本三种能力独立配置、保存和展示，为视频生成及视频抽帧流程预留稳定字段。
- 设置页支持预设添加、Provider 卡片编辑、当前配置值直接测试连接、当前配置值直接获取模型、模型列表过滤和按能力点选。
- 生成入口根据任务媒体类型过滤可用 Provider 和模型；不可用 Provider 或能力不匹配时显示明确原因，不静默切换协议或模型。
- 支持自定义 CLI 的命令、提示词、输出、模型、引用图参数和额外参数配置，并保留安全的参数数组执行方式。
- 支持提示词增强模型关联到已有 Provider 的文本模型；旧版独立凭证配置继续兼容迁移。
- 为每种协议、模型列表、连接测试、配置校验、持久化、重启恢复和错误边界增加 fake transport/单元测试。
- 保留现有三种内置 Provider、自定义兼容 Provider、环境变量密钥回退和已有配置迁移；不改变生成确认前零外呼、重试上限和本地密钥存储原则。

## Capabilities

### New Capabilities

- `provider-adapters`: FrameBaker 对齐的多协议 Provider 适配器、认证、模型获取、连接测试和能力声明。
- `provider-model-catalog`: 图片、视频、文本模型的独立配置、分类、筛选和生成时选择。
- `provider-cli`: 自定义 CLI Provider 的结构化命令配置、安全执行和引用图参数支持。

### Modified Capabilities

- `generation`: Provider 类型、能力过滤、模型选择、视频模型配置和协议路由行为发生变化。
- `workbench-ui`: 设置页预设、Provider 卡片、模型管理、连接测试和生成入口选择器行为发生变化。

## Impact

- Go：`core/provider/`、`core/service/`、Provider 配置持久化、生成服务和 Wails Provider 绑定。
- 前端：`SettingsPanel`、Provider/Model 选择组件、生成表单和设置相关样式与类型。
- CLI：Provider 配置查看、写入、校验和可能新增的类型/模型参数。
- 数据：现有 Provider 配置需要向新的类型和模型能力字段兼容迁移；密钥仍只保存在本机设置中。
- 测试：新增各协议 fake transport、配置迁移、能力筛选、连接测试、模型获取、CLI 参数和重启恢复测试。
- 依赖：不新增 npm 或 Go 依赖；继续使用现有 HTTP 客户端、Wails、React/TypeScript 和本地缓存。
- 发布：设置扩展完成后需要重新执行 Go/前端验证、便携版构建和 NSIS 构建，并更新人工测试清单。
