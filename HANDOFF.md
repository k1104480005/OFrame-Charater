# OFrameCharacter 开发交接文档

> 生成时间：2026-08（阶段 7 完成，阶段 8 进行中）
> 用途：让另一位 AI 在不依赖原会话的情况下继续开发本项目。

## 0. 项目一句话

本地优先的 Windows 桌面 AI 角色动画资产工作台：从文字/参考图/既有精灵创建「角色身份包」，生成并确定性校正像素动画（动作 × 方向集 × 帧序列 × 锚点），经质量验收后导出精灵表供游戏引擎直接使用。

## 1. 必读文件（按顺序）

| 文件 | 作用 |
|---|---|
| `PLAN.md` | 总体计划：产品边界、架构、阶段划分、风险（已确认） |
| `CONTEXT.md` | 领域词汇（角色身份包、方向集、生成候选、候选接受、身份版本等），**写代码/文档必须用这套术语** |
| `openspec/changes/build-oframe-character-workbench/proposal.md` | 变更目标与范围 |
| `openspec/changes/build-oframe-character-workbench/design.md` | 架构决策（Go 单核多端、Wails、filmstrip、镜像、任务恢复等） |
| `openspec/changes/build-oframe-character-workbench/specs/` | 11 个能力规格（identity-package、motion、generation、filmstrip-pipeline、quality、tasks、editing、versioning、export、workbench-ui、cli），**需求真相以 spec 为准** |
| `openspec/changes/build-oframe-character-workbench/tasks.md` | 任务清单与勾选进度，**每完成一个任务才勾选** |

OpenSpec 工作流：用 `openspec status --change build-oframe-character-workbench --json`、`openspec instructions apply --change ... --json` 读取上下文；不要跳过 spec 直接写实现。

## 2. 当前进度（切换时的准确状态）

- **任务进度：67/68 勾选完成**。
- 已完成阶段：P0 骨架 → 阶段1 Go核心/工作区 → 阶段2 Wails+React/PixiJS → 阶段3 身份候选+Provider → 阶段4 确定性图像管线 → 阶段5 动作与方向集 → 阶段6 任务队列+验收+版本化 → 阶段7 轻量编辑 → 阶段8 导出与 CLI（Go 侧 + 前端接线 + CLI 命令与测试均完成）+ 13.2 用户/CLI 文档 + 1.3 Windows 打包冒烟 + 13.1 示例资产与教程 + 13.3 性能优化。
- 阶段 8（导出与 CLI）**全部接通**：ExportTab 真实导出界面、client.ts 导出绑定、设置面板外观主题、CLI generation/validate/export 命令 + 双端一致性回归。
- **1.3 完成**：便携版 `build/bin/OFrameCharacterWorkbench.exe`（启动冒烟通过）+ NSIS 安装包 `build/bin/oframe-character-workbench-amd64-installer.exe` 均已产出。NSIS 3.12 免安装版已装到 `%LOCALAPPDATA%\Programs\nsis-3.12` 并加入用户 PATH。
- **全量回归（65/68 时执行）全绿**：gofmt / go vet / go test ./...（16 包）/ 前端 typecheck+build / 绑定重生成 / 打包产物 / CLI 实机冒烟。期间修复：CLI 帮助文本与文档的参数顺序（标志必须在位置参数前）；新增 `identity canvas --width --height <pkg>` 子命令（CLI 创建的身份包无逻辑画布、生成前置必需）。
- **13.1 完成**：`cmd/examplegen`（合成 filmstrip、零付费 API）产出 `examples/hero-walk` 身份包 + `examples/hero-exports/{generic,godot}` 导出包；教程 `docs/example-walkthrough.md`；测试 `cmd/examplegen/main_test.go`。
- **13.3 完成**：`PixelCanvas` 纹理图集（单纹理 + 子区域视图）、静态/动态分层（tick 不再重建舞台/重复解码）、降级缩放（像素预算限幅 + data-degraded）；纯函数 `frontend/src/components/pixelAtlas.ts`，校验脚本 `frontend/scripts/pixelAtlasCheck.ts`（`npm run check:pixel-atlas`，12 断言）。打包产物已含新前端（17:46 重建 + 启动冒烟通过）。
- **重要变更**：`core/export` 包已重命名为 `core/assetexport`（原包名 `export` 在 Wails 生成的 TS 绑定中产生 `export namespace export`，与 TS 保留字冲突导致前端无法编译；`wails generate module` 已重新生成绑定）。
- **重要变更 2**：CLI `generation` 新增 `--motion` 标志（批量生成落到具体动作，否则候选无 motionId 无法导出）；CLI `export create/history` 新增 `--settings-dir`；`validate` 命令按 `identity.name` 区分身份包与导出包（两者都叫 manifest.json）。
- **打包环境注意**：`wails build` 内部 vite 清理 `frontend/dist` 会撞本机 safe-delete 垫片（trash 失败）；解法是先 `rm -rf frontend/dist` + `npm run build`，再 `wails build -s -m -webview2 browser`（跳过前端构建、避免 WebView2 下载）。

