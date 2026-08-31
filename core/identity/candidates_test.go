package identity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBaseCharacterCandidateAdoption(t *testing.T) {
	root := filepath.Join(t.TempDir(), "hero.oframe")
	p, err := Create(root, "Hero")
	if err != nil {
		t.Fatal(err)
	}
	first, err := p.AddBaseCharacterCandidate("candidates/first.png", "doubao", "model-a", "first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := p.AddBaseCharacterCandidate("candidates/second.png", "doubao", "model-b", "second")
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AdoptBaseCharacter(first.ID); err != nil {
		t.Fatal(err)
	}
	if err := p.AdoptBaseCharacter(second.ID); err == nil {
		t.Fatal("adopting another candidate after the first selection must be refused")
	}
	opened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := opened.Identity().BaseCharacter; got != first.ID {
		t.Fatalf("current base character = %q, want %q", got, first.ID)
	}
	candidates := opened.BaseCharacterCandidates()
	if len(candidates) != 2 || candidates[0].Status != BaseCharacterAdopted || candidates[1].Status != BaseCharacterRejected {
		t.Fatalf("unexpected candidate statuses: %#v", candidates)
	}
}

func TestAdoptRejectsAllOtherPendingCandidates(t *testing.T) {
	root := filepath.Join(t.TempDir(), "hero.oframe")
	p, err := Create(root, "Hero")
	if err != nil {
		t.Fatal(err)
	}
	first, err := p.AddBaseCharacterCandidate("candidates/first.png", "doubao", "model-a", "first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := p.AddBaseCharacterCandidate("candidates/second.png", "doubao", "model-b", "second")
	if err != nil {
		t.Fatal(err)
	}
	third, err := p.AddBaseCharacterCandidate("candidates/third.png", "doubao", "model-c", "third")
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AdoptBaseCharacter(second.ID); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range p.BaseCharacterCandidates() {
		want := BaseCharacterRejected
		if candidate.ID == second.ID {
			want = BaseCharacterAdopted
		}
		if candidate.Status != want {
			t.Fatalf("candidate %s status = %q, want %q (selected=%s, other=%s)", candidate.ID, candidate.Status, want, second.ID, first.ID+","+third.ID)
		}
	}
	if err := p.AdoptBaseCharacter(first.ID); err == nil {
		t.Fatal("adopting another candidate after selection must be refused")
	}
}

func TestDeleteBaseCharacterCandidate(t *testing.T) {
	root := filepath.Join(t.TempDir(), "hero.oframe")
	p, err := Create(root, "Hero")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "candidates"), 0o755); err != nil {
		t.Fatal(err)
	}
	// pending 候选可删除：记录与图片文件一并移除。
	pendingRel := "candidates/pending.png"
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(pendingRel)), []byte("png-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	pending, err := p.AddBaseCharacterCandidate(pendingRel, "doubao", "model-a", "first")
	if err != nil {
		t.Fatal(err)
	}
	if err := p.DeleteBaseCharacterCandidate(pending.ID); err != nil {
		t.Fatal(err)
	}
	if candidates := p.BaseCharacterCandidates(); len(candidates) != 0 {
		t.Fatalf("candidates after delete = %#v, want empty", candidates)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(pendingRel))); !os.IsNotExist(err) {
		t.Fatalf("deleted image file should be gone, stat err = %v", err)
	}
	// 已采用基准不可删除，图片文件保留。
	adoptedRel := "candidates/adopted.png"
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(adoptedRel)), []byte("png-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	adopted, err := p.AddBaseCharacterCandidate(adoptedRel, "doubao", "model-b", "second")
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AdoptBaseCharacter(adopted.ID); err != nil {
		t.Fatal(err)
	}
	if err := p.DeleteBaseCharacterCandidate(adopted.ID); err == nil {
		t.Fatal("deleting the adopted basis must be refused")
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(adoptedRel))); err != nil {
		t.Fatalf("adopted image file must remain: %v", err)
	}
	if candidates := p.BaseCharacterCandidates(); len(candidates) != 1 {
		t.Fatalf("candidates = %#v, want the adopted one only", candidates)
	}
}

func TestCandidateCreatedAfterAdoptionIsRejected(t *testing.T) {
	root := filepath.Join(t.TempDir(), "hero.oframe")
	p, err := Create(root, "Hero")
	if err != nil {
		t.Fatal(err)
	}
	first, err := p.AddBaseCharacterCandidate("candidates/first.png", "doubao", "model-a", "first")
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AdoptBaseCharacter(first.ID); err != nil {
		t.Fatal(err)
	}
	second, err := p.AddBaseCharacterCandidate("candidates/second.png", "doubao", "model-b", "second")
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != BaseCharacterRejected {
		t.Fatalf("candidate created after adoption status = %q, want rejected", second.Status)
	}
	if err := p.AdoptBaseCharacter(second.ID); err == nil {
		t.Fatal("adopting a candidate created after the adoption must be refused")
	}
	if got := p.Identity().BaseCharacter; got != first.ID {
		t.Fatalf("base character = %q, want unchanged %q", got, first.ID)
	}
}

