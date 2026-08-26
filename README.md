# OFrame Character Workbench（角色动画资产工作台）

面向独立游戏与像素游戏开发者的 2D 角色动画资产制作工作台（工作代号：OFrame Character）。
以可版本化的「角色身份包」为根对象，将文字、参考图或既有精灵转化为可直接导入
Godot / Unity / 通用序列帧引擎的动画资产。首发为本地优先桌面工作台（Windows 10/11 x64），
oframe CLI 与 Wails GUI 共享同一 Go 核心库（单核多端）。

领域语言与术语见 `CONTEXT.md`；总体计划见 `PLAN.md`；OpenSpec 变更（规格/设计/任务）见 `openspec/`。

## 仓库结构（阶段 3：Provider 与生成确认 + 共享 application service）

```
main.go / app.go       Wails v2 桌面壳（GUI 入口；绑定方法即类型化 Go bindings）
bindings.go            身份包会话 + 身份子页绑定（经 core/identity、core/version）
generation.go          Provider 配置/验证、调用统计、PerfectPixel 预设、
                       生成确认绑定（经共享 core/service，GUI/CLI 同源）
tasks.go               全局任务抽屉的 typed task registry（完整队列在任务 6.x）
wails.json             Wails 项目配置
build/                 Windows 打包资源（appicon、icon.ico、info.json）
frontend/              React + TypeScript + PixiJS 前端（Vite）
  src/pages/           启动页、三标签主界面、制作子页（身份/动作/编辑）
  src/components/      Tabs、TaskDrawer（全局任务抽屉）、SettingsPanel（provider/
                       密钥/统计设置）、PixelCanvas（PixiJS 骨架）
  src/styles/          暖白/深墨双主题令牌（8px 栅格、像素边框、洋红仅用于抠图）
  wailsjs/             wails generate module 生成的类型化绑定
  dist/                vite build 输出（main.go 经 //go:embed 嵌入；提交 index.html
                       占位页保证全新克隆可直接 go build，真实构建产物会替换它）
core/                Go 核心库（领域逻辑唯一来源，design D1）
  identity/          角色身份包：manifest、创建/打开、逻辑画布、锚点、素材区
                     （阶段 3：参考图 1 主 + 最多 2 辅助角色语义）
  motion/            动作/方向集/帧序列（骨架，任务 3.x）
  pipeline/          PerfectPixel 预设（四个风格预设 + 动作预设）与提示词快照
                     （阶段 3；胶片条执行在任务 5.x）
  provider/          生成 provider 抽象与适配器（Doubao 默认 / gpt-image-2 /
                     Agnes）、注册表、重试硬上限、配置校验、调用统计
  service/           GUI/CLI 共享 application service（阶段 3）：provider 配置/
                     验证、预设、生成确认 Prepare/Confirm
  settings/          本地设置存储（密钥/模型/调用统计，JSON，%AppData%）
  task/              可恢复任务队列（骨架，任务 6.x）
  edit/              轻量编辑（骨架，任务 7.x）
  version/           不可变身份版本管理
  export/            导出包（骨架，任务 11.x）
  workspace/         工作区：组织与管理角色身份包的本地空间
  store/             本地应用库：SQLite schema 与版本化迁移
  logging/           结构化日志（log/slog）
  pathutil/          Windows 优先的路径处理
cmd/oframe/          oframe CLI（与 GUI 共享核心库的可脚本化入口；provider 与
                     generation plan/run 命令与 GUI 走同一 core/service）
docs/screenshots/    wails dev 冒烟验证截图
```

GUI 与 CLI 共享同一 Go 核心库：Wails 绑定方法（`main.go`/`app.go`/`bindings.go`）
直接调用 `core/workspace`、`core/identity`、`core/version`，与 `cmd/oframe` 走同一
代码路径，行为不会漂移（design D1/D12）。

## 构建与验证