### 最近一次验证结果（全绿）

```text
gofmt -l .          无输出
go test -count=1 ./...   全部通过（含 core/edit、core/export）
go vet ./...        无输出
wails generate module    已执行，绑定已重新生成
```

> 注：项目当前未完成 git 提交，工作区有大量未提交文件，切换后先看 `git status`。

## 3. 架构与关键决策（接手者必须知道）

- **技术栈**：Go 核心库（`core/`）+ Wails v2 桌面壳（根目录 `main.go`/`app.go` 等）+ React 19/TypeScript/Vite/PixiJS 前端（`frontend/`）+ 独立 CLI（`cmd/oframe`）。GUI 与 CLI 调用同一 `core/service`（单核多端，不允许双端逻辑漂移）。
- **持久化**：SQLite（`modernc.org/sqlite`，schema migration v1-v3）存任务队列/结果缓存；身份包为目录 + `manifest.json`（格式 v1）+ `motions.json` + `versions/<id>/assets/` + `log/`（追加式操作日志 JSONL）+ 候选历史；provider 密钥/统计在用户配置目录 `%AppData%\OFrameCharacter\settings.json`，**密钥不进工作区、不进导出包**。
- **方向规则（已确认，勿改）**：默认单方向 `down`（正面/south）；4 方向 = 3 生成（right/up/down）+ 1 镜像（left←right）；8 方向 = **5 生成（right/up/down/up-right/down-left）+ 3 镜像（left←right、up-left←up-right、down-right←down-left）**，up/down 自对称；关闭镜像时全方向独立生成。镜像锚点换算 X'=width-1-X、Y'=Y。
- **生成契约**：每个真实生成方向一次提示词产出**横向 filmstrip**，后端确定性切片（alpha projection + DP optimal cut）、YCbCr 抠图、质心/基线对齐、共享调色板量化、质量评分；单帧生成仅用于验收修补。每方向最多 3 次总尝试（默认），镜像不消耗调用；生成必须先 `PrepareGeneration` → 用户确认 → `ConfirmGeneration`，**确认前零外呼**。
- **质量验收**：硬门槛（帧数/透明/无裁切/锚点稳定等）+ 综合评分，默认阈值 0.7；通过 = 评分达标 **且** 用户在预览确认；拒绝候选只留在候选历史；接受后形成不可变动画资产，人工编辑保存为新修订（追加式操作日志支持回退）。
- **Provider**：Doubao（默认）、OpenAI gpt-image-2、Agnes（显式选择）；每批次锁定 provider/model，不静默跨 provider 回退；预算在生成确认时锁定。
- **导出**：`core/assetexport` 已实现 generic/godot/unity 目标：`spritesheet.png` + `manifest.json` + 逐帧 PNG + `generic.json|godot.json|unity.json` 目标元数据；`Build` 后自动 `Validate` 完整性，失败则任务失败；`exports/history.jsonl` 记录历史。

