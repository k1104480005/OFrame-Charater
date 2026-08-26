# OFrame Character Workbench — 产品描述

> 面向独立游戏与像素游戏开发者的 2D 角色动画资产工作台。
> 从文字/参考图/既有精灵，到可直接导入 Godot / Unity / 自研引擎的动画资产。

---

## 一句话简介（GitHub About / 仓库描述）

**OFrame Character**：本地优先的 Windows 桌面工作台，把「文字或参考图」变成
「可直接进游戏引擎的 2D 像素角色动画」——确定性生成、方向一致、质量可验收、一键导出。

*(EN: Local-first Windows workbench that turns text/reference art into engine-ready 2D pixel character animations — deterministic, direction-consistent, quality-gated, one-click export.)*

---

## 产品介绍

### 它解决什么问题

做 2D 像素角色动画，独立开发者通常要面对两难：

- 逐帧手绘：费时、难以保持帧间和方向间一致；
- 用通用绘图/AI 工具生成：帧与帧、方向与方向互相"走样"，拿到游戏引擎里根本无法直接用。

OFrame Character 把「生成 → 校正 → 验收 → 导出」串成一条**本地优先、确定性**的流水线：
你描述一个角色，它产出**方向一致、锚点对齐、可直接进引擎**的动画资产。

### 核心亮点

- **PerfectPixel filmstrip 确定性管线**：每个方向一次提示词产出整条横向"胶片条"，
  保证帧间风格/体型/节奏天然一致；再以**整数像素级**确定性切片与校正
  （透明抠图、锚点对齐、共享调色板量化），预览所见即引擎所得，无二次插值模糊。
- **方向策略与自动镜像**：新动作默认单方向；4/8 方向时仅生成基本方向、
  其余方向水平镜像派生（8 方向 = 5 生成 + 3 镜像），独立帧序列 + 锚点镜像换算
  （X' = 宽-1-X），成本低、方向一致；镜像不自然的方向可在验收时手动替换。
- **生成确认与成本控制**：任何可能产生外部调用的操作，先展示预计调用量、
  每方向最多尝试次数与预算，确认后才执行、取消则零调用；失败自动退避重试
  （硬上限）、相同任务幂等缓存不重复计费、本地调用统计。
- **质量评分与验收关口**：结构/规则指标量化评分（切片完整性、锚点偏差、帧间抖动、
  调色板一致性等），评分达标且预览确认方通过；候选历史保留每一次结果。
- **可恢复任务队列**：任务本地持久化，崩溃/关机/断网后一键续跑。
- **版本化修订**：不可变身份版本、候选接受语义、追加式操作日志支持回退到任意历史点。
- **多引擎导出**：通用序列帧 / Godot / Unity，导出包自动校验完整性并记录历史。
- **轻量编辑**：帧级/序列级/锚点级/批量校正，以可回放指令非破坏式记录
  （核心能力已实现；编辑器界面为预览骨架，见 Roadmap）。
- **单核多端**：Wails GUI 与 `oframe` CLI 共享同一 Go 核心库，行为不会漂移，
  支持 CI 与批量处理。
- **本地优先**：密钥仅存本机、永不进导出包；暖白/深墨双主题像素化界面。

### 快速开始

**方式一：GUI（Windows 10/11 x64）**

1. 安装便携版或 NSIS 安装包，启动后在启动页「创建身份包」。
2. 制作 > 身份：填写文字描述，可选添加参考图。
3. 制作 > 动作：新建动作、选择方向策略，生成确认后执行。
4. 验收：预览回放、确认通过候选。
5. 导出：选择引擎目标，生成并校验导出包。

> 无 provider 密钥？`go run ./cmd/examplegen` 可离线生成一个完整示例身份包
> （`examples/hero-walk`），直接在 GUI 打开体验全流程。

**方式二：CLI（共享同一核心）**

```bash
oframe identity create --workspace <ws> --name Hero
oframe identity canvas --width 32 --height 32 <pkg>
oframe generation plan --directions 4 <pkg>        # 零调用确认预览
oframe generation run  --directions 4 --yes <pkg>  # 确认后执行
oframe validate <pkg>
oframe export create --output <dir> --target godot <pkg>
```

### 技术栈

| 层 | 技术 |
|---|---|
| 桌面壳 | Go + Wails v2 |
| 前端 | React 19 + TypeScript + Vite |
| 预览/编辑画布 | PixiJS 8（最近邻采样、纹理图集、降级缩放） |
| 持久化 | SQLite（modernc.org/sqlite，任务队列/幂等缓存）+ 身份包目录 manifest |
| CLI | oframe（Go 同源，与 GUI 单核多端） |
| 生成 provider | Doubao（默认）/ OpenAI gpt-image-2 / Agnes，可插拔 |

### 当前状态与 Roadmap

- **状态**：公开 Beta 就绪（P0 单方向 MVP、P1 方向集与质量闭环、P2 导出/CLI/打包/文档 已完成）。
- **已支持**：文字/参考图入口、4/8 方向自动镜像、filmstrip 确定性管线、质量验收、
  候选历史、版本化回退、可恢复任务、多引擎导出、CLI、Windows 打包（NSIS + 便携版）、示例资产。
- **Roadmap**：
  - 轻量编辑 GUI 接线（核心已实现）
  - 更多引擎导出格式与模板/风格库
  - macOS / Linux
  - 批量任务与批处理增强

### 仓库导航

- `docs/user-guide.md` — 用户指南（镜像语义、生成确认、成本说明）
- `docs/cli-guide.md` — CLI 参考
- `docs/example-walkthrough.md` — 示例走查（文字 → 4 方向行走资产）
- `examples/hero-walk` — 可直接打开的示例身份包
- `openspec/` — 规格与设计文档（OpenSpec）

### License

（待定 —— 建议 MIT，发布前补充 LICENSE 文件。）

---

## English Summary（Release Note 用）

**OFrame Character** is a local-first Windows workbench for indie and pixel-game
developers that turns a text description or reference art into engine-ready 2D
pixel character animations. It pipelines generation → deterministic correction →
quality acceptance → export: a single prompt produces a horizontal filmstrip per
direction, which is sliced and corrected at integer-pixel precision so preview
exactly matches what the engine consumes. Direction sets default to one direction
and auto-mirror up to 4/8 directions (5 generated + 3 mirrored at 8), keeping
costs low and directions consistent. Every external call is gated by a
confirmation with expected cost and hard retry caps; idempotent caching avoids
duplicate billing. Accepted assets export to generic sprite sheets, Godot, or
Unity with automatic validation. A Go core powers both the Wails GUI and the
`oframe` CLI (single core, two entry points), backed by SQLite persistence and a
recoverable task queue. Windows 10/11 x64 first release.
