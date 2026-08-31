package service

import (
	"context"
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oframe/character-workbench/core/identity"
	"github.com/oframe/character-workbench/core/pipeline"
	"github.com/oframe/character-workbench/core/provider"
	"github.com/oframe/character-workbench/core/version"
)

// prepareBaseCharacter builds the base-character confirmation plan (kind
// base-character): ONE image call producing the identity's base sprite. It
// follows the same gates as filmstrip generation — provider registry,
// capability/model validation, budget numbers — but the prompt is the single
// character prompt (pipeline.BuildCharacterPrompt) and no motion/direction
// strategy applies. No external call is made here.
func (s *Service) prepareBaseCharacter(pkg *identity.Package, req GenerationRequest) (*GenerationPlan, error) {
	ps := s.settings.ProviderSettings()
	if len(ps.Providers) == 0 {
		return nil, fmt.Errorf("service: 尚未配置任何 Provider — 请先在设置中添加图像 Provider")
	}
	providerID := req.ProviderID
	if providerID == "" {
		providerID = ps.ActiveProvider
	}
	if providerID == "" {
		providerID = provider.DefaultProviderID
	}
	prov, err := s.registry.Get(providerID)
	if err != nil {
		return nil, err
	}
	cfg := ps.ConfigFor(providerID)
	model, err := provider.ResolveValidatedModel(prov.Capabilities(), cfg, provider.ModalityImage, req.Model)
	if err != nil {
		return nil, fmt.Errorf("service: %w", err)
	}
	styleID := req.StylePresetID
	if styleID == "" {
		styleID = pipeline.StylePresetClassic.ID
	}
	style, err := pipeline.StylePresetByID(styleID)
	if err != nil {
		return nil, err
	}
	// 自定义风格：非空时优先于内置预设，直接作为提示词风格片段。
	if custom := strings.TrimSpace(req.StyleCustom); custom != "" {
		style = pipeline.StylePreset{ID: "custom", Name: "自定义", PromptSuffix: custom}
	}
	// 任务级提示词覆盖：非空时本次生成使用它；空 → 已保存的身份描述。
	description := strings.TrimSpace(req.Description)
	if description == "" {
		description = pkg.Description()
	}
	canvas := pkg.LogicalCanvas()
	if canvas == nil {
		return nil, fmt.Errorf("service: logical canvas must be set before planning base-character generation")
	}
	outbound := make([]OutboundMaterial, 0, len(pkg.ReferenceImages()))
	refs := make([]pipeline.ReferenceImageRef, 0, len(pkg.ReferenceImages()))
	for _, material := range pkg.ReferenceImages() {
		abs, err := pkg.MaterialPath(material)
		if err != nil {
			return nil, err
		}
		outbound = append(outbound, OutboundMaterial{MaterialID: material.ID, Kind: material.Kind, Role: material.Role, Name: material.Name, Path: abs})
		refs = append(refs, pipeline.ReferenceImageRef{MaterialID: material.ID, Role: material.Role, Name: material.Name})
	}
	prompt, err := pipeline.BuildCharacterPrompt(description, style, canvas.UnitWidth, canvas.UnitHeight, refs)
	if err != nil {
		return nil, err
	}
	maxAttempts := req.MaxAttemptsPerDirection
	if maxAttempts <= 0 {
		maxAttempts = cfg.EffectiveMaxAttempts()
	}
	price := cfg.EffectivePrice()
	plan := &GenerationPlan{
		ID: newPlanID(), Kind: PlanKindBaseCharacter, Capability: provider.ModalityImage.String(),
		PackagePath: pkg.Root(), ProviderID: providerID, ProviderType: cfg.EffectiveType(), Model: model,
		Directions: 1, BasicDirections: 1, BasicLabels: []string{"base-character"}, ExpectedCalls: 1,
		MaxAttemptsPerDirection: maxAttempts, MaxTotalAttempts: maxAttempts, OutboundMaterials: outbound,
		Prompt: prompt, Canvas: *canvas, FrameCount: 1, CostPerCall: price, Currency: provider.Currency(providerID),
		ExpectedCost: round2(price), MaxCost: round2(float64(maxAttempts) * price), Status: PlanPending,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	s.plans.put(plan)
	s.log.Info("base character plan prepared", "plan", plan.ID, "provider", providerID, "model", model,
		"outboundMaterials", len(outbound), "maxAttempts", maxAttempts)
	return plan, nil
}

// BaseCharacterCandidates returns the recorded base-character candidates of a
// package. It opens the package FRESH from disk: generation execution writes
// candidates through its own instance, so the GUI session instance may lag.
func (s *Service) BaseCharacterCandidates(pkgPath string) ([]identity.BaseCharacterCandidate, error) {
	pkg, err := identity.Open(pkgPath)
	if err != nil {
		return nil, err
	}
	return pkg.BaseCharacterCandidates(), nil
}

// AdoptBaseCharacter adopts one candidate as the package's identity basis,
// opening the package fresh from disk for the same reason.
func (s *Service) AdoptBaseCharacter(pkgPath, candidateID string) error {
	pkg, err := identity.Open(pkgPath)
	if err != nil {
		return err
	}
	return pkg.AdoptBaseCharacter(candidateID)
}

// RejectBaseCharacter marks a pending base-character candidate as rejected
// (弃用) so it can no longer be adopted. No external call, no basis change.
func (s *Service) RejectBaseCharacter(pkgPath, candidateID string) error {
	pkg, err := identity.Open(pkgPath)
	if err != nil {
		return err
	}
	return pkg.RejectBaseCharacter(candidateID)
}

// DeleteBaseCharacter deletes a non-adopted candidate record together with its
// image file (删除候选图). No external call, no basis change — the adopted
// basis is refused by the identity layer.
func (s *Service) DeleteBaseCharacter(pkgPath, candidateID string) error {
	pkg, err := identity.Open(pkgPath)
	if err != nil {
		return err
	}
	return pkg.DeleteBaseCharacterCandidate(candidateID)
}

// importBasePrepare opens the package fresh and enforces the preconditions
// shared by plain and cropped imports: a saved logical canvas (画布优先) and
// the permanently locked import source.
func (s *Service) importBasePrepare(pkgPath string) (*identity.Package, *identity.CanvasSpec, error) {
	pkg, err := identity.Open(pkgPath)
	if err != nil {
		return nil, nil, err
	}
	canvas := pkg.LogicalCanvas()
	if canvas == nil {
		return nil, nil, fmt.Errorf("service: logical canvas must be set before importing a base character")
	}
	if err := pkg.LockBaseCharacterSource(identity.BaseCharacterSourceImport); err != nil {
		return nil, nil, fmt.Errorf("service: %w", err)
	}
	return pkg, canvas, nil
}

// persistImportDraft encodes the prepared image as the package's pending
// import draft: write a true PNG into the candidate area, record the candidate
// (provider "import"), then replace any older un-locked draft so the
// confirm-lock step always shows exactly one image (一次一张的草稿模型).
// Adopted/rejected records are never touched.
func (s *Service) persistImportDraft(pkg *identity.Package, img *image.RGBA) (identity.BaseCharacterCandidate, error) {
	pngBytes, err := pipeline.EncodeFilmstripPNG(img)
	if err != nil {
		return identity.BaseCharacterCandidate{}, err
	}
	rel := filepath.ToSlash(filepath.Join(identity.DirCandidates, fmt.Sprintf("import-%d.png", time.Now().UnixNano())))
	abs := filepath.Join(pkg.Root(), filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return identity.BaseCharacterCandidate{}, fmt.Errorf("service: create candidate area: %w", err)
	}
	if err := os.WriteFile(abs, pngBytes, 0o644); err != nil {
		return identity.BaseCharacterCandidate{}, fmt.Errorf("service: persist imported base character: %w", err)
	}
	candidate, err := pkg.AddBaseCharacterCandidateFromSource(rel, "import", "本地文件", "", identity.BaseCharacterSourceImport)
	if err != nil {
		_ = os.Remove(abs)
		return identity.BaseCharacterCandidate{}, err
	}
	for _, old := range pkg.BaseCharacterCandidates() {
		if old.ID != candidate.ID && old.Status == identity.BaseCharacterPending {
			if derr := pkg.DeleteBaseCharacterCandidate(old.ID); derr != nil {
				s.log.Warn("replace pending import draft failed", "old", old.ID, "err", derr)
			} else {
				s.log.Info("replaced pending import draft", "old", old.ID, "new", candidate.ID)
			}
		}
	}
	s.log.Info("base character imported", "candidate", candidate.ID,
		"size", fmt.Sprintf("%dx%d", img.Bounds().Dx(), img.Bounds().Dy()))
	return candidate, nil
}

// ImportBaseCharacter records a user-provided sprite image as a PENDING
// base-character candidate — the second base source beside AI generation.
// It converges on the SAME review flow (record → explicit adopt/lock) and
// involves NO external call. Imports are one image per call (一次一张); a
// successful import REPLACES the previous un-locked draft (pending) so the
// confirm-lock step always shows exactly one image. The canvas-first gate
// lives in the GUI; the base semantic is a single frame at the logical canvas
// size (frame slicing and anchors depend on it), so the image must decode and
// match the canvas exactly; anything else is rejected (拦截) with a clear
// message — the GUI then offers the aspect-locked crop tool instead.
func (s *Service) ImportBaseCharacter(pkgPath, srcPath string) (identity.BaseCharacterCandidate, error) {
	pkg, canvas, err := s.importBasePrepare(pkgPath)
	if err != nil {
		return identity.BaseCharacterCandidate{}, err
	}
	raw, err := os.ReadFile(srcPath)
	if err != nil {
		return identity.BaseCharacterCandidate{}, fmt.Errorf("service: read sprite image: %w", err)
	}
	img, err := pipeline.DecodeImageAny(raw)
	if err != nil {
		return identity.BaseCharacterCandidate{}, fmt.Errorf("service: 解码图像失败（支持 PNG/JPEG/GIF）: %w", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() != canvas.UnitWidth || bounds.Dy() != canvas.UnitHeight {
		return identity.BaseCharacterCandidate{}, fmt.Errorf(
			"service: 已拦截导入：图像尺寸 %dx%d 与逻辑画布 %dx%d 不一致 —— 请在裁剪工具中按画布比例框选，或先把图片调整为 %dx%d",
			bounds.Dx(), bounds.Dy(), canvas.UnitWidth, canvas.UnitHeight, canvas.UnitWidth, canvas.UnitHeight)
	}
	return s.persistImportDraft(pkg, img)
}

// ImportBaseCharacterCropped crops srcPath to the given source-pixel rectangle
// (GUI crop tool with aspect pre-locked to the logical canvas + guide lines)
// and registers the result, nearest-resized to the logical canvas, as the
// pending import draft — same one-draft replace rule as ImportBaseCharacter.
func (s *Service) ImportBaseCharacterCropped(pkgPath, srcPath string, x, y, w, h int) (identity.BaseCharacterCandidate, error) {
	pkg, canvas, err := s.importBasePrepare(pkgPath)
	if err != nil {
		return identity.BaseCharacterCandidate{}, err
	}
	raw, err := os.ReadFile(srcPath)
	if err != nil {
		return identity.BaseCharacterCandidate{}, fmt.Errorf("service: read sprite image: %w", err)
	}
	img, err := pipeline.DecodeImageAny(raw)
	if err != nil {
		return identity.BaseCharacterCandidate{}, fmt.Errorf("service: 解码图像失败（支持 PNG/JPEG/GIF）: %w", err)
	}
	bounds := img.Bounds()
	if w <= 0 || h <= 0 || x < 0 || y < 0 || x+w > bounds.Dx() || y+h > bounds.Dy() {
		return identity.BaseCharacterCandidate{}, fmt.Errorf(
			"service: 裁剪区域 (%d,%d) %dx%d 超出图像范围 %dx%d",
			x, y, w, h, bounds.Dx(), bounds.Dy())
	}
	// 比例校验：取整允许 1 像素误差，超出则拒绝，避免静默变形。
	idealH := float64(w) * float64(canvas.UnitHeight) / float64(canvas.UnitWidth)
	if math.Abs(float64(h)-idealH) > math.Max(1, idealH*0.02) {
		return identity.BaseCharacterCandidate{}, fmt.Errorf(
			"service: 裁剪区域比例 %d:%d 与画布 %d:%d 不符 —— 请按画布比例框选",
			w, h, canvas.UnitWidth, canvas.UnitHeight)
	}
	cropped := pipeline.ToRGBA(img.SubImage(image.Rect(x, y, x+w, y+h)))
	if cropped.Bounds().Dx() != canvas.UnitWidth || cropped.Bounds().Dy() != canvas.UnitHeight {
		cropped = resizeNearest(cropped, canvas.UnitWidth, canvas.UnitHeight)
	}
	return s.persistImportDraft(pkg, cropped)
}

func resizeNearest(src *image.RGBA, width, height int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	sb := src.Bounds()
	for y := 0; y < height; y++ {
		sy := sb.Min.Y + y*sb.Dy()/height
		for x := 0; x < width; x++ {
			sx := sb.Min.X + x*sb.Dx()/width
			dst.SetRGBA(x, y, src.RGBAAt(sx, sy))
		}
	}
	return dst
}

// runBaseCharacter executes a confirmed base-character plan through the
// persisted task queue (executePlanTask dispatch): ONE provider call → decode
// in any supported format → re-encode as true PNG → persist under the package
// candidate area → record a PENDING base-character candidate. Execution never
// changes the identity basis; adoption is the explicit user decision
// (identity.AdoptBaseCharacter).
func (s *Service) runBaseCharacter(ctx context.Context, plan *GenerationPlan, prov provider.Provider, cfg provider.ProviderConfig, refs []provider.ReferenceImage, progress func(completed, total int)) *GenerationResult {
	res := &GenerationResult{PlanID: plan.ID, Accepted: true, Status: PlanExecuted, Results: []DirectionResult{}}

	pkg, err := identity.Open(plan.PackagePath)
	if err != nil {
		return s.failPlan(plan, res, err.Error())
	}
	if err := pkg.LockBaseCharacterSource(identity.BaseCharacterSourceAI); err != nil {
		return s.failPlan(plan, res, fmt.Sprintf("lock base character source: %v", err))
	}
	if progress != nil {
		progress(1, 20) // 已提交 provider：5%（provider 调用是最长的一段）
	}

	raw, attempts, err := s.callProviderOnce(ctx, prov, cfg, plan, refs)
	if err != nil {
		return s.failPlan(plan, res, err.Error())
	}
	res.Attempts = attempts
	res.CallsMade = 1
	if progress != nil {
		progress(17, 20) // 已返回并完成解码/量化：85%
	}
	if err != nil {
		return s.failPlan(plan, res, err.Error())
	}
	res.Attempts = attempts
	res.CallsMade = 1

	img, err := pipeline.DecodeImageAny(raw)
	if err != nil {
		return s.failPlan(plan, res, fmt.Sprintf("decode base character: %v", err))
	}
	if img.Bounds().Dx() != plan.Canvas.UnitWidth || img.Bounds().Dy() != plan.Canvas.UnitHeight {
		img = resizeNearest(img, plan.Canvas.UnitWidth, plan.Canvas.UnitHeight)
	}
	if size := pipeline.PaletteSizeForStyle(plan.Prompt.StylePresetID); size > 0 {
		palette, perr := pipeline.BuildSharedPalette([]*image.RGBA{img}, size)
		if perr != nil {
			return s.failPlan(plan, res, fmt.Sprintf("build base character palette: %v", perr))
		}
		frames, perr := pipeline.QuantizeToPalette([]*image.RGBA{img}, palette)
		if perr != nil {
			return s.failPlan(plan, res, fmt.Sprintf("quantize base character palette: %v", perr))
		}
		if len(frames) == 1 {
			img = frames[0]
		}
	}
	pngBytes, err := pipeline.EncodeFilmstripPNG(img)
	if err != nil {
		return s.failPlan(plan, res, err.Error())
	}
	rel := filepath.ToSlash(filepath.Join(identity.DirCandidates, "base-"+plan.ID+".png"))
	abs := filepath.Join(plan.PackagePath, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return s.failPlan(plan, res, fmt.Sprintf("create candidate area: %v", err))
	}
	if err := os.WriteFile(abs, pngBytes, 0o644); err != nil {
		return s.failPlan(plan, res, fmt.Sprintf("persist base character: %v", err))
	}
	candidate, err := pkg.AddBaseCharacterCandidateFromSource(rel, plan.ProviderID, plan.Model, plan.Prompt.Prompt, identity.BaseCharacterSourceAI)
	if err != nil {
		_ = os.Remove(abs)
		return s.failPlan(plan, res, err.Error())
	}
	res.Results = append(res.Results, DirectionResult{
		Direction: "base-character", Attempts: attempts, Bytes: len(pngBytes), Model: plan.Model, CandidateID: candidate.ID,
	})
	if progress != nil {
		progress(1, 1)
	}
	s.plans.setStatus(plan.ID, PlanExecuted)
	s.log.Info("base character generated", "plan", plan.ID, "candidate", candidate.ID, "attempts", attempts)
	s.logGeneration(plan, res, version.ActionGeneration)
	return res
}
