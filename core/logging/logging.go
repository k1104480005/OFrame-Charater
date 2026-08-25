// Package logging provides structured logging for the workbench on top of
// log/slog: a logger factory with configurable level and JSON/text output.
// All core packages log through slog; the CLI and (later) the Wails shell
// configure the global default with logging.Setup.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// Options configures the application logger.
type Options struct {
	// Level is the minimum level to emit.
	Level slog.Level
	// JSON selects JSON output; otherwise human-readable text output.
	JSON bool
	// Output is the writer; defaults to os.Stderr (logs never pollute
	// stdout, which is reserved for machine-readable CLI output).
	Output io.Writer
}

// New creates a logger with the given options.
func New(opts Options) *slog.Logger {
	if opts.Output == nil {
		opts.Output = os.Stderr
	}
	var h slog.Handler
	ho := &slog.HandlerOptions{Level: opts.Level}
	if opts.JSON {
		h = slog.NewJSONHandler(opts.Output, ho)
	} else {
		h = slog.NewTextHandler(opts.Output, ho)
	}
	return slog.New(h)
}

// ParseLevel parses a level name ("debug", "info", "warn", "error") into a
// slog.Level. An empty string yields slog.LevelInfo.
func ParseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("logging: unknown level %q (want debug|info|warn|error)", s)
	}
}

// Setup builds a logger from opts and installs it as the slog default so every
// package that uses slog.Default() shares the same configuration. It returns
// the configured logger.
func Setup(opts Options) *slog.Logger {
	l := New(opts)
	slog.SetDefault(l)
	return l
}
