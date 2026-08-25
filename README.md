# OFrame Character Workbench（角色动画资产工作台）

面向独立游戏与像素游戏开发者的 2D 角色动画资产制作工作台（工作代号：OFrame Character）。
以可版本化的「角色身份包」为根对象，将文字、参考图或既有精灵转化为可直接导入
Godot / Unity / 通用序列帧引擎的动画资产。首发为本地优先桌面工作台（Windows 10/11 x64），
oframe CLI 与 Wails GUI 共享同一 Go 核心库（单核多端）。

领域语言与术语见 `CONTEXT.md`；总体计划见 `PLAN.md`；OpenSpec 变更（规格/设计/任务）见 `openspec/`。

## 仓库结构（阶段 1：Go 核心与工作区）

```
core/                Go 核心库（领域逻辑唯一来源，design D1）
  identity/          角色身份包：manifest、创建/打开、逻辑画布、锚点、素材区
  motion/            动作/方向集/帧序列（骨架，任务 3.x）
  pipeline/          PerfectPixel filmstrip 管线（骨架，任务 5.x）
  provider/          生成 provider 抽象与适配器（骨架，任务 4.x）
  task/              可恢复任务队列（骨架，任务 6.x）
  edit/              轻量编辑（骨架，任务 7.x）
  version/           不可变身份版本管理
  export/            导出包（骨架，任务 11.x）
  workspace/         工作区：组织与管理角色身份包的本地空间
  store/             本地应用库：SQLite schema 与版本化迁移
  logging/           结构化日志（log/slog）
  pathutil/          Windows 优先的路径处理
cmd/oframe/          oframe CLI（与 GUI 共享核心库的可脚本化入口）
```

## 构建与验证

```powershell
go build ./...          # 全模块可编译
go test ./...           # 单元测试（含身份包、身份版本、工作区、迁移框架）
go vet ./...            # 静态检查
gofmt -l .              # 格式检查（应无输出）

go run ./cmd/oframe --help
go run ./cmd/oframe identity create --workspace <工作区目录> --name Hero --json
go run ./cmd/oframe identity open <身份包目录> --json
```

## 阶段说明

- **阶段 1（当前）**：Go 核心与工作区 —— 核心模块骨架、身份包核心（manifest/创建/打开/
  身份定义入口/逻辑画布/锚点）、不可变身份版本模型、工作区、SQLite schema/迁移、
  基础 oframe CLI、日志、Windows 路径处理。不含 Wails 前端、provider、图像管线与后续阶段。
- **后续阶段**：见 `openspec/changes/build-oframe-character-workbench/tasks.md`。

## 依赖说明

核心库当前仅依赖 Go 标准库。本地应用库（`core/store`）通过 `database/sql` 注入 SQLite
驱动（计划使用纯 Go 的 `modernc.org/sqlite`，注册名为 `sqlite`）；在网络可用后加入依赖即可，
迁移框架与 schema 定义不依赖驱动即可测试。
