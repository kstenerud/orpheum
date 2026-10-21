// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"context"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// listWork is a listing held until its device has a reader free, which posts what it found to the
// core (D240 §2).
func (agent *Agent) listWork(ctx context.Context, nodeID int64, held attempt, command nodeproto.List) heldWork {
	return agent.answeringWork(ctx, nodeID, answeringJob{
		attempt: held, jobKind: "a listing", reading: command.RootPath,
		makeAnswer: func(jobCtx context.Context) (nodeproto.Result, string) {
			listing := agent.runList(jobCtx, command)
			return nodeproto.Result{Listing: &listing}, listing.Refusal
		},
		stalledAnswer: func(stall nodeproto.Stall) nodeproto.Result {
			return nodeproto.Result{Listing: &nodeproto.Listing{Stall: &stall}}
		},
	})
}

// runList lists under one of the node's roots. A root the node was not given is refused, whatever
// the core asks (D107); a listing stopped partway says so with its records, and never that it is
// complete.
func (agent *Agent) runList(ctx context.Context, command nodeproto.List) nodeproto.Listing {
	root, known := agent.roots[command.RootPath]
	if !known {
		return nodeproto.Listing{Refusal: "this node was not given the root " + command.RootPath}
	}
	records, err := agent.workers.Scanner.List(ctx, root, command)
	if err != nil && ctx.Err() == nil {
		agent.logger.WarnContext(ctx, "a listing ended early", "rootPath", root.Path(), "err", err)
	}
	return nodeproto.Listing{Records: records}
}

// examineWork is an examining held until its device has a reader free, which posts what it read to
// the core (D240 §4). Between one file and the next it waits while it is paused, as a run of FFmpeg
// is stopped, for play-driven work on its device (D240 §6).
func (agent *Agent) examineWork(
	ctx context.Context, nodeID int64, held attempt, command nodeproto.Examine,
) heldWork {
	return agent.answeringWork(ctx, nodeID, answeringJob{
		attempt: held, jobKind: "an examining", reading: command.RootPath,
		stalledAnswer: func(stall nodeproto.Stall) nodeproto.Result {
			return nodeproto.Result{Examined: &nodeproto.Examined{Stall: &stall}}
		},
		makeAnswer: func(jobCtx context.Context) (nodeproto.Result, string) {
			root, known := agent.roots[command.RootPath]
			if !known {
				refusal := "this node was not given the root " + command.RootPath
				return nodeproto.Result{Examined: &nodeproto.Examined{Refusal: refusal}}, refusal
			}
			between := func() error { return agent.work.waitWhilePaused(jobCtx, held.token) }
			records, err := agent.workers.Scanner.Examine(jobCtx, root, command.Files, between)
			if err != nil && jobCtx.Err() == nil {
				agent.logger.WarnContext(ctx, "an examining ended early", "rootPath", root.Path(), "err", err)
			}
			return nodeproto.Result{Examined: &nodeproto.Examined{Records: records}}, ""
		},
	})
}
