## Purpose

让制作、预览、验收、编辑和导出围绕同一个当前资产上下文工作，保留 perfectpixel 已验证的快速浏览和即时反馈体验。

## ADDED Requirements

### Requirement: Share current work context
系统 SHALL 在工作台会话中共享当前身份包、动作、方向、候选和预览对象；切换视图不得清除未保存草稿或当前选择。

#### Scenario: Switch views
- **WHEN** 用户从制作切换到验收或编辑
- **THEN** 系统保留当前动作、方向和候选上下文，并展示对应资产

### Requirement: Preview and control animation
系统 SHALL 提供当前动画的播放/暂停、重播、帧定位、FPS、缩放、网格、锚点和方向集预览，并显示帧数与来源状态。

#### Scenario: Inspect generated animation
- **WHEN** 用户选择已生成的动作方向
- **THEN** 系统在主工作区即时显示可播放的逐帧预览和帧信息

### Requirement: Regenerate with feedback
系统 SHALL 允许用户基于当前候选输入反馈并发起重新生成，且重新生成前必须经过已有确认门。

#### Scenario: Feedback regeneration
- **WHEN** 用户提交反馈重生成
- **THEN** 系统保留原候选历史，生成新的候选计划，并在确认前不发起外部调用
