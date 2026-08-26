// Command examplegen generates a deterministic OFFLINE example identity
// package — 一个完整角色从文字到 4 方向行走资产 — using the shared core with a
// SYNTHETIC filmstrip transport. No real provider call, no API key, no
// network: the filmstrip pipeline runs on a generated strip exactly as it
// would on a provider response, so the whole chain (身份包 → 逻辑画布 → 动作 →
// 生成确认 → filmstrip 管线 → 质量验收(接受) → 导出) is exercised for real.
//
// The output is a genuine identity package that opens in the workbench GUI:
//
//	examples/hero-walk/            identity package (candidates + accepted assets)
//	examples/hero-exports/generic/ generic sprite-sheet export package
//	examples/hero-exports/godot/   Godot export package
//
// Usage:
//
//	go run ./cmd/examplegen -output examples/hero-walk -name Hero
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/oframe/character-workbench/core/assetexport"
	"github.com/oframe/character-workbench/core/identity"
	"github.com/oframe/character-workbench/core/motion"
	"github.com/oframe/character-workbench/core/pipeline"
	"github.com/oframe/character-workbench/core/provider"
	"github.com/oframe/character-workbench/core/service"
)

const (
	exampleWidth  = 32
	exampleHeight = 32
	exampleFrames = 4
)

// filmstripRT is a fake provider transport answering a valid synthetic
// filmstrip PNG (magenta technical background + one opaque block per frame —
// the same deterministic pattern the tests use, so the pipeline output passes
// the acceptance threshold).
type filmstripRT struct{}

func (filmstripRT) RoundTrip(r *http.Request) (*http.Response, error) {
	canvas, err := identity.NewCanvasSpec(exampleWidth, exampleHeight)
	if err != nil {
		return nil, err
	}
	layout, err := pipeline.NormalizeFrameList(*canvas, exampleFrames)
	if err != nil {
		return nil, err
	}
	frames := make([]*image.RGBA, exampleFrames)
	for i := range frames {
		frames[i] = blockFrame(10+(i%3), 22)
	}
	strip, err := pipeline.AssembleFilmstrip(frames, layout)
	if err != nil {
		return nil, err
	}
	data, err := pipeline.EncodeFilmstripPNG(strip)
	if err != nil {
		return nil, err
	}
	payload, _ := json.Marshal(map[string]any{
		"data": []map[string]any{{"b64_json": base64.StdEncoding.EncodeToString(data)}},
	})
	return &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(payload)),
	}, nil
}

// blockFrame is a 32×32 magenta frame (洋红仅用于抠图技术背景) with a 10×10
// opaque block at (bx, by), so the deterministic pipeline sees a real sprite.
func blockFrame(bx, by int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, exampleWidth, exampleHeight))
	for y := 0; y < exampleHeight; y++ {
		for x := 0; x < exampleWidth; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 255, G: 0, B: 255, A: 255})
		}
	}
	for y := by; y < by+10 && y < exampleHeight; y++ {
		for x := bx; x < bx+10 && x < exampleWidth; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 120, G: 200, B: 80, A: 255})
		}
	}
	return img
}

func main() {
	out := flag.String("output", "examples/hero-walk", "identity package output directory")
	name := flag.String("name", "Hero", "identity name")
	exports := flag.String("exports", "examples/hero-exports", "export output base directory")
	force := flag.Bool("force", false, "remove the output directory first if it exists")
	flag.Parse()

	if err := run(*out, *name, *exports, *force); err != nil {
		fmt.Fprintln(os.Stderr, "examplegen:", err)
		os.Exit(1)
	}
}

