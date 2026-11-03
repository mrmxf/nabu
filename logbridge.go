package nabu

import (
	"context"
	"log/slog"

	"github.com/pocketbase/pocketbase/core"
)

// bridgeLogsToPocketBase tees the default slog handler into PocketBase's logger,
// so every slog.* call during serving is written BOTH to the pretty console and
// to the structured `_logs` table shown at /_/ Logs. Without this, operational
// logs (agent dispatch, s3 access, the job sweeper…) only reach the console and
// an operator can't find them in the admin UI. Called once on serve.
func bridgeLogsToPocketBase(app core.App) {
	slog.SetDefault(slog.New(teeHandler{
		console: slog.Default().Handler(),
		pb:      app.Logger().Handler(),
	}))
}

// teeHandler fans each record out to two handlers, each gated by its own level
// (so PocketBase's configured log level and the console level are both honoured).
type teeHandler struct{ console, pb slog.Handler }

func (t teeHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return t.console.Enabled(ctx, l) || t.pb.Enabled(ctx, l)
}

func (t teeHandler) Handle(ctx context.Context, r slog.Record) error {
	if t.console.Enabled(ctx, r.Level) {
		_ = t.console.Handle(ctx, r.Clone()) // best-effort console; clone so pb sees the original
	}
	if t.pb.Enabled(ctx, r.Level) {
		return t.pb.Handle(ctx, r)
	}
	return nil
}

func (t teeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return teeHandler{t.console.WithAttrs(attrs), t.pb.WithAttrs(attrs)}
}

func (t teeHandler) WithGroup(name string) slog.Handler {
	return teeHandler{t.console.WithGroup(name), t.pb.WithGroup(name)}
}
