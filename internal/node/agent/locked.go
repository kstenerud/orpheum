// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/slogx"
	"github.com/kstenerud/orpheum/internal/platform/unixsocket"
)

// WaitForLeave is a locked node's loop (D450 §1): it reads no media and runs no FFmpeg, and posts
// report to the core at once and then each tick, until the core answers that an admin lets it run
// without the confinement it lacks. It is true then, and false when ctx ends first.
func WaitForLeave(
	ctx context.Context, socketPath string, report nodeproto.LockedReport, tick <-chan time.Time,
	logger *slog.Logger,
) bool {
	report.ProtocolVersion = nodeproto.ProtocolVersion
	locked := &Agent{client: unixsocket.Client(socketPath, 0)}
	for {
		reply, err := locked.postLocked(ctx, report)
		switch {
		case err == nil && reply.MayRunUnconfined:
			logger.WarnContext(ctx, "an admin lets this node run without the confinement its kernel lacks; "+
				"it starts its work", slogx.EventKey, "unconfinedLeaveTaken")
			return true
		case err != nil && ctx.Err() == nil:
			logger.WarnContext(ctx, "could not tell the core this node is locked; trying again at the next tick",
				"err", err)
		}
		select {
		case <-ctx.Done():
			return false
		case <-tick:
		}
	}
}

// postLocked posts a locked node's report and reads the core's answer.
func (agent *Agent) postLocked(ctx context.Context, report nodeproto.LockedReport) (nodeproto.LockedReply, error) {
	encoded, err := json.Marshal(report)
	if err != nil {
		return nodeproto.LockedReply{}, fmt.Errorf("encode the locked report: %w", err)
	}
	var reply nodeproto.LockedReply
	err = agent.exchange(ctx, http.MethodPost, nodeproto.PathLocked, 0, bytes.NewReader(encoded), &reply)
	return reply, err
}
