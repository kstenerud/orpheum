// SPDX-License-Identifier: AGPL-3.0-or-later

package renditions

import (
	"context"
	"io"
	"log/slog"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
)

// Settings are the settings renditions reads.
type Settings interface {
	Int(ctx context.Context, key settings.Key) (int64, error)
}

// Nodes is what renditions asks of the node channel: nodes.Service.
type Nodes interface {
	library.NodeConnections
}

// Queue is the job queue: jobs.Queue.
type Queue interface {
	Enqueue(ctx context.Context, job jobs.Request) (bool, error)
	SetProgress(ctx context.Context, id int64, fraction float64) error
	// KeepWanted says a person still waits for a job's work: a player asking for a rendition being
	// prepared (D234 §3).
	KeepWanted(ctx context.Context, kind jobs.Kind, target jobs.Target) error
	// ReleaseWanted says no person waits for a job's work any more: a player that left (D258 §2).
	ReleaseWanted(ctx context.Context, kind jobs.Kind, target jobs.Target) error
}

// Loudness says the gains that level each measured sound of a file's bytes: loudness.Service.
type Loudness interface {
	Levels(ctx context.Context, payloadHash []byte) (decide.Levels, error)
}

// Parts are what the renditions service is built from.
type Parts struct {
	Database *store.Database
	Cache    *store.Cache
	Queue    Queue
	Nodes    Nodes
	// Exchanges are how a prepare asks its node and takes its answer; the service records its
	// results with them.
	Exchanges *nodes.Exchanges
	Settings  Settings
	// Loudness says the gains a making puts in its audio tracks (D226 §4).
	Loudness Loudness
	Clock    clock.Clock
	// Random makes the run ids of renditions.
	Random io.Reader
	Logger *slog.Logger
}

// Service has renditions made: it keeps their rows, queues their jobs, gives each job to the node
// that holds the source and records its reports (cut-a §6), and says where a rendition's files are.
type Service struct {
	database  *store.Database
	cache     *store.Cache
	queue     Queue
	nodes     Nodes
	exchanges *nodes.Exchanges
	settings  Settings
	loudness  Loudness
	clock     clock.Clock
	random    io.Reader
	logger    *slog.Logger
}

// NewService returns the renditions service, recording the nodes' prepares with exchanges.
func NewService(parts Parts) *Service {
	service := &Service{
		database: parts.Database, cache: parts.Cache, queue: parts.Queue, nodes: parts.Nodes,
		exchanges: parts.Exchanges, settings: parts.Settings, loudness: parts.Loudness, clock: parts.Clock,
		random: parts.Random, logger: parts.Logger,
	}
	parts.Exchanges.Record(nodeproto.CommandPrepare, service.recordPrepare)
	return service
}
