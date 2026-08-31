package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/oframe/character-workbench/core/identity"
	"github.com/oframe/character-workbench/core/pipeline"
	"github.com/oframe/character-workbench/core/provider"
)

// describeImagePromptTemplate is the vision-model instruction for AI image
// captioning (识图生成描述): look at the character sprite and produce an
// accurate Chinese description usable as the prompt basis for later motion
// generation. The output is a suggestion: the frontend fills it into the
// description textarea for user review; nothing is saved automatically.
func describeImagePromptTemplate() string {
	return "请仔细观察这张像素游戏角色图，写一段准确的中文描述，作为后续像素动作生成的提示词基础。\n" +
		"要求：\n" +
		"1. 只描述图中可见内容：体型比例、头部/躯干/四肢特征、服装与配色、显著标志；不要编造看不见的细节。\n" +
		"2. 风格如实描述（例如像素风、描边、明暗对比）。\n" +
		"3. 输出一段纯文本，80-200 字；不要标题、列表、markdown 或任何解释。"
}

// DescribeBaseCharacterImage asks the configured prompt-enhancement text model
// (vision-capable) to look at one base-character candidate image and return a
// description prompt. Provider/model resolution mirrors EnhanceDescription
// (settings → 增强模型 → active provider text model). One billed text call;
// if the model has no vision support the provider error is surfaced so the
// user can pick a vision-capable model in settings.
func (s *Service) DescribeBaseCharacterImage(ctx context.Context, pkgPath, candidateID string) (string, error) {
	pkg, err := identity.Open(pkgPath)
	if err != nil {
		return "", err
	}
	var target identity.BaseCharacterCandidate
	found := false
	for _, c := range pkg.BaseCharacterCandidates() {
		if c.ID == candidateID {
			target = c
			found = true
			break
		}
	}
	if !found {
		return "", fmt.Errorf("service: 候选不存在或已删除 —— 请重新导入角色图")
	}
	raw, err := os.ReadFile(filepath.Join(pkg.Root(), filepath.FromSlash(target.ImagePath)))
	if err != nil {
		return "", fmt.Errorf("service: 读取候选图失败: %w", err)
	}
	img, err := pipeline.DecodeImageAny(raw)
	if err != nil {
		return "", fmt.Errorf("service: 解码候选图失败: %w", err)
	}
	pngBytes, err := pipeline.EncodeFilmstripPNG(img)
	if err != nil {
		return "", err
	}
	dataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes)

	ps := s.settings.ProviderSettings()
	if len(ps.Providers) == 0 {
		return "", fmt.Errorf("service: 尚未配置任何 Provider —— 请先在设置中添加支持文本的 Provider")
	}
	assoc := s.EnhanceSettingsGet()
	providerID := assoc.ProviderID
	if providerID == "" {
		providerID = ps.ActiveProvider
	}
	if providerID == "" {
		providerID = provider.DefaultProviderID
	}
	prov, err := s.registry.Get(providerID)
	if err != nil {
		return "", err
	}
	cfg := ps.ConfigFor(providerID)
	if !prov.Capabilities().Has(provider.ModalityText) {
		return "", fmt.Errorf("service: 当前 Provider（%s）不支持文本模型，无法识图", providerID)
	}
	model, err := provider.ResolveValidatedModel(prov.Capabilities(), cfg, provider.ModalityText, assoc.Model)
	if err != nil {
		return "", fmt.Errorf("service: %w", err)
	}
	res, err := prov.GenerateText(ctx, provider.TextRequest{
		Prompt:       describeImagePromptTemplate(),
		Model:        model,
		ImageDataURL: dataURI,
	})
	if err != nil {
		if ctx.Err() != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("service: 识图调用超时（90 秒）—— 可在设置中更换更快的文本模型后重试")
		}
		return "", fmt.Errorf("service: 识图调用失败: %w", err)
	}
	out := strings.TrimSpace(res.Text)
	if out == "" {
		return "", fmt.Errorf("service: 识图返回为空")
	}
	s.log.Info("base character image described", "candidate", candidateID, "provider", providerID, "model", model)
	return out, nil
}
