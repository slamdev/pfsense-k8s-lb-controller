package testdata

import (
	"io"
	"log/slog"
	"testing"

	"github.com/neilotoole/slogt"
)

func SetTestLogger(t *testing.T) {
	f := slogt.Factory(func(w io.Writer) slog.Handler {
		opts := &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}
		return slog.NewTextHandler(w, opts)
	})
	slog.SetDefault(slogt.New(t, f))
}
