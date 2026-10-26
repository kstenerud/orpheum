// SPDX-License-Identifier: AGPL-3.0-or-later

package slogx_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

// Every line logged with a request's context carries its ID, through a logger's own attributes and
// groups; a line logged with no request's context carries none (D347).
func TestNew_ALineLoggedWithARequestsContextCarriesItsID(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	logger, err := slogx.New(&output, slogx.FormatJSON, slog.LevelInfo)
	if err != nil {
		t.Fatal(err)
	}
	ctx := slogx.WithRequestID(t.Context(), "R7Q2")
	logger.InfoContext(ctx, "asked")
	logger.With("component", "api").WarnContext(ctx, "refused")
	logger.WithGroup("scan").InfoContext(ctx, "grouped")
	logger.InfoContext(t.Context(), "no request")
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want 4:\n%s", len(lines), output.String())
	}
	for index, line := range lines {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		id := record[slogx.RequestIDKey]
		if group, isGroup := record["scan"].(map[string]any); isGroup {
			id = group[slogx.RequestIDKey]
		}
		if want := index < 3; (id == "R7Q2") != want {
			t.Errorf("line %d, %s: requestId = %v; want it carried %v", index, line, id, want)
		}
	}
	if got := slogx.RequestID(ctx); got != "R7Q2" {
		t.Errorf("RequestID = %q", got)
	}
}

// A logger built at a *slog.LevelVar logs at whatever level it holds now (D347).
func TestNew_ALevelVarChangesTheLevelWhileItRuns(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	level := &slog.LevelVar{}
	logger, err := slogx.New(&output, slogx.FormatText, level)
	if err != nil {
		t.Fatal(err)
	}
	logger.Debug("hidden")
	level.Set(slog.LevelDebug)
	logger.Debug("shown")
	if text := output.String(); strings.Contains(text, "hidden") || !strings.Contains(text, "shown") {
		t.Errorf("the log is\n%s\nwant only the line logged after the level fell to debug", text)
	}
}
