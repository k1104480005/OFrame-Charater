package service

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/oframe/character-workbench/core/motion"
	"github.com/oframe/character-workbench/core/provider"
)

// TestMotionBatchSummaryCounts verifies the 批量操作 summary (批量生成前置统计):
// 待生成格子按 生成调用 / 镜像派生 / 卡住的镜像槽 分类，费用按动作解析的单价
// 按币种聚合；生成完成后该动作从汇总中消失。全程 offline（不建 plan、不调用）。
func TestMotionBatchSummaryCounts(t *testing.T) {
	svc, root := newPhase5Svc(t)
	ctx := context.Background()

	single, err := svc.MotionCreate(root, "idle", motion.DirectionStrategy{Count: 1, Mirror: true})
	if err != nil {
		t.Fatal(err)
	}
	eight, err := svc.MotionCreate(root, "walk", motion.DirectionStrategy{Count: 8, Mirror: true})
	if err != nil {
		t.Fatal(err)
	}

	// 勾选全部方向（模拟每张卡全点亮）：统计口径 = 勾选方向 ∩ 未生成。
	allChecked := func(ids ...string) []MotionBatchSelection {
		out := make([]MotionBatchSelection, 0, len(ids))
		for _, id := range ids {
			m, err := svc.MotionGet(root, id)
			if err != nil {
				t.Fatal(err)
			}
			dirs := make([]string, 0, len(m.Directions))
			for _, d := range m.Directions {
				dirs = append(dirs, d.Direction)
			}
			out = append(out, MotionBatchSelection{MotionID: id, Directions: dirs})
		}
		return out
	}

	sum, err := svc.MotionBatchSummary(root, allChecked(single.ID, eight.ID))
	if err != nil {
		t.Fatal(err)
	}
	if sum.Motions != 2 || sum.PendingCalls != 6 || sum.PendingCells != 9 || len(sum.Items) != 2 {
		t.Fatalf("summary = %+v, want motions=2 calls=6 cells=9 items=2", sum)
	}
	itemByID := map[string]MotionBatchItem{}
	for _, it := range sum.Items {
		itemByID[it.MotionID] = it
	}
	if it := itemByID[single.ID]; it.Calls != 1 || len(it.BasicDirs) != 1 || it.BasicDirs[0] != motion.DirectionDown || len(it.MirrorDirs) != 0 {
		t.Fatalf("single item = %+v, want 1 call on down", it)
	}
	if it := itemByID[eight.ID]; it.Calls != 5 || len(it.MirrorDirs) != 3 || len(it.StuckDirs) != 0 {
		t.Fatalf("eight item = %+v, want 5 calls + 3 mirror derivations", it)
	}
	// 费用：全部动作解析到同一 Provider（Doubao），按币种聚合为一行。
	cfg, err := svc.ProviderConfig(provider.ProviderDoubao)
	if err != nil {
		t.Fatal(err)
	}
	if len(sum.Costs) != 1 || sum.Costs[0].Currency != provider.Currency(provider.ProviderDoubao) {
		t.Fatalf("costs = %+v, want one %s row", sum.Costs, provider.Currency(provider.ProviderDoubao))
	}
	if want := round2(6 * cfg.EffectivePrice()); sum.Costs[0].Amount != want {
		t.Fatalf("cost amount = %v, want %v", sum.Costs[0].Amount, want)
	}

	// 勾选子集：只勾 walk 的 right/up/down —— 批量统计只覆盖勾选的格子。
	sub, err := svc.MotionBatchSummary(root, []MotionBatchSelection{
		{MotionID: eight.ID, Directions: []string{motion.DirectionRight, motion.DirectionUp, motion.DirectionDown}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sub.PendingCalls != 3 || sub.PendingCells != 3 || len(sub.Items) != 1 || len(sub.Items[0].BasicDirs) != 3 {
		t.Fatalf("subset summary = %+v, want 3 calls on right/up/down", sub)
	}
	// 回归守卫：汇总 JSON 不得出现 null（Go nil 切片序列化为 null，会让前端
	// 对数组取 .length 时崩溃、动作页整体空白）。空切片必须是 []。
	data, err := json.Marshal(sum)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("null")) {
		t.Fatalf("summary JSON contains null: %s", data)
	}

	// 执行 single 的 down 之后：它离开汇总，总数同步下降。
	plan, err := svc.PrepareGeneration(ctx, GenerationRequest{
		PackagePath: root, MotionID: single.ID, StylePresetID: "pixel", ActionPresetID: "walk",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ConfirmGeneration(ctx, plan.ID, true); err != nil {
		t.Fatal(err)
	}
	sum2, err := svc.MotionBatchSummary(root, allChecked(single.ID, eight.ID))
	if err != nil {
		t.Fatal(err)
	}
	if sum2.Motions != 2 || sum2.PendingCalls != 5 || sum2.PendingCells != 8 || len(sum2.Items) != 1 {
		t.Fatalf("summary after generation = %+v, want calls=5 cells=8 items=1", sum2)
	}

	// 卡住的镜像槽：镜像关闭时先生成 right（left 不派生），再开启镜像 ——
	// left 源方向已有帧，无法再派生，计入 StuckDirs 且不产生调用。
	stuck8, err := svc.MotionCreate(root, "wave", motion.DirectionStrategy{Count: 8, Mirror: false})
	if err != nil {
		t.Fatal(err)
	}
	sp, err := svc.PrepareGeneration(ctx, GenerationRequest{
		PackagePath: root, MotionID: stuck8.ID, StylePresetID: "pixel",
		ActionPresetID: "walk", GenerateDirections: []string{motion.DirectionRight},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ConfirmGeneration(ctx, sp.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.MotionSetStrategy(root, stuck8.ID, motion.DirectionStrategy{Count: 8, Mirror: true}); err != nil {
		t.Fatal(err)
	}
	sum3, err := svc.MotionBatchSummary(root, allChecked(single.ID, eight.ID, stuck8.ID))
	if err != nil {
		t.Fatal(err)
	}
	var stuckItem *MotionBatchItem
	for i := range sum3.Items {
		if sum3.Items[i].MotionID == stuck8.ID {
			stuckItem = &sum3.Items[i]
		}
	}
	if stuckItem == nil {
		t.Fatalf("stuck motion missing from summary: %+v", sum3)
	}
	if stuckItem.Calls != 4 || len(stuckItem.MirrorDirs) != 2 || len(stuckItem.StuckDirs) != 1 || stuckItem.StuckDirs[0] != motion.DirectionLeft {
		t.Fatalf("stuck item = %+v, want 4 calls + 2 mirror + stuck [left]", stuckItem)
	}
}