func TestOpenNormalizesStalePendingCandidates(t *testing.T) {
	root := filepath.Join(t.TempDir(), "hero.oframe")
	p, err := Create(root, "Hero")
	if err != nil {
		t.Fatal(err)
	}
	first, err := p.AddBaseCharacterCandidate("candidates/first.png", "doubao", "model-a", "first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := p.AddBaseCharacterCandidate("candidates/second.png", "doubao", "model-b", "second")
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AdoptBaseCharacter(first.ID); err != nil {
		t.Fatal(err)
	}
	// 模拟旧规则时期写入的包：采用后仍残留 pending 候选。
	if err := p.Update(func(m *Manifest) error {
		for i := range m.BaseCharacters {
			if m.BaseCharacters[i].ID == second.ID {
				m.BaseCharacters[i].Status = BaseCharacterPending
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range reopened.BaseCharacterCandidates() {
		want := BaseCharacterRejected
		if c.ID == first.ID {
			want = BaseCharacterAdopted
		}
		if c.Status != want {
			t.Fatalf("candidate %s status = %q, want %q after open normalization", c.ID, c.Status, want)
		}
	}
	// 归一化结果必须落盘：再次打开保持一致。
	again, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range again.BaseCharacterCandidates() {
		want := BaseCharacterRejected
		if c.ID == first.ID {
			want = BaseCharacterAdopted
		}
		if c.Status != want {
			t.Fatalf("candidate %s status = %q, want %q after reopen", c.ID, c.Status, want)
		}
	}
}

func TestAdoptRejectedCandidateRefused(t *testing.T) {
	root := filepath.Join(t.TempDir(), "hero.oframe")
	p, err := Create(root, "Hero")
	if err != nil {
		t.Fatal(err)
	}
	first, err := p.AddBaseCharacterCandidate("candidates/first.png", "doubao", "model-a", "first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := p.AddBaseCharacterCandidate("candidates/second.png", "doubao", "model-b", "second")
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AdoptBaseCharacter(first.ID); err != nil {
		t.Fatal(err)
	}
	if err := p.AdoptBaseCharacter(second.ID); err == nil {
		t.Fatal("adopting another candidate after the first selection must be refused")
	}
	// first remains the adopted basis and the second was automatically rejected.
	opened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := opened.Identity().BaseCharacter; got != first.ID {
		t.Fatalf("current base character = %q, want unchanged %q", got, first.ID)
	}
	candidates := opened.BaseCharacterCandidates()
	if len(candidates) != 2 || candidates[0].Status != BaseCharacterAdopted || candidates[1].Status != BaseCharacterRejected {
		t.Fatalf("unexpected candidate statuses: %#v", candidates)
	}
}

func TestRejectBaseCharacterCandidate(t *testing.T) {
	root := filepath.Join(t.TempDir(), "hero.oframe")
	p, err := Create(root, "Hero")
	if err != nil {
		t.Fatal(err)
	}
	first, err := p.AddBaseCharacterCandidate("candidates/first.png", "doubao", "model-a", "first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := p.AddBaseCharacterCandidate("candidates/second.png", "doubao", "model-b", "second")
	if err != nil {
		t.Fatal(err)
	}
	// 弃用 pending 候选后不可采用。
	if err := p.RejectBaseCharacter(first.ID); err != nil {
		t.Fatal(err)
	}
	// 重复弃用幂等。
	if err := p.RejectBaseCharacter(first.ID); err != nil {
		t.Fatalf("re-reject should be idempotent: %v", err)
	}
	if err := p.AdoptBaseCharacter(first.ID); err == nil {
		t.Fatal("adopting a rejected candidate must be refused")
	}
	// 采用另一候选成为基准后，已采用的基准不可弃用。
	if err := p.AdoptBaseCharacter(second.ID); err != nil {
		t.Fatal(err)
	}
	if err := p.RejectBaseCharacter(second.ID); err == nil {
		t.Fatal("rejecting the adopted basis must be refused")
	}
	// 未知 id 报错。
	if err := p.RejectBaseCharacter("nope"); err == nil {
		t.Fatal("rejecting an unknown candidate must fail")
	}
}

func TestLegacyManifestWithoutBaseCharactersOpens(t *testing.T) {
	root := filepath.Join(t.TempDir(), "legacy.oframe")
	p, err := Create(root, "Legacy")
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Update(func(m *Manifest) error {
		m.BaseCharacters = nil
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	opened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(opened.BaseCharacterCandidates()) != 0 || opened.Identity().BaseCharacter != "" {
		t.Fatal("legacy package unexpectedly gained a base-character selection")
	}
}
