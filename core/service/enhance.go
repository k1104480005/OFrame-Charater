package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/oframe/character-workbench/core/provider"
)

// enhancePromptTemplate renders the text-model instruction for expanding a
// short character description into a pixel-art-ready Chinese description.
// The output is a suggestion: callers must show it to the user for review.
func enhancePromptTemplate(description string) string {
	return "你是像素游戏角色设定专家。请把下面的角色简述扩写成一段可直接用于像素角色生成的中文描述。\n" +
		"要求：\n" +
		"1. 保留原意，不引入与原文冲突的新设定；可补充合理的可视化细节（体型比例、配色、服装层次、标志特征）。\n" +
		"2. 输出一段纯文本，80-200 字；不要标题、列表、markdown 或任何解释。\n" +
		"3. 描述具体、可视化，适合直接作为图像生成提示词的一部分。\n\n" +
		"角色简述：" + strings.TrimSpace(description)
}

// EnhanceDescription expands a short character description with the
// prompt-enhancement association (settings → 增强模型): an explicitly
// configured enhance provider/model when set, otherwise the active provider's
// text model. One billed text call; the capability gate mirrors image
// generation before any network activity.
func (s *Service) EnhanceDescription(ctx context.Context, description string) (string, error) {
	if strings.TrimSpace(description) == "" {
		return "", fmt.Errorf("service: 描述为空，无法增强")
	}
	ps := s.settings.ProviderSettings()
	if len(ps.Providers) == 0 {
		return "", fmt.Errorf("service: 尚未配置任何 Provider —— 请先在设置中添加支持文本的 Provider")
	}
	assoc := s.EnhanceSettingsGet()
	providerID := assoc.ProviderID
	if providerID == "" {
		providerID = ps.ActiveProvider
		if providerID == "" {
			providerID = provider.DefaultProviderID
		}
	}
	prov, err := s.registry.Get(providerID)
	if err != nil {
		return "", err
	}
	cfg := ps.ConfigFor(providerID)
	if !prov.Capabilities().Has(provider.ModalityText) {
		return "", fmt.Errorf("service: 当前 Provider（%s）不支持文本模型，无法增强描述", providerID)
	}
	model, err := provider.ResolveValidatedModel(prov.Capabilities(), cfg, provider.ModalityText, assoc.Model)
	if err != nil {
		return "", fmt.Errorf("service: %w", err)
	}
	res, err := prov.GenerateText(ctx, provider.TextRequest{Prompt: enhancePromptTemplate(description), Model: model})
	if err != nil {
		if ctx.Err() != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("service: 文本模型调用超时（90 秒）—— 可在设置中更换更快的文本模型后重试")
		}
		return "", fmt.Errorf("service: 描述增强调用失败: %w", err)
	}
	out := strings.TrimSpace(res.Text)
	if out == "" {
		return "", fmt.Errorf("service: 描述增强返回为空")
	}
	return out, nil
}
