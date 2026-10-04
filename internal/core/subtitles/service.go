// SPDX-License-Identifier: GPL-3.0-or-later

package subtitles

import (
	"context"
	"time"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
)

// Nodes is what subtitles asks of the node channel: nodes.Service.
type Nodes interface {
	library.NodeConnections
	IsConnected(ctx context.Context, nodeID int64) bool
}

// Queue is the job queue: jobs.Queue.
type Queue interface {
	Enqueue(ctx context.Context, job jobs.Request) (bool, error)
	// KeepWanted says a person still waits for a job's work: a player asking for a text being made
	// (D234 §3).
	KeepWanted(ctx context.Context, kind jobs.Kind, target jobs.Target) error
}

// Parts are what the subtitles service is built from.
type Parts struct {
	Database *store.Database
	Queue    Queue
	Nodes    Nodes
	// Exchanges are how a subtitle's making asks its node and takes its answer; the service records
	// its results with them.
	Exchanges *nodes.Exchanges
	// Subtitler fetches subtitles on request (D224); nil when the server has none.
	Subtitler Subtitler
	Clock     clock.Clock
	// MaxSilence is how long a fetch waits for its node to convert what it fetched.
	MaxSilence time.Duration
}

// Service offers a played video's text subtitles, has their text made by the nodes and kept, serves
// it, and fetches a subtitle on request (D217 §6, D224).
type Service struct {
	database   *store.Database
	queue      Queue
	nodes      Nodes
	exchanges  *nodes.Exchanges
	subtitler  Subtitler
	clock      clock.Clock
	maxSilence time.Duration
	// keptTexts tells a fetch waiting for its conversion that the text was kept.
	keptTexts *keptTexts
}

// NewService returns the subtitles service, recording the nodes' texts with exchanges.
func NewService(parts Parts) *Service {
	service := &Service{
		database: parts.Database, queue: parts.Queue, nodes: parts.Nodes, exchanges: parts.Exchanges,
		subtitler: parts.Subtitler, clock: parts.Clock, maxSilence: parts.MaxSilence, keptTexts: newKeptTexts(),
	}
	parts.Exchanges.Record(nodeproto.CommandExtractSubtitle, service.record)
	return service
}
