package identity

import (
	"encoding/json"
	"fmt"
	"time"
)

// FormatVersion is the current identity package manifest format version
// (design D3: manifest carries a format version; migrators handle evolution).
const FormatVersion = 1

// FileName is the manifest file name inside an identity package directory.
const FileName = "manifest.json"

// Directory layout inside an identity package (task 1.4: 资产/候选/日志引用).
const (
	DirMaterials  = "materials"  // material area: reference images / existing sprites
	DirCandidates = "candidates" // candidate history index (later phase)
	DirLog        = "log"        // append-only operation log (later phase)
	DirVersions   = "versions"   // per-identity-version asset areas
)

// Material kinds stored in the material area (task 2.3 entry points).
const (
	MaterialKindReferenceImage = "reference_image" // 参考图
	MaterialKindSprite         = "sprite"          // 既有精灵
)

// MaterialRole is the semantic role of a material inside the identity
// definition (1 主参考图 + 最多 2 辅助参考图 semantics).
type MaterialRole = string

// Material roles express the reference-image semantics (阶段 3: 1 主参考图 +
// 最多 2 辅助参考图). A reference image is either the single main reference
// (主参考图) or one of at most two auxiliary references (辅助参考图); a sprite
// material always carries the sprite role.
const (
	RoleMainReference      = "main_reference"      // 主参考图（最多 1 张）
	RoleAuxiliaryReference = "auxiliary_reference" // 辅助参考图（最多 2 张）
	RoleSprite             = "sprite"              // 既有精灵
)

// MaxMainReferences and MaxAuxiliaryReferences bound the reference-image roles
// (1 主参考图 + 最多 2 辅助参考图).
const (
	MaxMainReferences      = 1
	MaxAuxiliaryReferences = 2
)

// Identity definition entry kinds (task 2.3).
const (
	EntryKindText           = "text"            // 文字描述
	EntryKindReferenceImage = "reference_image" // 参考图
	EntryKindSprite         = "sprite"          // 既有精灵
)

// InitialVersionID is the identity version created with a new package.
const InitialVersionID = "v1"

// IdentityMeta is the identity definition metadata carried by the manifest
// (task 1.4: 身份元数据; task 2.3: 文字描述/素材入口写入元数据).
type IdentityMeta struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Category        string    `json:"category,omitempty"`        // 主页分类管理（空=未分类）
	Description     string    `json:"description,omitempty"`     // 文字描述入口
	EntryKind       string    `json:"entryKind,omitempty"`       // text | reference_image | sprite
	EntryMaterialID string    `json:"entryMaterialId,omitempty"` // 素材区引用（参考图/精灵入口）
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// CoordinateRange is a closed integer coordinate range [XMin,XMax]×[YMin,YMax].
// Anchors and the logical canvas carry one so coordinates are always bounded
// (task 2.5: 锚点及其坐标范围写入 manifest).
type CoordinateRange struct {
	XMin int `json:"xMin"`
	YMin int `json:"yMin"`
	XMax int `json:"xMax"`
	YMax int `json:"yMax"`
}

// Contains reports whether the point (x, y) lies inside the range.
func (r CoordinateRange) Contains(x, y int) bool {
	return x >= r.XMin && x <= r.XMax && y >= r.YMin && y <= r.YMax
}

// Width returns the inclusive width of the range.
func (r CoordinateRange) Width() int { return r.XMax - r.XMin + 1 }

// Height returns the inclusive height of the range.
func (r CoordinateRange) Height() int { return r.YMax - r.YMin + 1 }

// CanvasSpec is the logical canvas (逻辑画布): the single unit size and
// coordinate range shared by all motions and direction sets (task 2.4).
type CanvasSpec struct {
	UnitWidth       int             `json:"unitWidth"`
	UnitHeight      int             `json:"unitHeight"`
	CoordinateRange CoordinateRange `json:"coordinateRange"`
}

// DefaultCanvasRange derives the coordinate range for a unit size.
func DefaultCanvasRange(w, h int) CoordinateRange {
	return CoordinateRange{XMin: 0, YMin: 0, XMax: w - 1, YMax: h - 1}
}

// NewCanvasSpec validates and creates a canvas specification.
func NewCanvasSpec(w, h int) (*CanvasSpec, error) {
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("identity: canvas unit size must be positive, got %dx%d", w, h)
	}
	return &CanvasSpec{UnitWidth: w, UnitHeight: h, CoordinateRange: DefaultCanvasRange(w, h)}, nil
}

