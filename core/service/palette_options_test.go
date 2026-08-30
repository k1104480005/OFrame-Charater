package service

import (
	"testing"

	"github.com/oframe/character-workbench/core/pipeline"
)

func TestProcessOptionsPaletteByStyle(t *testing.T) {
	cases := []struct {
		style string
		max   int
		skip  bool
	}{
		{style: "pixel", max: 32},
		{style: "retro16", max: 16},
		{style: "chibi", skip: true},
	}
	for _, tc := range cases {
		t.Run(tc.style, func(t *testing.T) {
			s := &Service{}
			opts := s.processOptions(&GenerationPlan{Prompt: pipeline.PromptSnapshot{StylePresetID: tc.style}})
			if opts.Palette.MaxColors != tc.max || opts.Palette.Skip != tc.skip {
				t.Fatalf("palette options = %+v, want MaxColors=%d Skip=%t", opts.Palette, tc.max, tc.skip)
			}
		})
	}
}
