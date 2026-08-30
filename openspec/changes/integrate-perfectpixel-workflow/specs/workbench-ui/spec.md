## Purpose

提供以预览为中心、连续且可恢复的角色动画制作工作区，减少页面跳转和重复选择，同时保持身份、验收和导出的明确边界。

## ADDED Requirements

### Requirement: Preview-first workbench
系统 SHALL 在同一工作区提供身份上下文、动作/方向列表、当前动画预览、任务状态和主要操作入口；用户 SHALL 能从当前对象直接进入生成、验收、编辑或导出。

#### Scenario: Continue current asset
- **WHEN** 用户选择一个动作方向
- **THEN** 系统同步更新预览、帧操作、验收和编辑入口，而无需再次选择同一对象

### Requirement: Preserve drafts
系统 SHALL 在标签切换、任务运行和应用重启后的会话恢复中保留未提交的身份和动作草稿，并明确标识未保存状态。

#### Scenario: Switch with unsaved draft
- **WHEN** 用户修改描述后切换视图但尚未保存
- **THEN** 系统保留草稿并提示其未保存，不得静默丢失
