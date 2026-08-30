package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/oframe/character-workbench/core/service"
)

// cmdGeneration implements `oframe generation plan|run`: the generation
// confirmation flow over the shared application service. plan builds the
// confirmation payload (no external calls); run additionally executes the
// provider calls, but ONLY when the user explicitly confirms with --yes —
// without it the plan is printed and no call is made (杜绝静默超支).
func cmdGeneration(args []string, jsonOut bool, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("generation requires a subcommand: plan | run")
	}
	switch args[0] {
	case "plan":
		return cmdGenerationPlan(args[1:], jsonOut, stdout)
	case "run":
		return cmdGenerationRun(args[1:], jsonOut, stdout)
	default:
		return fmt.Errorf("unknown generation subcommand %q (plan|run)", args[0])
	}
}

type generationFlags struct {
	settingsDir string
	providerID  string
	model       string
	style       string
	action      string
	motionID    string
	directions  int
	frameCount  int
	maxAttempts int
}

func parseGenerationFlags(name string, args []string) (*generationFlags, string, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	g := &generationFlags{}
	fs.StringVar(&g.settingsDir, "settings-dir", "", "local settings directory")
	fs.StringVar(&g.providerID, "provider", "", "provider id (default: active provider)")
	fs.StringVar(&g.model, "model", "", "image model (default: provider default)")
	fs.StringVar(&g.style, "style", "pixel", "PerfectPixel style preset id")
	fs.StringVar(&g.action, "action", "walk", "action preset id")
	fs.StringVar(&g.motionID, "motion", "", "motion id (batch generate into a motion; default: legacy direction-count mode)")
	fs.IntVar(&g.directions, "directions", 1, "direction strategy: 1 | 4 | 8")
	fs.IntVar(&g.frameCount, "frame-count", 0, "filmstrip frame count (default 4)")
	fs.IntVar(&g.maxAttempts, "max-attempts", 0, "max total attempts per direction (default 3)")
	if err := fs.Parse(args); err != nil {
		return nil, "", err
	}
	if fs.NArg() != 1 {
		return nil, "", fmt.Errorf("usage: oframe generation %s <identity-package-path> [flags]", name)
	}
	return g, fs.Arg(0), nil
}

func cmdGenerationPlan(args []string, jsonOut bool, stdout io.Writer) error {
	g, pkgPath, err := parseGenerationFlags("plan", args)
	if err != nil {
		return err
	}
	svc, err := newCLIService(g.settingsDir)
	if err != nil {
		return err
	}
	defer func() { _ = svc.Close() }()
	plan, err := svc.PrepareGeneration(context.Background(), service.GenerationRequest{
		PackagePath:             pkgPath,
		ProviderID:              g.providerID,
		Model:                   g.model,
		MotionID:                g.motionID,
		Directions:              g.directions,
		StylePresetID:           g.style,
		ActionPresetID:          g.action,
		FrameCount:              g.frameCount,
		MaxAttemptsPerDirection: g.maxAttempts,
	})
	if err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "generation plan %s\n", plan.ID)
	fmt.Fprintf(&b, "  provider/model : %s / %s\n", plan.ProviderID, plan.Model)
	fmt.Fprintf(&b, "  directions     : %d (%d 生成 + %d 镜像)\n", plan.Directions, plan.BasicDirections, plan.MirroredDirections)
	fmt.Fprintf(&b, "  预计调用量      : %d (每方向最多 %d 次总尝试, 预算上限 %d 次)\n", plan.ExpectedCalls, plan.MaxAttemptsPerDirection, plan.MaxTotalAttempts)
	fmt.Fprintf(&b, "  预算           : ~%s %.2f (上限 %s %.2f)\n", plan.Currency, plan.ExpectedCost, plan.Currency, plan.MaxCost)
	fmt.Fprintf(&b, "  外发素材       : %d 张参考图\n", len(plan.OutboundMaterials))
	for _, m := range plan.OutboundMaterials {
		fmt.Fprintf(&b, "    - [%s] %s (%s)\n", m.Role, m.Name, m.Path)
	}
	fmt.Fprintf(&b, "  提示词快照     :\n    %s\n", plan.Prompt.Prompt)
	fmt.Fprintln(&b, "提示: 使用 `oframe generation run ... --yes` 确认后执行")
	return emit(stdout, jsonOut, map[string]any{"ok": true, "plan": plan}, b.String())
}

func cmdGenerationRun(args []string, jsonOut bool, stdout io.Writer) error {
	yes := false
	rest := []string{}
	for _, a := range args {
		if a == "--yes" {
			yes = true
			continue
		}
		rest = append(rest, a)
	}
	g, pkgPath, err := parseGenerationFlags("run", rest)
	if err != nil {
		return err
	}
	svc, err := newCLIService(g.settingsDir)
	if err != nil {
		return err
	}
	defer func() { _ = svc.Close() }()
	plan, err := svc.PrepareGeneration(context.Background(), service.GenerationRequest{
		PackagePath: pkgPath, ProviderID: g.providerID, Model: g.model,
		MotionID: g.motionID, Directions: g.directions, StylePresetID: g.style,
		ActionPresetID: g.action, FrameCount: g.frameCount,
		MaxAttemptsPerDirection: g.maxAttempts,
	})
	if err != nil {
		return err
	}
	if !yes {
		// Confirmation gate: without --yes the plan is shown and NO external
		// call is made (杜绝静默超支).
		var b strings.Builder
		fmt.Fprintf(&b, "generation NOT confirmed — 未发起任何外部调用.\n")
		fmt.Fprintf(&b, "plan %s: %d directions (%d 生成 + %d 镜像), %d 预计调用量, 每方向最多 %d 次总尝试, 预算上限 %d 次.\n",
			plan.ID, plan.Directions, plan.BasicDirections, plan.MirroredDirections,
			plan.ExpectedCalls, plan.MaxAttemptsPerDirection, plan.MaxTotalAttempts)
		fmt.Fprintln(&b, "re-run with --yes to confirm and execute.")
		return emit(stdout, jsonOut, map[string]any{"ok": false, "confirmed": false, "plan": plan}, b.String())
	}

	res, err := svc.ConfirmGeneration(context.Background(), plan.ID, true)
	if err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "generation confirmed and executed (plan %s)\n", plan.ID)
	fmt.Fprintf(&b, "  status   : %s\n", res.Status)
	fmt.Fprintf(&b, "  calls    : %d (attempts %d)\n", res.CallsMade, res.Attempts)
	for _, r := range res.Results {
		fmt.Fprintf(&b, "    - %-10s attempts=%d bytes=%d model=%s\n", r.Direction, r.Attempts, r.Bytes, r.Model)
	}
	if res.Error != "" {
		fmt.Fprintf(&b, "  error    : %s\n", res.Error)
	}
	return emit(stdout, jsonOut, map[string]any{"ok": res.Status != "failed", "result": res}, b.String())
}
