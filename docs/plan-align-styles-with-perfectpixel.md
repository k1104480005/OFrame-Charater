# 执行计划：风格系统对齐 perfectpixel（Style Alignment Plan）

> 本文档是自包含的执行计划，供独立执行者使用。执行前请通读全文，按步骤顺序实施，
> 每步之后运行对应验证命令。**不要执行任何 git commit / push。**

## 0. 背景与目标

FrameBaker 集成了 perfectpixel-studio（https://github.com/gykim80/perfectpixel-studio）
的确定性胶片条管线。经源码核对，perfectpixel 的"风格"不只是提示词短语，而是三层体系：

1. **风格契约文本**（`internal/sprite/prompt.go` 的 `StylePresets`）：长契约 + 显式负面清单（"Never use ..."）。
2. **结构契约注入**：对像素类风格，提示词额外拼接 `spriteDesignContract()`（游戏精灵设计契约）与
   `lowResPixelContract()`（低清渲染契约）；另有 `canvasContract()`（洋红抠图底，本计划**不采用**，见 §6）。
3. **风格专属调色板后处理**（`internal/sprite/pixelize.go`）：
   ```go
   PaletteSizeForStyle:  retro16 → 16 色;  pixel → 32 色;  chibi/cartoon/custom → 0（跳过量化强制）
   ```

当前 FrameBaker（`core/pipeline/presets.go`）的四个风格预设是自拟的泛化短语，与 perfectpixel
实际契约不一致，且没有结构契约注入与风格→调色板映射。**本计划将其对齐。**

对齐后预期效果：选择 `pixel` / `retro16` 风格时，生成提示词带完整契约与负面清单，
后处理按风格使用 32/16 色调色板；`chibi` / `cartoon` / 自定义风格不做量化强制。

## 1. 涉及文件

| 文件 | 改动 |
|---|---|
| `core/pipeline/presets.go` | 重写四个风格预设（ID/名称/描述/PromptSuffix），新增契约常量与注入函数 |
| `core/pipeline/prompt.go` | `BuildCharacterPrompt` 与 `BuildPrompt` 注入结构契约 |
| `core/pipeline/process.go` | `PaletteOptions` 增加 `Skip`；`ProcessFilmstrip` 支持"跳过量化但仍计算调色板供评分" |
| `core/pipeline/palette.go` | 无需改动（`DefaultMaxPaletteColors = 32` 保留） |
| `core/service/generation.go` | `processOptions(plan)` 按 `plan.Prompt` 的风格 ID 映射调色板参数 |
| `core/pipeline/presets_test.go` | 更新预设断言（4 个新 ID/非空校验） |
| `core/pipeline/process_test.go` / `prompt_test.go`（若存在） | 新增对齐测试 |
| `frontend/src/pages/make/IdentityPage.tsx` | 默认风格 fallback 字符串 `"pixel_classic"` → `"pixel"`（两处） |

**不需要**新增/修改任何 Wails 绑定（无新 API）；`frontend/wailsjs/` 不动。

## 2. 第一步：重写风格预设（`core/pipeline/presets.go`）

用以下内容**整块替换**现有的 `StylePresetClassic/Modern/Minimal/RetroArcade` 定义与
`StylePresets()` 函数（保留 `StylePreset` 结构体与其余代码不动）：

