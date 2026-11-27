// SPDX-License-Identifier: AGPL-3.0-or-later

package nodes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// fakeSheets take the sheets posted, or refuse them with refusal.
type fakeSheets struct {
	refusal error
	taken   []nodeproto.TrickplaySheet
}

func (sheets *fakeSheets) DeliverTrickplaySheet(_ context.Context, _ int64, sheet nodeproto.TrickplaySheet) error {
	if sheets.refusal != nil {
		return sheets.refusal
	}
	sheets.taken = append(sheets.taken, sheet)
	return nil
}

// A sheet's answer is its node's acknowledgement: 200 once kept; 422 for a run no longer wanted, which
// ends the node's run (internal/node/agent's postSheet); any other failure refused, for the node to
// send it again.
func TestTrickplaySheet_IsAcknowledgedOnceKeptAndRejectedForARunNoLongerWanted(t *testing.T) {
	t.Parallel()
	harness := newFixture(t, time.Second)
	reply := harness.hello(t, nodeproto.Hello{Name: "local"})
	post := func() int {
		encoded, err := json.Marshal(nodeproto.TrickplaySheet{
			JobID: 4, AttemptToken: "a4", Index: 0, WebP: []byte("RIFF"),
		})
		if err != nil {
			t.Fatal(err)
		}
		return harness.post(t, nodeproto.PathTrickplaySheet, reply.NodeID, bytes.NewReader(encoded)).status
	}
	if status := post(); status != http.StatusServiceUnavailable {
		t.Errorf("a sheet to a core taking none: %d, want 503", status)
	}
	sheets := &fakeSheets{}
	harness.service.SetTrickplaySheets(sheets)
	sheets.refusal = errors.New("cache.db is busy")
	if status := post(); status != http.StatusInternalServerError {
		t.Errorf("a sheet that could not be kept: %d, want 500", status)
	}
	sheets.refusal = fmt.Errorf("sheet 0: %w", ErrAttemptNotHeld)
	if status := post(); status != nodeproto.StatusResultRejected {
		t.Errorf("a sheet of a run no longer wanted: %d, want %d", status, nodeproto.StatusResultRejected)
	}
	sheets.refusal = nil
	if status := post(); status != http.StatusOK || len(sheets.taken) != 1 || string(sheets.taken[0].WebP) != "RIFF" {
		t.Errorf("a sheet kept: %d, taken %+v; want 200 and the sheet", status, sheets.taken)
	}
}
