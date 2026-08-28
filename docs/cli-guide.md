# oframe CLI 指南

`oframe` 是与 Wails GUI 共享同一 Go 核心库的可脚本化入口（单核多端）：
GUI 与 CLI 调用同一 `core/service`，行为不会漂移。适合 CI、批量处理与高级用户。

## 0. 快速开始

```powershell
# 查看帮助与全部命令
go run ./cmd/oframe --help

# 1) 初始化工作区并创建身份包
go run ./cmd/oframe workspace init D:\my-workspace
go run ./cmd/oframe identity create --workspace D:\my-workspace --name Hero --json

# 2) 配置 provider 密钥（本地保存，离线校验）
go run ./cmd/oframe provider config set --key sk-... doubao
go run ./cmd/oframe provider validate doubao

# 2b) 设置逻辑画布（生成的前置条件）
go run ./cmd/oframe identity canvas --width 32 --height 32 D:\my-workspace\Hero

# 3) 生成确认预览（不发起任何调用）
go run ./cmd/oframe generation plan --directions 4 D:\my-workspace\Hero --json

# 4) 确认后执行（--yes 是显式确认；没有 --yes 一律不调用）
go run ./cmd/oframe generation run --directions 4 --yes D:\my-workspace\Hero --json
```

## 1. 全局约定

- **参数顺序**：`flag` 解析在第一个位置参数处停止，因此**标志必须写在位置参数之前**
  （`generation plan --directions 4 <pkg>`，不是 `<pkg> --directions 4`）。
- `--json`：stdout 输出机器可读 JSON（`{"ok":true,...}` / `{"ok":false,"error":...}`）。
- `--log-level <debug|info|warn|error>`：日志写 stderr，绝不污染 stdout。
- 每个需要本地配置的命令支持 `--settings-dir <dir>`（默认用户配置目录，
  与 GUI 共享密钥/模型/统计）。

## 2. 命令参考

### `workspace`

```
workspace init <path>         创建/初始化工作区
workspace list [path]         列出工作区内的身份包（默认用户主目录工作区）
```

### `identity`

```
identity create --workspace <path> --name <name>   创建身份包
identity open <path>                               打开并校验身份包
identity canvas --width <w> --height <h> <path>    设置逻辑画布（生成前置条件）
```

### `provider`（生成 provider 配置）

```
provider list                        列出 provider 及其配置状态
provider config get <id>             查看本地配置（密钥脱敏显示 ****）
provider config set <id> --key ...   设置密钥/模型/endpoint（本地保存）
provider validate <id>               离线配置校验
provider stats                       本地调用统计（次数与费用估算）
```

内置 provider id：`doubao`（默认主力）、`openai`（gpt-image-2）、`agnes`。全新安装**不预置**
任何 provider（人工验收更新）——对未配置的 id 直接执行 `provider config set <id>` 会校验、
持久化并注册该 provider（内置 id 使用其默认端点/模型）。

自定义 provider（在 GUI 设置面板「快速添加」创建，生成新的 id）支持显式协议类型：
`compatible` / `api`（OpenAI 兼容）、`dashscope`（百炼）、`gemini`（banana/Gemini）、
`minimax`、`volcengine`（火山方舟/豆包）、`cli`（本地命令行工具）。每个 provider 的
图像 / 视频 / 文本模型独立保存在目录字段（`imageModels` / `videoModels` / `textModels`，
兼容旧的单模型字段）；视频目录为预留配置，视频执行管线接入前不可调用。添加后同样可用
`provider config set <id>` 配置密钥/模型；`provider list` 会显示每个 provider 的协议类型
与能力声明。

### `generation`（生成确认 + 批量生成）

```
generation plan [flags] <pkg>               生成确认预览（零调用）
generation run [flags] --yes <pkg>          确认后执行（无 --yes 零调用）

flags:
  --directions <1|4|8>   方向策略（默认 1；8 方向 = 5 生成 + 3 镜像）
  --motion <id>          目标动作 id（批量生成落到具体动作，资产才可导出）
  --style <id>           风格预设（默认 pixel_classic）
  --action <id>          动作预设（默认 walk）
  --provider <id>        显式 provider（默认当前激活 provider）
  --model <name>         显式图像模型
  --frame-count <n>      filmstrip 帧数（默认 4 或动作已有序列长度）
  --max-attempts <n>     每方向最多总尝试（默认 3，生成确认预算）
```

- 生成走**共享持久化任务队列**：`generation run --yes` 后任务行写入
  `queue.db`（settings 目录），状态/进度/结果与 GUI 任务抽屉一致；
  相同请求命中**幂等缓存**时直接复用结果、不重复计费。
- 镜像方向不消耗调用；`--directions 4` 预计 3 次调用（right/up/down），
  `--directions 8` 预计 5 次调用。

### `validate`（身份包与导出包完整性校验）

```
validate <path>   校验身份包（identityPackage）或导出包（exportPackage）
```

身份包按 `identity.name` 元数据判定；导出包按 `manifest.json` + 精灵表 + 逐帧
文件完整性校验。

### `export`（导出包）

```
export create --output <dir> --target <generic|godot|unity> [--version <id>] <pkg>
                   [--settings-dir <dir>]     生成并校验导出包
export validate <dir>                         校验已有导出包
export history [--settings-dir <dir>] <pkg>   查看导出历史
```

- 仅**验收通过**的资产可导出；`--version` 缺省用当前身份版本。
- 产出 `spritesheet.png` + `manifest.json` + 逐帧 PNG + `<target>.json`；
  生成后自动校验，失败即报错。
- 历史记录写入身份包 `exports/history.jsonl`。

## 3. 批量/CI 用法示例

```powershell
# 一批角色：创建 → 画布 → 生成 → 校验 → 导出
$ws = "D:\my-workspace"
foreach ($name in "Hero","Goblin","Knight") {
  go run ./cmd/oframe identity create --workspace $ws --name $name
  go run ./cmd/oframe identity canvas --width 32 --height 32 "$ws\$name"
  go run ./cmd/oframe generation run --directions 4 --yes "$ws\$name"
  go run ./cmd/oframe validate "$ws\$name"
}
```

> 注意：CLI 生成产出**候选**，导出需要**已接受资产**。CLI 目前未提供「接受候选」
> 子命令，接受操作在 GUI「验收」标签完成（或由共享 `core/service` 的程序化流程
> 完成）。`--motion` 需要先在 GUI 或服务层创建动作。

## 4. 与 GUI 的一致性（单核多端）

CLI 与 GUI 调用同一 `core/service`：同样的请求参数产生同样的候选、同样的方向
派生与导出清单。`cmd/oframe/cli_phase8_test.go` 中的双端一致性回归在同一批
用例上同时运行 CLI 与服务层并断言结果一致（候选方向/帧数、导出 manifest 结构）。

## 5. 成本与安全

- 生成前先 `generation plan` 查看预计调用量与预算，`run` 必须带 `--yes` 才调用。
- 密钥只保存在本地配置目录，永不写入工作区或导出包。
- 幂等缓存避免相同任务重复计费；每方向最多尝试次数是硬上限。