```go
// 契约文本来源：perfectpixel-studio internal/sprite/prompt.go（逐字对齐，勿改写）。
const (
	ContractPixel   = "true low-resolution pixel-art game sprite, like a 32-64px sprite enlarged on the canvas, " +
		"chunky readable silhouette, clean dark 1px outline, visible square pixel blocks, " +
		"grid-aligned hard pixel edges, limited shared palette, solid tone clusters, " +
		"flat color shading with at most one highlight step and one shadow step, " +
		"simple readable face and clearly separated limbs. " +
		"Never use painterly rendering, smooth gradients, airbrush shading, glossy lighting, " +
		"anti-aliased fine detail, high-definition pixel art, fine-grained pixel art, anime illustration, concept art, or 3D rendering."
	ContractChibi   = "cute chibi game sprite with oversized head and small body, " +
		"bold dark outline, flat bright colors, minimal shading, large expressive eyes, " +
		"clean cartoon shapes readable at small size. " +
		"Never use realistic proportions, gradients, or painterly detail."
	ContractCartoon = "clean 2D cartoon game sprite, bold uniform outline, flat vivid colors, " +
		"simple two-tone cel shading, smooth rounded shapes, expressive but simple face. " +
		"Never use pixelation, gradients, photo textures, or 3D rendering."
	ContractRetro16 = "16-bit retro console era game sprite, restrained palette of 16-24 colors, " +
		"dark outline, dithering only where needed, compact proportions, " +
		"crisp hard pixel edges like a classic arcade fighter sprite. " +
		"Never use modern smooth shading or high-resolution detail."
)

var (
	StylePresetClassic = StylePreset{ID: "pixel", Name: "真·低清像素", Description: "perfectpixel 默认风格：32-64px 精灵放大感、1px 描边、有限调色板、平涂", PromptSuffix: ContractPixel}
	StylePresetChibi   = StylePreset{ID: "chibi", Name: "Q版", Description: "Q版风格：大头小身、粗描边、平涂亮色、大眼睛", PromptSuffix: ContractChibi}
	StylePresetCartoon = StylePreset{ID: "cartoon", Name: "卡通（非像素）", Description: "干净 2D 卡通：赛璐璐上色，明确禁止像素化", PromptSuffix: ContractCartoon}
	StylePresetRetro16 = StylePreset{ID: "retro16", Name: "复古 16-bit", Description: "16 位主机风：16-24 色克制调色板、深描边、必要处抖动", PromptSuffix: ContractRetro16}
)

func StylePresets() []StylePreset {
	return []StylePreset{StylePresetClassic, StylePresetChibi, StylePresetCartoon, StylePresetRetro16}
}
```

注意：
- 保留导出变量名 `StylePresetClassic`（`core/service/base_character.go` 用它作默认风格），
  其 ID 变为 `"pixel"`。**删除** `StylePresetModern` / `StylePresetMinimal` / `StylePresetRetroArcade`。
- 全仓库搜索旧 ID 字符串并更新：`pixel_classic`、`pixel_modern`、`pixel_minimal`、`pixel_retro`。
  已知出现点：`core/service/base_character.go`（默认 fallback，用变量则无需改）、
  `frontend/src/pages/make/IdentityPage.tsx`（两处 fallback `"pixel_classic"` → `"pixel"`）、
  `core/pipeline/presets_test.go`、可能存在的 `*_test.go` 断言。

## 3. 第二步：结构契约注入（`core/pipeline/prompt.go`）

新增两个函数（文本逐字取自 perfectpixel `spriteDesignContract()` / `lowResPixelContract()`）：

```go
// spriteDesignContract 与 lowResPixelContract 逐字对齐 perfectpixel
// internal/sprite/prompt.go，对像素类风格注入。
func spriteDesignContract() string {
	return "Game-sprite design contract:\n" +
		"- Interpret the subject as a game-ready character sprite, not an illustration, poster, sticker, mascot logo, or concept-art render.\n" +
		"- Preserve the subject's identity through a strong silhouette, hairstyle, outfit shapes, accessories, weapon or signature prop, and dominant color blocks.\n" +
		"- Simplify anatomy into readable sprite shapes: compact torso, clear head shape, simple arms and legs, minimal joint detail, no tiny anatomy rendering.\n" +
		"- Hair, clothing layers, capes, hats, weapons and accessories should read as distinct hard-edged pixel shapes, not detailed painted textures.\n" +
		"- Keep the face simple at sprite scale: readable eyes and mouth, minimal facial detail, no realistic nose or painted skin texture.\n"
}

func lowResPixelContract() string {
	return "Pixel rendering contract:\n" +
		"- The image must look like a 32-64px game sprite enlarged to the canvas, not newly painted at high resolution.\n" +
		"- Use chunky square pixel blocks, clean 1px outline, solid tone clusters, limited palette, minimal two-step flat shading.\n" +
		"- No dithering, no smooth gradients, no soft shadow, no blur, no airbrush, no texture, no fine hair strands, no tiny jewelry detail that would vanish at 64px.\n" +
		"- Every important shape must remain readable when shrunk to a thumbnail: silhouette first, details second.\n"
}

// injectPixelContracts 按预设 ID 注入（有意不用 perfectpixel 的"文本包含 pixel
// 子串"判断——该判断会让 cartoon 的 "Never use pixelation" 误触发注入）。
func injectPixelContracts(styleID string) string {
	if styleID == "pixel" || styleID == "retro16" {
		return "\n" + spriteDesignContract() + "\n" + lowResPixelContract()
	}
	return ""
}
```