```powershell
# Go 侧（含 Wails 壳）
go build ./...          # 全模块可编译
go test ./...           # 单元测试（身份包、身份版本、工作区、绑定层、任务注册表）
go vet ./...            # 静态检查
gofmt -l .              # 格式检查（应无输出）

# oframe CLI
go run ./cmd/oframe --help
go run ./cmd/oframe identity create --workspace <工作区目录> --name Hero --json
go run ./cmd/oframe identity open <身份包目录> --json

# Wails GUI
wails dev               # 开发模式（热重载 + 桌面窗口）
wails build             # 产物在 build/bin/OFrameCharacterWorkbench.exe

# 前端（独立于 Wails 的类型检查/构建）
cd frontend
npm install
npm run typecheck       # tsc --noEmit
npm run build           # vite build → dist/
npm run check:pixel-atlas   # PixelCanvas 图集/降级缩放纯函数校验（13.3）
```

> 预览画布性能（任务 13.3）：`PixelCanvas` 将所有帧解码进**单张纹理图集**
> （一次解码、一次 GPU 上传，帧精灵引用子区域），舞台只构建一次，播放仅切换
> 可见帧与锚点标记（不再每 tick 重建/重复解码）；画布超过像素预算时**降级缩放**
> 并关闭不可读的像素网格。纯函数在 `frontend/src/components/pixelAtlas.ts`，
> 由 `npm run check:pixel-atlas` 验证。

> 说明：`main.go` 通过 `//go:embed all:frontend/dist` 把前端构建产物嵌入二进制。
> 仓库提交了一个最小 `frontend/dist/index.html` 占位页（配合 `.gitignore` 中
> `frontend/dist/*` 与 `!frontend/dist/index.html` 规则），保证全新克隆未执行
> 前端构建时 `go build ./...` / `go test ./...` / `go vet ./...` 仍可运行；
> `npm run build`（或 `wails build` 内置的前端构建）会以真实构建产物替换占位页。

## 阶段说明

- **阶段 1**：Go 核心与工作区 —— 核心模块骨架、身份包核心（manifest/创建/打开/
  身份定义入口/逻辑画布/锚点）、不可变身份版本模型、工作区、SQLite schema/迁移、
  基础 oframe CLI、日志、Windows 路径处理。
- **阶段 2**：Wails v2 桌面壳 + React/TypeScript/PixiJS 前端骨架 —— 启动页
  （仅选择/创建身份包）、制作/验收/导出三标签、制作内身份/动作/编辑子页、暖白/深墨
  双主题视觉系统、全局任务抽屉、类型化 Go bindings 与 runtime events；GUI 与 CLI
  共享同一核心 workspace/identity 服务。
- **阶段 3（当前）**：Provider 与生成确认 + 共享 application service ——
  身份参考图 1 主 + 最多 2 辅助角色语义；PerfectPixel 四个风格预设与动作预设
  数据结构、提示词快照（core/pipeline）；统一 Provider 接口与 Doubao（默认）/
  gpt-image-2 / Agnes 适配器（core/provider，fake transport 单元测试，不调用
  真实付费服务）；本地密钥/模型配置与离线校验、调用统计（core/settings）；
  生成确认流程（外发素材 / provider/model / 方向数 / 预算 / 每方向最多 3 次总
  尝试，确认后执行、取消零调用）；GUI/CLI 共享 core/service，CLI 新增
  `provider` 与 `generation plan|run` 命令，GUI 新增设置面板与动作页生成确认预览。
- **后续阶段**：见 `openspec/changes/build-oframe-character-workbench/tasks.md`。
  确定性 filmstrip 图像管线（5.x）、完整可恢复任务队列（6.x）、质量验收（8.x）
  与导出（11.x）尚未实现。

## 依赖说明

- Go 核心库仅依赖标准库 + `github.com/wailsapp/wails/v2`（桌面壳）。
- 本地应用库（`core/store`）通过 `database/sql` 注入 SQLite 驱动（计划使用纯 Go 的
  `modernc.org/sqlite`）；迁移框架与 schema 定义不依赖驱动即可测试。
- 前端依赖：React 19、Vite 7、PixiJS 8、`@fontsource/press-start-2p`（像素字体点缀）。
- 离线构建：Go 模块已缓存于本机 module cache（`GOPROXY=off`、`GOSUMDB=off` 可用）；
  npm 依赖需 registry 可达。
