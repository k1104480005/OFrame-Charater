# OFrameCharacter 开发交接文档

> 生成时间：2026-08-26（UI 全面翻新已完成并推送 GitHub；唯一剩余 13.4 发布人工验收）
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

## 2. 当前进度（交接时准确状态）

- **任务进度：67/68 勾选完成**，唯一剩余 **13.4 发布公开 Beta**（人工验收 + 发布渠道，非阻塞）。
- 全部开发阶段已完成：P0 骨架 → 阶段1 Go核心/工作区 → 阶段2 Wails+React/PixiJS → 阶段3 身份候选+Provider → 阶段4 确定性图像管线 → 阶段5 动作与方向集 → 阶段6 任务队列+验收+版本化 → 阶段7 轻量编辑 → 阶段8 导出与 CLI + 13.1 示例 / 13.2 文档 / 13.3 性能 / 1.3 打包。
- **最近一轮（2026-08-26）UI 全面翻新，已提交并推送 GitHub（commit `c4a4830`）**：
  - **动森风格皮肤**（参考 `D:\GitProject\animal-island-ui` 组件库）：暖米白/棕/薄荷绿双主题、药丸圆角、3D 按下阴影、波点纹理背景、Nunito 字体**本地打包**（离线可用，`frontend/src/styles/fonts.css`）、自定义 Switch、弹窗 pop 动效；洋红仍仅限抠图检查视图。
  - **启动页 v2**：右上角「＋创建 / 工作区」弹窗；身份包**卡片网格**（缩略图占位 + 创建/修改时间）；左侧**分类栏**（全部 / 未分类 / 自定义，可新建分类；**拖拽卡片到分类改分类**，**右键删除分类**自动归未分类）；自定义动森分类下拉；卡片点名字改名、右上角红 ✕ 删除（动森 ConfirmModal 确认 → 移到工作区 `.trash`）。
  - **工作区**：浏览文件夹（原生对话框）、切换记忆、迁移（切换时询问是否移动包）。
  - **身份包元数据**：新增 `category`（≤32 字符，空=未分类）与 `createdAt/updatedAt`；`PackageCreate(name, category)` 创建即分类；改名 = manifest 级显示名（**目录路径不变**）。
  - **统一确认弹窗** `frontend/src/components/ConfirmModal.tsx`（替代原生 window.confirm）。
  - 修正「身分包→身份包」错别字 9 处。
- **git 状态**：已推送 GitHub（默认分支 master，最新 `c4a4830`），工作区干净。

### 最近一次验证结果（全绿）

```text
gofmt -l .           无输出
go vet ./...         无输出
go test -count=1 ./...   17 包全部通过
npm run typecheck    通过
npm run build        通过
wails build -s -m -webview2 browser  通过（便携版已产出）
```

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
- **网络受限（重要）**：Go module proxy、npm registry、Google Fonts 均不可达 → 依赖只能走本地缓存（`GOPROXY=off GOSUMDB=off`），**不要新增 npm/go 依赖**（Nunito 字体已本地打包，无需联网）。SourceForge 可达（装 NSIS 时验证过）。
- **git push 需代理**：仓库级已配置 `http.proxy socks5h://127.0.0.1:7890`（用户 VPN 为 Clash 系代理模式；VPN 开着才能推）。推送方式：`git push -u https://<user>:<TOKEN>@github.com/k1104480005/OFrame-Charater.git master`（用户保留经典令牌，repo 权限），推完 `git remote set-url origin https://github.com/...` 还原为无令牌地址 + 修正 `branch.master.remote=origin`。
- **不要碰的确认过的决策**：方向口径（5+3 镜像）、默认单方向 down、验收阈值 0.7、密钥本地明文存储、filmstrip 一次生成、每方向最多 3 次尝试、不做骨骼/多轨/完整绘画/云同步/MCP、不做多语言（i18n 已评估放弃：前端 16/18 文件硬编码中文 + Go 报错 13 条中文，npm 不可达装不了库）。
- **测试风格**：确定性合成图像测试，禁止依赖真实网络/付费 API；用 fake transport；`go test -count=1 ./...` 为准。
- **Windows 保留文件名**：测试文件不要用 `aux.*` 等保留名。
- **打包环境坑（仍在）**：`wails build` 内部 vite 清理 `frontend/dist` 会撞本机 safe-delete 垫片。可靠做法：**前台**执行 `rm -rf frontend/dist && npm run build`（或 vite 输出到临时目录再 `cp -r`），然后 `wails build -s -m -webview2 browser`（-s 跳过前端构建，避免 WebView2 下载）。
- **改动规范**：先读 spec → 实现 → 更新 `tasks.md` 勾选 → 运行 gofmt/go test/go vet/前端 typecheck/build/wails build。不要虚标完成。

## 6. 关键文件索引（按模块）

| 模块 | 文件 |
|---|---|
| 身份包 | `core/identity/`（package/manifest/definition/canvas/anchors；`SetName`/`SetCategory` 在 definition.go） |
| 工作区 | `core/workspace/`（workspace/config.go 新增：切换记忆/迁移；`TrashPackage` 移到 `.trash`） |
| 动作/方向集 | `core/motion/`（motion/mirror/store） |
| 图像管线 | `core/pipeline/`（filmstrip/slice/keying/align/palette/grid/quality/candidate/process） |
| Provider | `core/provider/`（provider/registry/config/retry/doubao/openai/agnes/http/stats） |
| 任务队列 | `core/task/`（task/store）+ `core/store/`（migrations） |
| 设置 | `core/settings/` |
| 编辑 | `core/edit/` + `core/service/edit.go` + `edit_bindings.go` |
| 导出 | `core/assetexport/` + `core/service/export.go` + 根 `export.go` |
| 服务层 | `core/service/`（service/generation/acceptance/queue/motion/export/edit） |
| 版本化 | `core/version/`（version/assets/acceptance/log/rollback） |
| CLI | `cmd/oframe/`（main/export/provider/generation/service） |
| Wails 壳 | 根目录 `main.go`/`app.go`/`bindings.go`/`tasks.go`/`generation.go`/`motion.go`/`export.go` |
| 前端 | `frontend/src/`（pages: LaunchPage v2/MainScreen/ExportTab/AcceptanceTab/make/*；components: ConfirmModal/PixelCanvas/pixelAtlas/ThemeToggle/SettingsPanel/TaskDrawer；styles: tokens/global/fonts.css；api/client.ts） |

## 7. 交接完成定义

新 AI 读完本文档 + 必读文件后，应能回答：当前任务进度、下一步任务、方向口径、验收规则、导出接线缺失点、验证命令、git 推送方式。从「人工验收（13.4，docs/manual-test-checklist.md）」与后续增强开始即可继续，无需重新规划。