在 `BuildCharacterPrompt` 与 `BuildPrompt` 中，把风格后缀拼进提示词之后追加
`injectPixelContracts(style.ID)`（保持既有的提示词组装顺序与快照字段不变；
先读这两个函数的当前实现，选择在 style 后缀之后、画布/帧约束之前插入）。

**不采用** perfectpixel 的 `canvasContract()`（洋红 #FF00FF 抠图底）——我们的抠图是
YCbCr 色度键 + "transparent background" 提示词路线，保持不变（见 §6）。

## 4. 第三步：风格→调色板映射（`core/pipeline/process.go` + `core/service/generation.go`）

### 4.1 pipeline 侧

`PaletteOptions` 增加字段：

```go
type PaletteOptions struct {
	MaxColors int
	// Skip 为 true 时不执行调色板量化（chibi/cartoon/自定义风格），
	// 但仍会为质量评分计算共享调色板。
	Skip bool
}
```

`ProcessFilmstrip` 第 6 步改为：

```go
// 6. Shared palette quantization (task 5.4) — per-style, skippable.
var palette []color.RGBA  // 使用现有调色板类型（对照 BuildSharedPalette 返回值）
maxColors := opts.Palette.MaxColors
if maxColors <= 0 && !opts.Palette.Skip {
	maxColors = DefaultMaxPaletteColors
}
if opts.Palette.Skip {
	// 评分仍需要共享调色板作为指标输入；帧保持未量化。
	palette, err = BuildSharedPalette(aligned, DefaultMaxPaletteColors)
	if err != nil {
		return fail(fmt.Errorf("pipeline: build shared palette: %w", err))
	}
} else {
	palette, err = BuildSharedPalette(aligned, maxColors)
	if err != nil {
		return fail(fmt.Errorf("pipeline: build shared palette: %w", err))
	}
	final, err = QuantizeToPalette(aligned, palette)
	if err != nil {
		return fail(fmt.Errorf("pipeline: quantize to shared palette: %w", err))
	}
}
```

（以现有代码为准做最小改写；`final` 变量的声明与后续使用保持一致。）

新增映射函数（放在 `presets.go`）：

```go
// PaletteSizeForStyle 对齐 perfectpixel pixelize.go：retro16→16，pixel→32，
// 其余（chibi/cartoon/custom）返回 0 表示跳过量化强制。
func PaletteSizeForStyle(styleID string) int {
	switch styleID {
	case "retro16":
		return 16
	case "pixel":
		return 32
	default:
		return 0
	}
}
```

### 4.2 service 侧（`core/service/generation.go` 的 `processOptions`）

```go
func (s *Service) processOptions(plan *GenerationPlan) pipeline.ProcessOptions {
	opts := pipeline.ProcessOptions{}
	if len(plan.Anchors) > 0 {
		opts.Anchors = plan.Anchors
	}
	if pkg, err := identity.Open(plan.PackagePath); err == nil {
		opts.PerfectPixelStandard = pkg.PerfectPixelStandard()
	}
	// 风格→调色板对齐 perfectpixel：plan.Prompt.StylePresetID（已核对字段名）
	// 决定量化策略。
	if size := pipeline.PaletteSizeForStyle(plan.Prompt.StylePresetID); size > 0 {
		opts.Palette.MaxColors = size
	} else {
		opts.Palette.Skip = true
	}
	return opts
}
```

