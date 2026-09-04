package pipeline

import (
	"crypto/rand"
	"fmt"
	"image"
	"time"
)

// Candidate statuses. Phase 4 candidates exist before the acceptance gate
// (task 8.3); they are pending until accepted, and failed when the pipeline
// could not produce conforming frames.
const (
	CandidatePending = "pending"
	CandidateFailed  = "failed"
)

// Candidate is the full result of one filmstrip pipeline run: the preserved
// original filmstrip artifact (原始产物), the immutable prompt snapshot
// (提示词快照), the processed frames, and the quality scores. A candidate that
// fails still retains the original artifact and the recorded reason so the
// user is never left empty-handed (生成结果保留最佳候选而非空手返回).
type Candidate struct {
	ID             string
	CreatedAt      time.Time
	Prompt         PromptSnapshot
	Layout         FrameList
	FilmstripPNG   []byte // 原始产物 (original filmstrip artifact)
	Frames         []*image.RGBA
	AnchorSets     [][]AnchorPoint // per-frame corrected anchors (锚点清单)
	Direction      string          // motion direction this filmstrip belongs to
	Scores         QualityScores
	Status         string
	Reason         string
	RegenerationOf string // parent candidate id when this is a regeneration
}

// CandidateSet retains candidates and always keeps the best-scoring one
// (task 5.6: 生成结果保留最佳候选而非空手返回). Failed candidates are retained
// too, so Best() is non-nil once any candidate was added.
type CandidateSet struct {
	items []Candidate
	best  int
}

// NewCandidateSet creates an empty candidate set.
func NewCandidateSet() *CandidateSet {
	return &CandidateSet{best: -1}
}

// Add appends a candidate and keeps the best-scoring one (by Overall; ties
// keep the earlier candidate — deterministic).
func (cs *CandidateSet) Add(c Candidate) {
	cs.items = append(cs.items, c)
	if cs.best == -1 || c.Scores.Overall > cs.items[cs.best].Scores.Overall {
		cs.best = len(cs.items) - 1
	}
}

// Replace swaps a retained candidate's content in place (matched by ID); it is
// a no-op when the id is unknown. Used when a candidate's artifacts are
// rewritten on disk (e.g. horizontal flip) so the in-memory cache never serves
// stale pixels: scores are untouched by such rewrites, so best-tracking stays
// valid.
func (cs *CandidateSet) Replace(c Candidate) {
	for i := range cs.items {
		if cs.items[i].ID == c.ID {
			cs.items[i] = c
			return
		}
	}
}

// Best returns the best-scoring candidate, or nil when no candidate was
// added.
func (cs *CandidateSet) Best() *Candidate {
	if cs.best == -1 {
		return nil
	}
	return &cs.items[cs.best]
}

// All returns a copy of all retained candidates in insertion order.
func (cs *CandidateSet) All() []Candidate {
	return append([]Candidate(nil), cs.items...)
}

// Count returns the number of retained candidates.
func (cs *CandidateSet) Count() int {
	return len(cs.items)
}

// newCandidateID returns a random UUIDv4 for candidate identifiers.
func newCandidateID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("pipeline: crypto/rand failed: %v", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
