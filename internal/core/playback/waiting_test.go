// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// An answer no job waits for is dropped, and the log names the job by its id, as the records do
// (D227).
func TestDeliver_AnAnswerForNoWaitingJobIsLoggedByItsJobID(t *testing.T) {
	t.Parallel()
	var logged bytes.Buffer
	answers := newAnswerRouter[nodeproto.PrepareReport](slog.New(slog.NewJSONHandler(&logged, nil)), "a prepare report")
	answers.deliver(t.Context(), 1, 42, nodeproto.PrepareReport{})
	if !strings.Contains(logged.String(), `"jobId":42`) || !strings.Contains(logged.String(), `"nodeId":1`) {
		t.Errorf("the log = %s, want the job named as jobId and the node as nodeId", logged.String())
	}
}
