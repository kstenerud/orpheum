// SPDX-License-Identifier: AGPL-3.0-or-later

package byteserver_test

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/byteserver"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock/clocktest"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// A node's recent log is every line it kept, the debug ones its log does not write too, with each
// field's value of its kind (D524 §3).
func TestServeRecentLog_SaysEveryLineTheNodeKeptWithItsFieldsKinds(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	recent := slogx.NewRecent(slogx.ProcessRecentLimits())
	logger, err := slogx.NewKeeping(io.Discard, slogx.FormatText, slog.LevelInfo, recent)
	if err != nil {
		t.Fatal(err)
	}
	record := slog.NewRecord(start, slog.LevelDebug, "examined a file", 0)
	record.AddAttrs(slog.Int("fileId", 7), slog.Uint64("bytes", 1<<63), slog.Float64("ratio", 0.25),
		slog.Bool("isDone", true), slog.Duration("took", 3*time.Second), slog.Time("at", start),
		slog.String("codec", "hevc"), slog.Any("err", errors.ErrUnsupported))
	if err = logger.Handler().Handle(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	handler := byteserver.New(nil).ServingRecentLog(recent, clocktest.NewFake(start.Add(time.Minute))).Handler()

	recorder := request(t, handler, http.MethodGet, nodeproto.PathBytesRecentLog, nil)
	var answer nodeproto.RecentLog
	if recorder.Code != http.StatusOK {
		t.Fatalf("answered %d; want 200", recorder.Code)
	}
	if err = json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if len(answer.Lines) != 1 || answer.Lines[0].Message != "examined a file" ||
		answer.Lines[0].Level != int(slog.LevelDebug) {
		t.Fatalf("lines %+v; want the debug line", answer.Lines)
	}
	want := []nodeproto.LogField{
		{Key: "fileId", Kind: nodeproto.LogValueInt, Value: "7"},
		{Key: "bytes", Kind: nodeproto.LogValueUint, Value: "9223372036854775808"},
		{Key: "ratio", Kind: nodeproto.LogValueFloat, Value: "0.25"},
		{Key: "isDone", Kind: nodeproto.LogValueBool, Value: "true"},
		{Key: "took", Kind: nodeproto.LogValueDuration, Value: "3000000000"},
		{Key: "at", Kind: nodeproto.LogValueTime, Value: "2026-11-19T12:00:00Z"},
		{Key: "codec", Kind: nodeproto.LogValueString, Value: "hevc"},
		{Key: "err", Kind: nodeproto.LogValueString, Value: "unsupported operation"},
	}
	if !slices.Equal(answer.Lines[0].Fields, want) {
		t.Errorf("fields %+v; want %+v", answer.Lines[0].Fields, want)
	}
}

// A node not given its recent log answers 404, as a node of protocol 64 does, so the core says it
// absent.
func TestServeRecentLog_IsNotFoundOnANodeKeepingNone(t *testing.T) {
	t.Parallel()
	recorder := request(t, byteserver.New(nil).Handler(), http.MethodGet, nodeproto.PathBytesRecentLog, nil)
	if recorder.Code != http.StatusNotFound {
		t.Errorf("answered %d; want 404", recorder.Code)
	}
}
