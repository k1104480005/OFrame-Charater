package identity

import (
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
	if err := p.AdoptBaseCharacter(second.ID); err != nil {
		t.Fatal(err)
	}
	opened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := opened.Identity().BaseCharacter; got != second.ID {
		t.Fatalf("current base character = %q, want %q", got, second.ID)
	}
	candidates := opened.BaseCharacterCandidates()
	if len(candidates) != 2 || candidates[0].Status != "rejected" || candidates[1].Status != "adopted" {
		t.Fatalf("unexpected candidate statuses: %#v", candidates)
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
	if err := p.AdoptBaseCharacter(second.ID); err != nil {
		t.Fatal(err)
	}
	// first was rejected by the second adoption; re-adopting it must fail so
	// the identity basis can never silently roll back.
	if err := p.AdoptBaseCharacter(first.ID); err == nil {
		t.Fatal("re-adopting a rejected candidate must be refused")
	}
	opened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := opened.Identity().BaseCharacter; got != second.ID {
		t.Fatalf("current base character = %q, want unchanged %q", got, second.ID)
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