// Validate checks internal consistency of the specification.
func (c *CanvasSpec) Validate() error {
	if c == nil {
		return fmt.Errorf("identity: canvas is nil")
	}
	if c.UnitWidth <= 0 || c.UnitHeight <= 0 {
		return fmt.Errorf("identity: canvas unit size must be positive, got %dx%d", c.UnitWidth, c.UnitHeight)
	}
	if r := c.CoordinateRange; r.XMin != 0 || r.YMin != 0 || r.XMax != c.UnitWidth-1 || r.YMax != c.UnitHeight-1 {
		return fmt.Errorf("identity: canvas coordinate range %v does not match unit size %dx%d", r, c.UnitWidth, c.UnitHeight)
	}
	return nil
}

// ValidateFrame reports whether a frame of the given pixel size conforms to
// the logical canvas. This is the conformance check that motion/frame-sequence
// validation references (task 2.4: 规格被后续动作/帧序列校验引用).
func (c *CanvasSpec) ValidateFrame(w, h int) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if w != c.UnitWidth || h != c.UnitHeight {
		return fmt.Errorf("identity: frame size %dx%d does not conform to logical canvas %dx%d", w, h, c.UnitWidth, c.UnitHeight)
	}
	return nil
}

// Contains reports whether the point lies inside the canvas coordinate range.
func (c *CanvasSpec) Contains(x, y int) bool { return c.CoordinateRange.Contains(x, y) }

// Anchor is an identity-level anchor definition (脚底/手持点等) with its
// coordinate range, reusable by motions and direction sets by ID (task 2.5).
type Anchor struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Preset          string          `json:"preset,omitempty"`
	X               int             `json:"x"`
	Y               int             `json:"y"`
	CoordinateRange CoordinateRange `json:"coordinateRange"`
}

// Material is a reference to a file stored in the package material area
// (task 2.3 素材区 / 素材引用). Path is relative to the package root and uses
// forward slashes for portability. Role carries the reference semantics
// (main_reference | auxiliary_reference | sprite); older packages may leave it
// empty, in which case role-based validation treats the material as unassigned.
type Material struct {
	ID      string    `json:"id"`
	Kind    string    `json:"kind"` // reference_image | sprite
	Role    string    `json:"role,omitempty"`
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	AddedAt time.Time `json:"addedAt"`
}

// References carries references to derived content inside the package
// (task 1.4: 资产/候选/日志引用). Candidate history and operation log files are
// created by later phases; the manifest declares their locations from the start.
type References struct {
	// CandidateHistory is the relative path to the candidate history index.
	CandidateHistory string `json:"candidateHistory,omitempty"`
	// OperationLog is the relative path to the append-only operation log.
	OperationLog string `json:"operationLog,omitempty"`
}

// DefaultReferences returns the conventional derived-content layout.
func DefaultReferences() References {
	return References{
		CandidateHistory: DirCandidates + "/index.json",
		OperationLog:     DirLog + "/operations.jsonl",
	}
}

// VersionRecord is an immutable identity version (身份版本, task 9.1): formed
// after an explicit appearance revision; older versions stay preserved but no
// longer represent the current identity by default. AssetsRef points at that
// version's animation asset area (资产引用).
type VersionRecord struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	Reason    string    `json:"reason,omitempty"`
	Immutable bool      `json:"immutable"`
	AssetsRef string    `json:"assetsRef,omitempty"`
}

// Versions stores the identity versions of a package; Current is the ID of the
// version representing the current identity by default.
type Versions struct {
	Current string          `json:"current,omitempty"`
	Items   []VersionRecord `json:"items"`
}

// Manifest is the identity package manifest (task 1.4): format version,
// identity metadata, logical canvas, anchors, asset references, and references
// to candidate history and operation logs.
type Manifest struct {
	FormatVersion int          `json:"formatVersion"`
	Identity      IdentityMeta `json:"identity"`
	LogicalCanvas *CanvasSpec  `json:"logicalCanvas,omitempty"`
	Anchors       []Anchor     `json:"anchors,omitempty"`
	Materials     []Material   `json:"materials,omitempty"`
	References    References   `json:"references"`
	Versions      Versions     `json:"versions"`
}

// Encode serializes the manifest with stable field order and indentation.
func (m *Manifest) Encode() ([]byte, error) {
	return json.MarshalIndent(m, "", "  ")
}

// DecodeManifest parses manifest bytes.
func DecodeManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("identity: cannot parse manifest: %w", err)
	}
	return &m, nil
}
