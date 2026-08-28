## Purpose

为每个 Provider 建立图片、视频和文本模型的独立能力目录，支持设置阶段管理并为后续视频生成和抽帧流程提供稳定选择依据。

## ADDED Requirements

### Requirement: Separate model capability catalogs
系统 SHALL 为每个 Provider 独立保存图片模型、视频模型和文本模型列表；同一模型可以按实际能力出现在一个或多个分类中。

#### Scenario: Save categorized models
- **WHEN** 用户在 Provider 卡片中填写并保存图片、视频和文本模型
- **THEN** 系统分别持久化三个列表，重新打开设置时不混合、不丢失分类。

### Requirement: Filter models by task capability
系统 SHALL 在生成或视频抽帧选择模型时只展示满足当前任务能力的模型，并明确提示没有可用模型的原因。

#### Scenario: Select a video model
- **WHEN** 用户为视频抽帧任务选择 Provider
- **THEN** 系统只提供该 Provider 的视频模型，若列表为空则提示需要先配置视频模型。

### Requirement: Model list management
系统 SHALL 支持从 Provider 获取模型列表、按名称过滤、按图片/视频/文本分类并通过点选加入或移除模型，且保留用户手动输入的模型。

#### Scenario: Filter and classify discovered models
- **WHEN** 用户获取模型列表并输入过滤词后点击模型
- **THEN** 界面只显示匹配项，点击模型会在当前能力列表中切换加入/移除状态。

### Requirement: Stable model selection
系统 SHALL 在生成请求中传递用户明确选择的 Provider 和模型；若选择失效或不属于当前能力目录，系统 SHALL 在外部调用前阻止请求并显示原因。

#### Scenario: Reject an invalid model selection
- **WHEN** 生成请求引用了不存在或不具备当前任务能力的模型
- **THEN** 系统不发起外部调用，并返回 Provider 与模型能力不匹配的错误。
