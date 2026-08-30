## Purpose

将已验收的角色动画以引擎可用格式稳定导出，同时让导出对象和当前工作区上下文保持一致。

## ADDED Requirements

### Requirement: Export accepted assets only
系统 SHALL 仅允许已接受且属于当前身份版本的资产进入导出包，并在导出后校验帧、锚点和清单完整性。

#### Scenario: Export valid package
- **WHEN** 用户选择引擎目标并导出存在已接受资产的身份包
- **THEN** 系统生成目标元数据、逐帧 PNG、精灵表和清单，并报告校验结果

#### Scenario: No accepted assets
- **WHEN** 当前没有已接受资产
- **THEN** 系统禁用或拒绝导出，并说明必须先完成验收