（执行者需核对 `plan.Prompt` 中风格 ID 的实际字段名——`PromptSnapshot` 结构体在
`core/pipeline/prompt.go`，确认后使用正确字段。）

## 5. 第四步：前端默认风格（`frontend/src/pages/make/IdentityPage.tsx`）

两处 fallback 字符串 `"pixel_classic"` 改为 `"pixel"`：
- 任务初始化 `useEffect`：`const style = styles[0]?.id || "pixel";`
- "添加任务"按钮：`styles[0]?.id || "pixel"`。

（风格下拉的选项与名称来自 `PresetCatalog` 接口，改后端后自动更新；确认门快照里
`stylePresetId` 显示逻辑不变。）

## 6. 有意保留的差异（执行者不要"顺手"改动）

1. **洋红抠图底（canvasContract）不采用**：我们的抠图是 YCbCr 色度键 + despill + 泛洪填充，
   提示词走 "transparent background" 路线。切换洋红契约属于独立的大改动，本计划不含。
2. perfectpixel 用"文本包含 pixel 子串"决定是否注入契约，会让 cartoon 误注入；
   我们按预设 ID 注入（pixel/retro16），是**有意的偏离**。
3. Skip 量化时评分仍使用共享调色板（DefaultMaxPaletteColors）作为指标输入——
   与 perfectpixel 的"完全不做后处理"略有不同，是为了不破坏现有质量评分链路。

## 7. 测试要求

1. `core/pipeline/presets_test.go`：更新为 4 个预设（ID: pixel/chibi/cartoon/retro16），
   保留"ID/Name/PromptSuffix 非空"校验；断言 `PaletteSizeForStyle` 映射
   （retro16→16、pixel→32、chibi/cartoon/custom→0）。
2. 新增 prompt 断言：`BuildCharacterPrompt`（及 `BuildPrompt`）在 pixel/retro16 风格时
   提示词包含 `"Game-sprite design contract:"` 与 `"Pixel rendering contract:"`，
   在 chibi/cartoon 时不包含。
3. 新增 process 断言：`ProcessOptions{Palette: PaletteOptions{MaxColors: 16}}` 时输出帧
   不透明颜色种类 ≤ 16；`Skip: true` 时输出帧保留原色数（用合成条带构造 >16 色的帧验证）。
4. `core/service`：`processOptions` 映射测试（构造 plan 使 `Prompt.StylePreset` 分别为
   pixel/retro16/chibi，断言 MaxColors/Skip）。
5. 全量回归：`go test -count=1 ./...` 必须全绿。

## 8. 验证与打包（每步完成后按需执行；最终必须全部执行）

```powershell
# 后端（在仓库根目录）
go test -count=1 ./...
# 前端（必须在 frontend 目录，根目录没有 package.json）
cd frontend; pnpm run typecheck; pnpm run build; cd ..
# 打包
wails build -platform windows/amd64 -nsis
```

产物：`build/bin/OFrameCharacterWorkbench.exe` 与
`build/bin/oframe-character-workbench-amd64-installer.exe`。

## 9. 完成定义（DoD）

> 2026-08-31 执行完毕，逐项核对如下；后续增强（界面量化说明、内置负面提示词展示、
> 基础角色单图量化与尺寸规范化）在同日身份流程加固中完成。

- [x] 四个预设为 pixel/chibi/cartoon/retro16，契约文本与本文档逐字一致
- [x] pixel/retro16 的提示词包含两段结构契约；chibi/cartoon 不包含
- [x] `PaletteSizeForStyle` 映射生效：retro16 量化到 16 色，pixel 32 色，chibi/cartoon/自定义跳过量化的 ApplyPalette 但评分链路不报错
- [x] 旧 ID 字符串（pixel_classic 等）在全仓库无残留（测试除外——如有历史快照断言，改为新 ID）
- [x] `go test -count=1 ./...` 全绿；前端 typecheck/build 通过；Wails+NSIS 打包成功
- [x] 不提交、不推送（计划期约束；本次提交推送为用户事后明确授权，见 PUSH.md）
