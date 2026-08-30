## Purpose

将基础角色生成与动画生成纳入统一共享服务，并在所有外部 Provider 调用前提供可审查、可取消和可恢复的执行边界。

## ADDED Requirements

### Requirement: Confirm before external generation
系统 SHALL 在基础角色或动画生成发起外部调用前展示 Provider、模型、素材、方向、帧数、预计调用量、预算和提示词摘要，并仅在用户确认后执行。

#### Scenario: Review and confirm
- **WHEN** 用户查看生成计划并确认
- **THEN** 系统按计划创建任务并开始调用；取消时不得发起外部调用

### Requirement: Apply deterministic processing
系统 SHALL 对返回结果执行既有去背景、切帧、对齐、像素化、质量检查和有限重试流程，并保留最佳候选而不是返回空结果。

#### Scenario: Imperfect provider result
- **WHEN** Provider 返回帧数或一致性不符合要求的结果
- **THEN** 系统执行受限纠错重试并记录评分、警告和候选历史