## 4. 下一步工作（按优先级）

### 4.1 阶段 8 前端接线（已完成 ✅）

任务 10.5（导出标签）、10.6（设置入口）、11.1–11.4（导出）已全部接线并验证；
**编辑页已接线**（`core/service/edit.go` + `edit_bindings.go` + `EditPage.tsx` 真实 UI：
删除帧/顺序/时长/批量去背景/锚点偏移，指令可回放、写回当前版本资产）。

### 4.2 剩余未勾选任务（68 项中余 1 项）

- **13.4** 发布公开 Beta：NSIS 安装包 + 便携版构建已通过（1.3），示例资产（examples/hero-walk）与教程、**人工测试清单（docs/manual-test-checklist.md，A 无密钥全流程 / B 真实密钥生成 / C 边界恢复）**已就绪；剩「外部用户可完成一个完整角色资产」**人工验收**与发布渠道决策（用户当前进行中）。

## 5. 工程注意事项

- **Windows 环境**：本机 Windows 10 build 19041。Go 1.26.4 + `powershell.exe` 5.1 正常；若遇到 `Your Windows doesn't fully support CET`，是 Harness 的 `pwsh.exe` 启动器问题，不是项目问题，不要改 `go.mod` 掩盖。
- **网络受限**：Go module proxy 不可达；依赖需走本地 module cache（`GOPROXY=off GOSUMDB=off`）或 goproxy.cn。不要新增依赖；`modernc.org/sqlite` 已可用。
- **不要碰的确认过的决策**：方向口径（5+3 镜像）、默认单方向 down、验收阈值 0.7、密钥本地明文存储、filmstrip 一次生成、每方向最多 3 次尝试、不做骨骼/多轨/完整绘画/云同步/MCP。
- **测试风格**：确定性合成图像测试，禁止依赖真实网络/付费 API；用 fake transport；`go test -count=1 ./...` 为准。
- **Windows 保留文件名**：测试文件不要用 `aux.*` 等保留名。
- **改动规范**：先读 spec → 实现 → 更新 `tasks.md` 勾选 → 运行 gofmt/go test/go vet/前端 typecheck/build/wails build。不要虚标完成。

## 6. 关键文件索引（按模块）

| 模块 | 文件 |
|---|---|
| 身份包 | `core/identity/`（package/manifest/definition/canvas/anchors） |
| 动作/方向集 | `core/motion/`（motion/mirror/store） |
| 图像管线 | `core/pipeline/`（filmstrip/slice/keying/align/palette/grid/quality/candidate/process） |
| Provider | `core/provider/`（provider/registry/config/retry/doubao/openai/agnes/http/stats） |
| 任务队列 | `core/task/`（task/store）+ `core/store/`（migrations） |
| 设置 | `core/settings/` |
| 编辑 | `core/edit/`（本次新增 edit.go/edit_test.go） |
| 导出 | `core/assetexport/`（export.go/export_test.go）+ `core/service/export.go` + 根 `export.go` |
| 服务层 | `core/service/`（service/generation/acceptance/queue/motion/export） |
| 版本化 | `core/version/`（version/assets/acceptance/log/rollback） |
| CLI | `cmd/oframe/`（main/export/provider/generation/service） |
| Wails 壳 | 根目录 `main.go`/`app.go`/`bindings.go`/`tasks.go`/`generation.go`/`motion.go`/`export.go` |
| 前端 | `frontend/src/`（pages: LaunchPage/MainScreen/ExportTab/AcceptanceTab/make/*；api/client.ts；wailsjs 已重新生成） |

## 7. 交接完成定义

新 AI 读完本文档 + 必读文件后，应能回答：当前任务进度、下一步任务、方向口径、验收规则、导出接线缺失点、验证命令。从「4.1 前端接线」开始即可继续，无需重新规划。
