// Phase-7 typed bindings for the lightweight edit capability: EditDirection
// applies replayable edit instructions to a motion direction's current
// animation assets (editing spec 7.1–7.5) through the shared application
// service (core/service.EditDirection) — the same code path the CLI uses.
package main

import (
	"encoding/base64"
	"image/color"

	"github.com/oframe/character-workbench/core/edit"
)

// EditInstructionView mirrors edit.Instruction with plain fields so the Wails
// model stays simple (color.RGBA is carried as separate R/G/B/A channels).
type EditInstructionView struct {
	Kind       string            `json:"kind"`
	FrameIndex int               `json:"frameIndex,omitempty"`
	X          int               `json:"x,omitempty"`
	Y          int               `json:"y,omitempty"`
	Width      int               `json:"width,omitempty"`
	Height     int               `json:"height,omitempty"`
	R          uint8             `json:"r,omitempty"`
	G          uint8             `json:"g,omitempty"`
	B          uint8             `json:"b,omitempty"`
	A          uint8             `json:"a,omitempty"`
	DurationMs int               `json:"durationMs,omitempty"`
	DeltaX     int               `json:"deltaX,omitempty"`
	DeltaY     int               `json:"deltaY,omitempty"`
	Order      []int             `json:"order,omitempty"`
	FramePNG   string            `json:"framePng,omitempty"` // base64 (insert)
	FrameMeta  EditFrameMetaView `json:"frameMeta,omitempty"`
}

// EditFrameMetaView mirrors edit.FrameMeta.
type EditFrameMetaView struct {
	DurationMs int `json:"durationMs"`
	AnchorX    int `json:"anchorX"`
	AnchorY    int `json:"anchorY"`
}

// EditResultView mirrors service.EditResult.
type EditResultView struct {
	MotionID    string `json:"motionId"`
	Direction   string `json:"direction"`
	FrameCount  int    `json:"frameCount"`
	DurationsMs []int  `json:"durationsMs"`
	LogSeq      int    `json:"logSeq"`
}

// EditDirection applies replayable edit instructions (frame/sequence/anchor/
// batch) to a motion direction's accepted animation assets. The instructions
// are logged append-only; edits write back to the current version's assets and
// the motion metadata (durations/anchors/order) is updated (tasks 7.1–7.5).
func (a *App) EditDirection(motionID, direction string, instructions []EditInstructionView) (*EditResultView, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	pkg, err := a.requirePackage()
	if err != nil {
		return nil, err
	}
	insts := make([]edit.Instruction, 0, len(instructions))
	for _, v := range instructions {
		insts = append(insts, toEditInstruction(v))
	}
	res, err := svc.EditDirection(pkg.Root(), motionID, direction, insts)
	if err != nil {
		return nil, err
	}
	return &EditResultView{
		MotionID: res.MotionID, Direction: res.Direction, FrameCount: res.FrameCount,
		DurationsMs: res.DurationsMs, LogSeq: res.LogSeq,
	}, nil
}

// toEditInstruction converts a view instruction into the core edit instruction.
func toEditInstruction(v EditInstructionView) edit.Instruction {
	inst := edit.Instruction{
		Kind:       v.Kind,
		FrameIndex: v.FrameIndex,
		X:          v.X,
		Y:          v.Y,
		Width:      v.Width,
		Height:     v.Height,
		Color:      color.RGBA{R: v.R, G: v.G, B: v.B, A: v.A},
		DurationMs: v.DurationMs,
		DeltaX:     v.DeltaX,
		DeltaY:     v.DeltaY,
		Order:      append([]int(nil), v.Order...),
		FrameMeta:  edit.FrameMeta{DurationMs: v.FrameMeta.DurationMs, AnchorX: v.FrameMeta.AnchorX, AnchorY: v.FrameMeta.AnchorY},
	}
	if v.FramePNG != "" {
		if data, err := base64.StdEncoding.DecodeString(v.FramePNG); err == nil {
			inst.FramePNG = data
		}
	}
	return inst
}
