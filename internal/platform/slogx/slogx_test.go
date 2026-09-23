// SPDX-License-Identifier: GPL-3.0-or-later

package slogx_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/platform/slogx"
)

func TestNew_JSONWritesOneObjectPerRecord(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	logger, err := slogx.New(&output, slogx.FormatJSON, slog.LevelInfo)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	logger.Info("scan finished", "fileCount", 3)
	logger.Debug("below the level")

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1 (the debug record is below the level):\n%s", len(lines), output.String())
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatalf("the record is not JSON: %v\n%s", err, lines[0])
	}
	if record["msg"] != "scan finished" {
		t.Errorf("msg = %v, want %q", record["msg"], "scan finished")
	}
}

func TestNew_TextFormat(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	logger, err := slogx.New(&output, slogx.FormatText, slog.LevelInfo)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	logger.Info("started")
	if !strings.Contains(output.String(), "msg=started") {
		t.Errorf("text output = %q, want a msg=started field", output.String())
	}
}

func TestParseFormat(t *testing.T) {
	t.Parallel()
	cases := []struct {
		value     string
		want      slogx.Format
		wantError bool
	}{
		{value: "json", want: slogx.FormatJSON},
		{value: "text", want: slogx.FormatText},
		{value: "yaml", wantError: true},
		{value: "", wantError: true},
	}
	for _, tc := range cases {
		got, err := slogx.ParseFormat(tc.value)
		if tc.wantError {
			if err == nil {
				t.Errorf("ParseFormat(%q) = %q, want an error", tc.value, got)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("ParseFormat(%q) = %q, %v; want %q, nil", tc.value, got, err, tc.want)
		}
	}
}

func TestNew_RejectsAnUnknownFormat(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	if _, err := slogx.New(&output, slogx.Format("yaml"), slog.LevelInfo); err == nil {
		t.Error("New accepted the format \"yaml\"; want an error")
	}
}
