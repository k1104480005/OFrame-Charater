# OFrame Character Workbench（角色动画资产工作台）

> 面向独立游戏与像素游戏开发者的 2D 角色动画资产制作工作台。
> 从文字 / 参考图 / 既有精灵，到可直接导入 **Godot / Unity / 自研引擎** 的动画资产。
>
> *Local-first Windows workbench that turns text/reference art into engine-ready
> 2D pixel character animations — deterministic, direction-consistent,
> quality-gated, one-click export.*

本地优先的 Windows 10/11 x64 桌面应用：以可版本化的「角色身份包」为根对象，
把「生成 → 确定性校正 → 质量验收 → 导出」串成一条可中断恢复的流水线。
Wails GUI 与 `oframe` CLI 共享同一 Go 核心库（单核多端，行为不漂移）。

- 领域语言与术语：`CONTEXT.md`
- 总体计划：`PLAN.md`
- 规格/设计/任务：`openspec/`

---

## 它解决什么问题

做 2D 像素角色动画，独立开发者常陷入两难：逐帧手绘费时且难以保持一致；
用通用绘图/AI 工具生成，帧与帧、方向与方向互相「走样」，拿进游戏引擎无法直接用。
OFrame Character 用**确定性**的 filmstrip 管线解决「一致性」，用**生成确认 + 硬上限**
解决「成本」，用**质量验收关口**保证「可交付」。

## 核心亮点

- **PerfectPixel filmstrip 确定性管线**：每个方向一次提示词产出整条横向「胶片条」，
  帧间风格/体型/节奏天然一致；再以整数像素级确定性切片与校正（透明抠图、锚点对齐、
  共享调色板量化），预览所见即引擎所得，无二次插值模糊。
- **方向策略与自动镜像**：默认单方向；4/8 方向时仅生成基本方向、其余水平镜像派生
  （8 方向 = 5 生成 + 3 镜像），独立帧序列 + 锚点镜像换算（X' = 宽-1-X）；
  镜像不自然的方向可在验收时手动替换。
- **生成确认与成本控制**：任何外部调用先展示预计调用量、每方向最多尝试次数与预算，
  确认后才执行、取消则零调用；失败自动退避重试（硬上限）、相同任务幂等缓存不重复计费、
  本地调用统计。
- **质量评分与验收关口**：结构/规则指标量化评分（切片完整性、锚点偏差、帧间抖动、
  调色板一致性等），评分达标且预览确认方通过；候选历史保留每次结果。
- **可恢复任务队列**：任务本地持久化（SQLite），崩溃/关机/断网后一键续跑。
- **版本化修订**：不可变身份版本、候选接受语义、追加式操作日志支持回退到任意历史点。
- **多引擎导出**：通用序列帧 / Godot / Unity，导出包自动校验完整性并记录历史。
- **启动页分类管理**：身份包按分类组织（全部 / 未分类 / 自定义），卡片视图 + 缩略图占位，
  拖拽卡片改分类、右键删分类（包自动归未分类）、点名字改名、删除进工作区回收站。
- **单核多端**：GUI 与 `oframe` CLI 共享同一核心，支持 CI 与批量处理。
- **本地优先**：密钥仅存本机、永不进导出包；暖白/深墨双主题动森风格界面（Nunito 字体本地打包，离线可用）。

## 快速开始

**方式一：GUI（Windows 10/11 x64）**

1. 安装便携版或 NSIS 安装包，启动后点右上角「＋ 创建」新建身份包（可选分类）。
2. 制作 > 身份：填写文字描述，可选添加参考图（1 主 + 最多 2 辅助）。
3. 制作 > 动作：新建动作、选择方向策略（单方向 / 4 / 8），生成确认后执行。
4. 验收：PixelPerfect 预览回放、确认通过候选。
5. 导出：选择引擎目标，生成并校验导出包。

> 没有 provider 密钥？运行 `go run ./cmd/examplegen` 可离线生成完整示例身份包
> （`examples/hero-walk`），直接在 GUI 打开体验全流程。

**方式二：CLI（与 GUI 共享同一核心）**

```powershell
oframe identity create --workspace D:\my-workspace --name Hero
oframe identity canvas --width 32 --height 32 D:\my-workspace\Hero   # 生成前置
oframe provider config set --key <你的密钥> doubao
oframe generation plan  --directions 4 D:\my-workspace\Hero          # 零调用确认预览
oframe generation run   --directions 4 --yes D:\my-workspace\Hero    # 确认后执行
oframe validate D:\my-workspace\Hero
oframe export create --output D:\my-game\assets --target godot D:\my-workspace\Hero
```

