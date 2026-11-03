package nabu

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// TestTeeHandlerFansOut: records go to both sinks, each gated by its own level, so
// the console (Info) and PocketBase (here Warn) honour their configured levels.
func TestTeeHandlerFansOut(t *testing.T) {
	var console, pb bytes.Buffer
	h := teeHandler{
		console: slog.NewTextHandler(&console, &slog.HandlerOptions{Level: slog.LevelInfo}),
		pb:      slog.NewTextHandler(&pb, &slog.HandlerOptions{Level: slog.LevelWarn}),
	}
	lg := slog.New(h)
	lg.Info("hello world") // console yes, pb no (below its level)
	lg.Warn("danger")      // both

	if !strings.Contains(console.String(), "hello world") || !strings.Contains(console.String(), "danger") {
		t.Errorf("console missing records:\n%s", console.String())
	}
	if strings.Contains(pb.String(), "hello world") {
		t.Errorf("pb sink got a below-level (Info) record:\n%s", pb.String())
	}
	if !strings.Contains(pb.String(), "danger") {
		t.Errorf("pb sink missing the Warn record:\n%s", pb.String())
	}
}
