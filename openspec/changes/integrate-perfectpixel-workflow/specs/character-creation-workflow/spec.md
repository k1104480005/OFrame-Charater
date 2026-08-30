## Purpose

为用户提供从文字描述和参考素材创建基础角色、或导入已有角色图的连续入口，并将选定角色可靠地作为后续动画生成的身份基准；两种来源在身份包内必须二选一且不可切换。

## ADDED Requirements

### Requirement: Create base-character candidates
系统 SHALL 允许用户提交身份描述、风格、画布规格和可选参考素材，生成一个或多个基础角色候选，并显示每个候选的状态、Provider、模型和错误信息。

#### Scenario: Generate candidate
- **WHEN** 用户提交有效描述且完成生成确认
- **THEN** 系统创建任务并在完成后显示可预览的基础角色候选

#### Scenario: Missing provider
- **WHEN** 用户未配置可用的图像 Provider
- **THEN** 系统阻止外部调用并显示可执行的配置提示

### Requirement: Adopt a candidate as identity
系统 SHALL 允许用户预览候选并明确采用其中一个；采用后该角色成为身份包的当前基础角色，未采用候选不得作为动画生成输入。

#### Scenario: Adopt candidate
- **WHEN** 用户点击候选的采用操作
- **THEN** 系统保存基础角色及其生成元数据，并使后续动作生成使用该身份

#### Scenario: Preserve existing sprite
- **WHEN** 用户导入既有精灵而非生成角色
- **THEN** 系统允许将其作为身份基准继续流程，且不要求重新生成基础角色
