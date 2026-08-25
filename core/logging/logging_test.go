package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestNewTextOutput(t *testing.T) {
	var buf bytes.Buffer
	l := New(Options{Level: slog.LevelInfo, Output: &buf})
	l.Info("hello", "k", "v")
	out := buf.String()
	if !strings.Contains(out, "hello") {
		t.Fatalf("text output missing message: %q", out)
	}
}

func TestNewJSONOutput(t *testing.T) {
	var buf bytes.Buffer
	l := New(Options{Level: slog.LevelInfo, JSON: true, Output: &buf})
	l.Info("hello", "k", "v")
	out := buf.String()
	if !strings.HasPrefix(out, "{") || !strings.Contains(out, `"msg":"hello"`) {
		t.Fatalf("json output malformed: %q", out)
	}
}

func TestLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	l := New(Options{Level: slog.LevelWarn, JSON: true, Output: &buf})
	l.Info("dropped")
	l.Warn("kept")
	out := buf.String()
	if strings.Contains(out, "dropped") {
		t.Fatalf("info below warn level should be dropped: %q", out)
	}
	if !strings.Contains(out, "kept") {
		t.Fatalf("warn at warn level should be kept: %q", out)
	}
}

func TestParseLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"":        slog.LevelInfo,
		"debug":   slog.LevelDebug,
		"INFO":    slog.LevelInfo,
		"Warn":    slog.LevelWarn,
		"error":   slog.LevelError,
		"  info ": slog.LevelInfo,
	}
	for in, want := range cases {
		got, err := ParseLevel(in)
		if err != nil {
			t.Fatalf("ParseLevel(%q) unexpected error: %v", in, err)
		}
		if got != want {
			t.Errorf("ParseLevel(%q) = %v, want %v", in, got, want)
		}
	}
	if _, err := ParseLevel("bogus"); err == nil {
		t.Fatal("ParseLevel(bogus) should error")
	}
}
