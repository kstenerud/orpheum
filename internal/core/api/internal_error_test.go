// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/core/nodes"
)

// A read from a node whose client went away first is no silent node, and is kept quiet; a node that
// did not answer is a 503 node_silent, logged with why.
func TestNodeReadError_TellsAClientThatWentAwayFromANodeThatDidNotAnswer(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	server := NewServer(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	gone, cancel := context.WithCancel(t.Context())
	cancel()
	err := server.nodeReadError(gone, errors.Join(nodes.ErrNodeSilent, context.Canceled))
	var apiErr *Error
	if errors.As(err, &apiErr) && apiErr.ErrorBody.Error.Code == CodeNodeSilent {
		t.Errorf("a read whose client went away = %v; want no node_silent", err)
	}
	if strings.Contains(logs.String(), "level=ERROR") || strings.Contains(logs.String(), "level=WARN") {
		t.Errorf("a read whose client went away was logged loudly:\n%s", logs.String())
	}
	err = server.nodeReadError(t.Context(), errors.Join(nodes.ErrNodeSilent, errors.New("connection refused")))
	if !errors.As(err, &apiErr) || apiErr.ErrorBody.Error.Code != CodeNodeSilent {
		t.Errorf("a node that did not answer = %v; want node_silent", err)
	}
	if !strings.Contains(logs.String(), "level=WARN") || !strings.Contains(logs.String(), "connection refused") {
		t.Errorf("a node that did not answer was not logged with why:\n%s", logs.String())
	}
}

// A request that fails is logged as an error, but not one whose client went away first, as a
// player does from a segment it no longer wants: nothing failed but the asking.
func TestLogInternalError_KeepsQuietWhenTheClientWentAway(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	server := NewServer(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	gone, cancel := context.WithCancel(t.Context())
	cancel()
	//nolint:errcheck // the log is what is asked
	_ = server.logInternalError(gone, "find a chunk's file", context.Canceled)
	if strings.Contains(logs.String(), "level=ERROR") {
		t.Errorf("a request whose client went away was logged as an error:\n%s", logs.String())
	}
	//nolint:errcheck // as above
	_ = server.logInternalError(t.Context(), "find a chunk's file", errors.New("the disk is gone"))
	if !strings.Contains(logs.String(), "level=ERROR") {
		t.Errorf("a request that failed was not logged as an error:\n%s", logs.String())
	}
}
