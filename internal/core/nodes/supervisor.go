// SPDX-License-Identifier: GPL-3.0-or-later

package nodes

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/clock"
)

// Process is a running node child. The core cannot start one — it never reaches os/exec
// (TestArch_CoreNeverExecs) — so cmd/orpheum hands it a Starter built on execx (D207).
type Process interface {
	Done() <-chan struct{}
	Err() error
	Kill() error
}

// Starter starts the node child.
type Starter func(ctx context.Context) (Process, error)

// Liveness says when a node was last heard from; Service is the real one.
type Liveness interface {
	LastHeard(name string) (time.Time, bool)
}

// Supervisor keeps the co-located node running (cut-a §2, D43, D44): it starts the child, says
// when it has gone silent (node_silent_after_s), restarts it when silence lasts
// (node_restart_after_silent_s), and starts it again after node_restart_after_exit_s when it exits, so a
// crashing node cannot spin. It never exits because the node did: it reports.
type Supervisor struct {
	startNode Starter
	liveness  Liveness
	name      string
	clock     clock.Clock
	settings  Settings
	logger    *slog.Logger
}

// NewSupervisor supervises the node called name.
func NewSupervisor(
	start Starter, liveness Liveness, name string, clockSource clock.Clock, settingsService Settings,
	logger *slog.Logger,
) *Supervisor {
	return &Supervisor{
		startNode: start, liveness: liveness, name: name, clock: clockSource, settings: settingsService, logger: logger,
	}
}

// Run supervises until ctx ends, judging the node at each tick, and kills the child on the way
// out.
func (supervisor *Supervisor) Run(ctx context.Context, tick <-chan time.Time) error {
	state := &supervision{}
	defer func() {
		if state.child != nil {
			if err := state.child.Kill(); err != nil {
				supervisor.logger.WarnContext(context.WithoutCancel(ctx), "could not kill the node", "err", err)
			}
		}
	}()
	for {
		if err := supervisor.judge(ctx, state); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-tick:
		case <-doneOf(state.child): // judged at the top of the loop
		}
	}
}

// supervision is what the supervisor knows about its child between passes.
type supervision struct {
	child           Process
	startedAt       time.Time
	exitedAt        time.Time
	hasWarnedSilent bool
}

// judge is one pass: note an exit, start a child that is due, and act on silence.
func (supervisor *Supervisor) judge(ctx context.Context, state *supervision) error {
	now := supervisor.clock.Now()
	if state.child != nil && hasExited(state.child) {
		supervisor.logger.WarnContext(ctx, "the node exited; it will be started again", "node", supervisor.name,
			"err", state.child.Err())
		state.child, state.exitedAt = nil, now
	}
	limits, err := supervisor.readLivenessLimits(ctx)
	if err != nil {
		return err
	}
	if state.child == nil {
		if state.exitedAt.IsZero() || now.Sub(state.exitedAt) >= limits.restartAfterExit {
			supervisor.startChild(ctx, state, now)
		}
		return nil
	}
	silence := now.Sub(supervisor.lastHeard(state.startedAt))
	switch {
	case silence >= limits.restartAfterSilence:
		supervisor.logger.WarnContext(ctx, "the node has been silent too long; restarting it",
			"node", supervisor.name, "silentFor", silence.String())
		if err := state.child.Kill(); err != nil {
			return fmt.Errorf("kill the silent node: %w", err)
		}
	case silence >= limits.silentAfter && !state.hasWarnedSilent:
		supervisor.logger.WarnContext(ctx, "the node is silent", "node", supervisor.name, "silentFor", silence.String())
		state.hasWarnedSilent = true
	}
	return nil
}

func (supervisor *Supervisor) startChild(ctx context.Context, state *supervision, now time.Time) {
	child, err := supervisor.startNode(ctx)
	if err != nil {
		supervisor.logger.ErrorContext(ctx, "could not start the node; trying again", "err", err)
		state.exitedAt = now
		return
	}
	state.child, state.startedAt, state.hasWarnedSilent = child, now, false
	supervisor.logger.InfoContext(ctx, "started the node", "node", supervisor.name)
}

// lastHeard is when the node last spoke, or when it was started if it has not spoken since.
func (supervisor *Supervisor) lastHeard(startedAt time.Time) time.Time {
	heard, hasSpoken := supervisor.liveness.LastHeard(supervisor.name)
	if !hasSpoken || heard.Before(startedAt) {
		return startedAt
	}
	return heard
}

type livenessLimits struct {
	silentAfter, restartAfterSilence, restartAfterExit time.Duration
}

func (supervisor *Supervisor) readLivenessLimits(ctx context.Context) (livenessLimits, error) {
	var seconds [3]int64
	var errs []error
	keys := []settings.Key{settings.NodeSilentAfterS, settings.NodeRestartAfterSilentS, settings.NodeRestartAfterExitS}
	for index, key := range keys {
		value, err := supervisor.settings.Int(ctx, key)
		seconds[index] = value
		errs = append(errs, err)
	}
	if err := errors.Join(errs...); err != nil {
		return livenessLimits{}, fmt.Errorf("read the node's liveness settings: %w", err)
	}
	return livenessLimits{
		silentAfter:         time.Duration(seconds[0]) * time.Second,
		restartAfterSilence: time.Duration(seconds[1]) * time.Second,
		restartAfterExit:    time.Duration(seconds[2]) * time.Second,
	}, nil
}

// doneOf is the child's done channel, or nil — which never fires — when there is no child.
func doneOf(child Process) <-chan struct{} {
	if child == nil {
		return nil
	}
	return child.Done()
}

// hasExited reports, without waiting, whether the child has exited.
func hasExited(child Process) bool {
	select {
	case <-child.Done():
		return true
	default:
		return false
	}
}
