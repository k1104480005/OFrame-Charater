# 示例资产走查：一个角色从文字到 4 方向行走资产

> 本文档演示 OFrame Character 的完整主流程。示例资产位于 `examples/hero-walk`，
> 用 `cmd/examplegen`（合成 filmstrip，**零付费 API、零网络**）生成，验证了
> 身份包 → 逻辑画布 → 动作 → 生成确认 → filmstrip 管线 → 质量验收 → 导出
> 全链路真实可用（任务 13.1）。

## 0. 示例资产一览

```
examples/
  hero-walk/                   角色身份包（可直接在 GUI 打开）
    manifest.json              身份元数据 + 逻辑画布 + 脚底锚点
    motions.json               walk 动作（4 方向自动镜像）
    candidates/<id>/           3 个生成候选（right/up/down，综合 0.95）
    versions/v1/assets/        已接受的动画资产（right/up/down 各 4 帧 + left 镜像）
    exports/history.jsonl      导出历史
    log/operations.jsonl       追加式操作日志（生成/接受）
  hero-exports/
    generic/                   通用序列帧导出包（spritesheet.png + 逐帧 PNG + manifest.json）
    godot/                     Godot 导出包（含 godot.json 目标元数据）
```

## 1. 在 GUI 中查看示例

1. 启动工作台 → 启动页 → **「选择身份包」** → 选中 `examples/hero-walk`。
2. **制作 > 身份**：查看文字描述、32×32 逻辑画布、脚底锚点。
3. **制作 > 动作**：查看 `walk` 动作的方向集——`right/up/down` 为生成方向，
   `left` 为镜像派生（origin=mirrored，源=right）。
4. **验收**：选择方向后 PixelPerfect 预览回放；候选历史里能看到 3 条已接受记录
   （综合评分 0.95）；操作日志记录生成与接受事件。
5. **导出**：检视已接受资产（帧序列 + 锚点），重新导出任意引擎目标，查看导出历史。

## 2. 重新生成示例（离线、确定性）

```powershell
# 需要 Go 与本地 module cache（GOPROXY=off GOSUMDB=off）
go run ./cmd/examplegen -output examples/hero-walk -name Hero -force
```

工具产出与示例完全一致的结构；`-force` 覆盖已存在的身份包目录。
合成传输只作用于本工具的 provider 调用——它生成的 filmstrip 会被**真实的**
确定性管线处理（切片、抠图、对齐、量化、评分、验收、导出）。

## 3. 用真实 provider 复现同一流程（CLI）

示例生成的每一步都能用 `oframe` CLI + 真实密钥复现：

```powershell
# 1) 身份包 + 画布（与示例相同的描述）
oframe workspace init D:\my-workspace
oframe identity create --workspace D:\my-workspace --name Hero
oframe identity canvas --width 32 --height 32 D:\my-workspace\Hero

# 2) provider 密钥
oframe provider config set --key <你的 Doubao/OpenAI 密钥> doubao

# 3) 生成确认预览（零调用）→ 确认后执行（4 方向 = 3 生成 + 1 镜像）
oframe generation plan --directions 4 D:\my-workspace\Hero --json
oframe generation run --directions 4 --yes D:\my-workspace\Hero --json

# 4) 校验
oframe validate D:\my-workspace\Hero
```

> CLI 目前不提供「接受候选」子命令——接受在 GUI「验收」标签完成；
> 无真实 provider 时用 `go run ./cmd/examplegen` 走完整链路。

## 4. 完整流程映射（文字 → 4 方向行走资产）

| 步骤 | 领域概念 | 示例中的体现 |
|---|---|---|
| 文字描述 | 身份定义入口 | 「一个绿色的像素小英雄…」写入 manifest |
| 逻辑画布 | 统一动画单元规格 | 32×32，所有方向共享 |
| 脚底锚点 | 锚点定义与预设 | feet 预设，默认位置 (16,31) |
| walk 动作 | 动作 + 方向集 | 4 方向自动镜像 |
| filmstrip 一次生成 | 生成确认 + 确定性管线 | 每生成方向一次调用产整条胶片条 |
| 镜像派生 | 方向策略 | left ← right（独立帧序列 + 锚点 X'=width-1-X） |
| 接受 3 个候选 | 质量验收 / 候选接受 | 综合 0.95 ≥ 0.7 且确认通过 |
| generic/godot 导出 | 导出包生成与校验 | spritesheet.png + manifest + 逐帧 PNG |

## 5. 验证示例的命令

```powershell
go test -count=1 ./cmd/examplegen/          # 示例生成器回归测试
oframe validate examples/hero-walk          # 身份包校验
oframe export validate examples/hero-exports/generic
oframe export validate examples/hero-exports/godot
```

## 6. 成本说明

示例与 `examplegen` 全程**不产生任何外部调用与费用**（合成传输）；
`generation plan` 在任何 provider 下都是零调用预览，`generation run` 必须
`--yes` 显式确认后才调用真实 API。每方向最多 3 次总尝试为硬上限，相同请求
命中幂等缓存不重复计费。
