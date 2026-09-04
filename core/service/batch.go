package service

import (
	"sort"

	"github.com/oframe/character-workbench/core/identity"
	"github.com/oframe/character-workbench/core/motion"
	"github.com/oframe/character-workbench/core/provider"
)

// MotionBatchItem is the per-motion pending-generation summary of the 批量操作
// area: the still-empty directions split into billable basic dirs (AI calls),
// free mirror derivations, and stuck mirror slots (mirror on but the source
// already has frames — they need mirror off to be generated).
type MotionBatchItem struct {
	MotionID     string   `json:"motionId"`
	MotionName   string   `json:"motionName"`
	BasicDirs    []string `json:"basicDirs"`           // 需 AI 调用生成的空方向
	MirrorDirs   []string `json:"mirrorDirs"`          // 源方向生成后自动派生的空方向
	StuckDirs    []string `json:"stuckDirs,omitempty"` // 镜像开启但源方向已有帧（需关镜像才能生成）
	Calls        int      `json:"calls"`               // 预计 AI 调用次数 = len(BasicDirs)
	Currency     string   `json:"currency"`            // 该动作解析到的模型计价币种
	CostPerCall  float64  `json:"costPerCall"`         // 单次调用单价（动作级 Provider/模型解析）
	ExpectedCost float64  `json:"expectedCost"`        // 预计费用 = Calls × CostPerCall
	ProviderID   string   `json:"providerId,omitempty"` // 该动作解析到的图像 Provider（空 = 未解析到）
	Model        string   `json:"model,omitempty"`      // 解析到的模型（空 = Provider 默认）
}

// MotionBatchCost aggregates the estimated cost per currency (different
// motions may resolve to providers billing in different currencies).
type MotionBatchCost struct {
	Currency string  `json:"currency"`
	Amount   float64 `json:"amount"`
}

// MotionBatchSelection is one motion's checked directions (动作卡九宫格的点亮
// 集合) feeding the batch summary: 批量统计口径 = 每张卡勾选的方向 ∩ 未生成.
type MotionBatchSelection struct {
	MotionID   string   `json:"motionId"`
	Directions []string `json:"directions"`
}

// MotionBatchSummary is the 批量操作 area payload: totals across every motion
// of the open identity package. Computed OFFLINE — no plan is created and no
// external call is made.
type MotionBatchSummary struct {
	Motions      int               `json:"motions"`      // 动作卡总数
	PendingCells int               `json:"pendingCells"` // 勾选且未生成的格子总数
	PendingCalls int               `json:"pendingCalls"` // 预计 AI 调用总次数
	Costs        []MotionBatchCost `json:"costs"`        // 按币种聚合的预估费用
	Items        []MotionBatchItem `json:"items"`        // 勾选方向中仍有待生成的动作明细
}

// MotionBatchSummary computes the batch-generation summary of one identity
// package: for every motion, its CHECKED directions (动作卡九宫格勾选) that
// are still empty are classified exactly like the 点亮式 generation path
// (mirror slots are never billed directly; they are derived from their source
// when the source is generated in the same batch). Motions without checked
// directions contribute nothing. The per-motion Provider/模型/单价 resolution
// mirrors PrepareGeneration's fallback chain (动作级配置 → 激活 Provider →
// 内置默认); a motion whose provider cannot be resolved keeps its counts but
// reports zero cost.
func (s *Service) MotionBatchSummary(pkgPath string, selections []MotionBatchSelection) (*MotionBatchSummary, error) {
	pkg, err := identity.Open(pkgPath)
	if err != nil {
		return nil, err
	}
	if err := pkg.ValidateReferenceRoles(); err != nil {
		return nil, err
	}
	ms, err := motion.NewStore(pkg.Root()).Load()
	if err != nil {
		return nil, err
	}
	checked := make(map[string][]string, len(selections))
	for _, sel := range selections {
		if sel.MotionID == "" {
			continue
		}
		checked[sel.MotionID] = sel.Directions
	}
	summary := &MotionBatchSummary{Motions: ms.Len(), Items: []MotionBatchItem{}, Costs: []MotionBatchCost{}}
	costs := map[string]float64{}
	for _, m := range ms.List() {
		selDirs := checked[m.ID]
		if len(selDirs) == 0 {
			continue
		}
		// 注意：必须初始化为非 nil 空切片 —— nil 切片经 JSON 序列化成 null，
		// 前端 TS 模型按数组取 .length 会直接崩溃（界面空白）。
		basic := []string{}
		mirror := []string{}
		stuck := []string{}
		for _, dir := range selDirs {
			d := m.Direction(dir)
			if d == nil || len(d.Sequence.Frames) > 0 {
				// 只统计勾选中且未生成的格子；已生成/不存在的方向跳过。
				continue
			}
			if m.Strategy.Mirror && containsStr(motion.MirroredDirections(m.Strategy), dir) {
				// 镜像派生槽：仅当其源方向也在本批生成集合中（源为空）才可派生；
				// 源已有帧或缺失时本批无法覆盖（需关闭镜像后单独生成）。
				src := m.Direction(motion.MirrorSource(dir))
				if src != nil && len(src.Sequence.Frames) == 0 {
					mirror = append(mirror, dir)
				} else {
					stuck = append(stuck, dir)
				}
			} else {
				basic = append(basic, dir)
			}
		}
		if len(basic)+len(mirror)+len(stuck) == 0 {
			continue
		}
		item := MotionBatchItem{
			MotionID:   m.ID,
			MotionName: m.Name,
			BasicDirs:  basic,
			MirrorDirs: mirror,
			StuckDirs:  stuck,
			Calls:      len(basic),
		}
		if _, cfg, providerID, model, err := s.resolveImageProvider(pkg.Root(), m.ID, "", ""); err == nil {
			item.CostPerCall = cfg.EffectivePrice()
			item.Currency = providerCurrency(providerID)
			item.ExpectedCost = round2(float64(len(basic)) * item.CostPerCall)
			item.ProviderID = providerID
			item.Model = model
			if len(basic) > 0 {
				costs[item.Currency] += item.ExpectedCost
			}
		}
		summary.Items = append(summary.Items, item)
		summary.PendingCells += len(basic) + len(mirror) + len(stuck)
		summary.PendingCalls += len(basic)
	}
	for cur, amount := range costs {
		summary.Costs = append(summary.Costs, MotionBatchCost{Currency: cur, Amount: round2(amount)})
	}
	sort.Slice(summary.Costs, func(i, j int) bool { return summary.Costs[i].Currency < summary.Costs[j].Currency })
	return summary, nil
}

// providerCurrency resolves a provider id's billing currency (the currency
// table lives in the provider package); empty falls back to a neutral marker.
func providerCurrency(providerID string) string {
	if cur := provider.Currency(providerID); cur != "" {
		return cur
	}
	return "CREDIT"
}
