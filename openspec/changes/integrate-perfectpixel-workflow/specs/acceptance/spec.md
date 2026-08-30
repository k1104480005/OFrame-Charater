## Purpose

让用户在生成结果进入工作区后快速比较、判断和修正候选，并通过版本和操作日志保持资产决策可追溯。

## ADDED Requirements

### Requirement: Accept or reject candidates
系统 SHALL 为每个候选显示逐帧预览、方向来源、质量评分、警告和历史，并允许用户接受、拒绝或发起替换。

#### Scenario: Accept valid candidate
- **WHEN** 用户接受候选且候选满足验收约束
- **THEN** 系统将其写入当前资产版本并允许后续编辑和导出

#### Scenario: Reject candidate
- **WHEN** 用户拒绝候选
- **THEN** 系统保留候选历史但不将其作为当前可导出资产

### Requirement: Roll back decisions
系统 SHALL 允许用户从操作日志回退到此前资产状态，并刷新当前预览与可导出资产列表。

#### Scenario: Roll back
- **WHEN** 用户确认回退到历史操作点
- **THEN** 系统恢复该点状态并保留回退记录