## 文档

| 文档 | 内容 |
|---|---|
| `docs/user-guide.md` | 用户指南：全流程、镜像语义、生成确认、成本说明、FAQ |
| `docs/cli-guide.md` | CLI 参考：命令、批量/CI 用法、双端一致性说明 |
| `docs/example-walkthrough.md` | 示例走查：一个角色从文字到 4 方向行走资产 |
| `docs/product-description.md` | 产品描述（GitHub About / 发布用） |
| `examples/hero-walk` | 可直接打开的示例身份包 + generic/godot 导出包 |

## 技术栈

| 层 | 技术 |
|---|---|
| 桌面壳 | Go + Wails v2 |
| 前端 | React 19 + TypeScript + Vite |
| 预览/编辑画布 | PixiJS 8（最近邻采样、纹理图集、降级缩放） |
| 持久化 | SQLite（modernc.org/sqlite）+ 身份包目录 manifest |
| CLI | oframe（Go 同源，单核多端） |
| 生成 provider | Doubao（默认）/ OpenAI gpt-image-2 / Agnes，可插拔 |

## 仓库结构

```
main.go / app.go / bindings.go …   Wails v2 桌面壳（类型化 Go bindings）
frontend/                          React + TypeScript + PixiJS 前端（Vite）
  src/pages/                       启动页、三标签（制作/验收/导出）、制作子页
  src/components/                  任务抽屉、设置面板、PixelCanvas（图集+降级缩放）
  src/styles/                      暖白/深墨双主题令牌
  wailsjs/                         wails generate module 生成的绑定
core/                              Go 核心库（领域逻辑唯一来源，design D1）
  identity/  角色身份包（manifest/画布/锚点/素材/候选历史）
  motion/    动作、方向集、帧序列、镜像派生
  pipeline/  PerfectPixel filmstrip 确定性管线（切片/抠图/对齐/量化/质量）
  provider/  生成 provider（Doubao/OpenAI/Agnes）、重试硬上限、调用统计
  service/   GUI/CLI 共享 application service（生成确认/任务队列/验收/导出）
  task/      可恢复任务队列（SQLite）+ 幂等缓存
  edit/      轻量编辑（帧/序列/锚点/批量，指令可回放）
  version/   不可变身份版本 + 追加式操作日志 + 回退
  assetexport/ 导出包（generic/godot/unity）+ 校验
  workspace/ 工作区 · settings/ 本地设置 · store/ SQLite 迁移 · logging/ 日志
cmd/oframe/        oframe CLI（与 GUI 共享核心）
cmd/examplegen/    示例生成器（合成 filmstrip，离线产出示例身份包）
examples/          hero-walk 示例资产 + 导出包
docs/              用户/CLI/示例/产品描述文档
```

## 构建与验证

```powershell
# Go 侧（含 Wails 壳）
go build ./...          # 全模块可编译
go test -count=1 ./...  # 单元测试（全绿基线）
go vet ./...            # 静态检查
gofmt -l .              # 格式检查（应无输出）

# 前端（独立类型检查/构建）
cd frontend
npm run typecheck
npm run build
npm run check:pixel-atlas    # PixelCanvas 图集/降级缩放纯函数校验

# Wails GUI（网络受限环境下先 rm -rf frontend/dist + npm run build，再 wails build -s -m -webview2 browser）
wails build             # 产物在 build/bin/OFrameCharacterWorkbench.exe
wails build -nsis       # 另产出 NSIS 安装包（需 makensis 在 PATH）
```

> 离线构建：Go 依赖走本地 module cache（`GOPROXY=off GOSUMDB=off`）；
> 测试均为确定性合成图像，不调用真实付费 API。

## 项目状态与 Roadmap

- **状态**：公开 Beta 就绪（OpenSpec 任务 67/68 完成）。
- **已支持**：文字/参考图入口、4/8 方向自动镜像、filmstrip 确定性管线、质量验收与
  候选历史、版本化回退与操作日志回退、可恢复任务队列、轻量编辑 GUI、多引擎导出、
  启动页分类管理（卡片/拖拽/右键删分类）、动森风格双主题界面、CLI、Windows 打包
  （NSIS + 便携版）、示例资产与文档。
- **Roadmap**：
  - 更多引擎导出格式、模板与风格库
  - macOS / Linux
  - 批量任务与批处理增强

## License

[MIT](LICENSE) — Copyright (c) 2026 康坤涛 (k1104480005)
