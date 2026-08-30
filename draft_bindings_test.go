// Binding-level acceptance of unsaved-draft persistence (workbench-ui spec):
// drafts survive restarts via the package sidecar, never leak into the
// manifest, and clear cleanly after the real fields are saved.
package main

import "testing"

func strPtr(s string) *string { return &s }
func intPtr(n int) *int       { return &n }

func TestDraftBindings(t *testing.T) {
	app, _ := newTestAppSimple(t)
	if _, err := app.PackageCreate("Hero", ""); err != nil {
		t.Fatal(err)
	}

	// Fresh package: empty draft view.
	d, err := app.DraftGet()
	if err != nil || d == nil || d.Description != "" || d.MotionName != "" {
		t.Fatalf("fresh draft = %+v err=%v", d, err)
	}

	// Put → Get roundtrip, with MERGE semantics: the second put touches only
	// the motion fields and must not clobber the description.
	mirror := true
	if err := app.DraftPut(DraftInput{Description: strPtr("草稿描述")}); err != nil {
		t.Fatal(err)
	}
	if err := app.DraftPut(DraftInput{MotionName: strPtr("walk"), MotionCount: intPtr(4), MotionMirror: &mirror}); err != nil {
		t.Fatal(err)
	}
	d, err = app.DraftGet()
	if err != nil || d.Description != "草稿描述" || d.MotionName != "walk" || d.MotionCount != 4 || d.MotionMirror == nil || !*d.MotionMirror {
		t.Fatalf("draft roundtrip = %+v err=%v", d, err)
	}

	// The draft must never leak into the manifest.
	view, err := app.IdentityGet()
	if err != nil {
		t.Fatal(err)
	}
	if view.Description != "" {
		t.Fatalf("draft leaked into manifest description: %q", view.Description)
	}

	// Real save → clear → empty again.
	if err := app.IdentitySetDescription("正式描述"); err != nil {
		t.Fatal(err)
	}
	if err := app.DraftClear(); err != nil {
		t.Fatal(err)
	}
	d, err = app.DraftGet()
	if err != nil || d == nil || d.Description != "" {
		t.Fatalf("post-clear draft = %+v err=%v", d, err)
	}
}