func run(outDir, name, exportsBase string, force bool) error {
	ctx := context.Background()
	settingsDir, err := os.MkdirTemp("", "oframe-examplegen-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(settingsDir)

	svc, err := service.New(service.Options{SettingsDir: settingsDir, HTTPClient: &http.Client{Transport: filmstripRT{}}})
	if err != nil {
		return err
	}
	defer func() { _ = svc.Close() }()

	if force {
		if err := os.RemoveAll(outDir); err != nil {
			return err
		}
	}

	// 1) 身份包：文字描述 → 逻辑画布 → 脚底锚点
	pkg, err := identity.Create(outDir, name)
	if err != nil {
		return fmt.Errorf("create identity package: %w (use -force to overwrite)", err)
	}
	if err := pkg.SetTextDescription("一个绿色的像素小英雄：2D 像素风格，头戴小帽、身披披风，正面朝下行走，动作清晰连贯，适合作为游戏主角动画。"); err != nil {
		return err
	}
	if err := pkg.SetLogicalCanvas(exampleWidth, exampleHeight); err != nil {
		return err
	}
	if _, err := pkg.AddAnchorPreset(identity.PresetFeet, "脚底"); err != nil {
		return err
	}

	// 2) provider 密钥（本地保存；合成传输不产生真实调用）
	cfg, err := svc.ProviderConfig(provider.ProviderDoubao)
	if err != nil {
		return err
	}
	cfg.APIKey = "examplegen-local"
	if err := svc.SaveProviderConfig(provider.ProviderDoubao, cfg); err != nil {
		return err
	}

	// 3) 动作 + 生成确认 → filmstrip 管线（4 方向 = 3 生成 + 1 镜像）
	m, err := svc.MotionCreate(outDir, "walk", motion.DirectionStrategy{Count: 4, Mirror: true})
	if err != nil {
		return err
	}
	plan, err := svc.PrepareGeneration(ctx, service.GenerationRequest{
		PackagePath: outDir, MotionID: m.ID, StylePresetID: "pixel_classic", ActionPresetID: "walk",
	})
	if err != nil {
		return err
	}
	res, err := svc.ConfirmGeneration(ctx, plan.ID, true)
	if err != nil {
		return err
	}
	if res.Status != service.PlanExecuted {
		return fmt.Errorf("generation failed: %s %s", res.Status, res.Error)
	}
	fmt.Printf("generation: %d 次调用 / %d 次尝试 (4 方向 = %d 生成 + %d 镜像)\n",
		res.CallsMade, res.Attempts, plan.BasicDirections, plan.MirroredDirections)

	// 4) 质量验收：接受所有通过阈值的候选 → 当前动画资产
	accepted := 0
	for _, c := range svc.CandidateList(outDir) {
		dec, err := svc.CandidateDecide(ctx, outDir, c.ID, true, "示例资产：自动接受通过阈值的候选")
		if err == nil && dec.Decision == identity.CandidateAccepted {
			accepted++
			fmt.Printf("accepted candidate %s (%s, overall %.2f)\n", c.ID[:8], c.Direction, c.Scores.Overall)
		}
	}
	if accepted == 0 {
		return fmt.Errorf("no candidate passed the acceptance gate")
	}

	// 5) 导出：通用序列帧 + Godot（生成后自动校验）。导出目录先清理，
	//    使工具可重复运行（幂等）。
	for _, target := range []string{assetexport.TargetGeneric, assetexport.TargetGodot} {
		dir := filepath.Join(exportsBase, target)
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		r, err := svc.ExportPackage(outDir, dir, target, "")
		if err != nil {
			return fmt.Errorf("export %s: %w", target, err)
		}
		fmt.Printf("exported %-7s → %s (%d animations, %d×%d cell, validated)\n",
			target, r.OutputDir, len(r.Manifest.Animations), r.Manifest.CellWidth, r.Manifest.CellHeight)
	}

	// 6) 汇总
	fmt.Printf("\n示例身份包已生成：%s\n", outDir)
	fmt.Printf("  文字描述：%s\n", pkg.Description())
	fmt.Printf("  逻辑画布：%dx%d · 锚点：%d\n", exampleWidth, exampleHeight, len(pkg.Manifest().Anchors))
	fmt.Printf("  动作：walk（4 方向自动镜像）· 已接受资产：%d\n", accepted)
	fmt.Printf("  导出包：%s/{generic,godot}\n", exportsBase)
	fmt.Printf("在 GUI 启动页「选择身份包」打开 %s 即可查看示例资产。\n", outDir)
	return nil
}
