// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

// A request that fails is logged as an error, but not one whose client went away first, as a
// player does from a segment it no longer wants: nothing failed but the asking.
func TestLogInternalError_KeepsQuietWhenTheClientWentAway(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	server := NewServer(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	gone, cancel := context.WithCancel(t.Context())
	cancel()
	//nolint:errcheck // the log is what is asked
	_ = server.logInternalError(gone, "find a rendition's file", context.Canceled)
	if strings.Contains(logs.String(), "level=ERROR") {
		t.Errorf("a request whose client went away was logged as an error:\n%s", logs.String())
	}
	//nolint:errcheck // as above
	_ = server.logInternalError(t.Context(), "find a rendition's file", errors.New("the disk is gone"))
	if !strings.Contains(logs.String(), "level=ERROR") {
		t.Errorf("a request that failed was not logged as an error:\n%s", logs.String())
	}
}
