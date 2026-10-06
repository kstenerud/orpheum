// SPDX-License-Identifier: GPL-3.0-or-later

package chunks

import (
	"context"
	"time"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/store"
)

// Settings are the settings the chunks read: the segment length a grid is merged to, and the
// qualities a slow link is offered.
type Settings interface {
	Duration(ctx context.Context, key settings.Key) (time.Duration, error)
	String(ctx context.Context, key settings.Key) (string, error)
}

// Queue is the job queue: jobs.Queue.
type Queue interface {
	Enqueue(ctx context.Context, job jobs.Request) (bool, error)
	SetProgress(ctx context.Context, id int64, fraction float64) error
	// KeepWanted says a person still waits for a job's work: a player asking for a store's chunks
	// while a run makes them (D234 §3).
	KeepWanted(ctx context.Context, kind jobs.Kind, target jobs.Target) error
}

// Loudness says the gains that level each measured sound of a file's bytes: loudness.Service.
type Loudness interface {
	Levels(ctx context.Context, payloadHash []byte) (decide.Levels, error)
}

// ReadThrough has a film read through at play priority, for its picture's timeline, which its grid
// is cut from: readthrough.Service.
type ReadThrough interface {
	ReadForPlay(ctx context.Context, fileID int64) (bool, error)
	RetryFailedForPlay(
		ctx context.Context, fileID int64, payloadHash []byte, videoStreamIndex int, before time.Time,
	) (bool, error)
}

// Parts are what the chunks service is built from.
type Parts struct {
	// Database holds the files and their pictures' timelines, which internal/core/readthrough keeps.
	Database *store.Database
	// Cache holds the chunk stores, which the runs of chunks target (D247 §10).
	Cache    *store.Cache
	Settings Settings
	Queue    Queue
	// Nodes say whether a film's node can read it now, for a run to park on (D231 §5).
	Nodes library.NodeConnections
	// Exchanges are how a run is given to its node and its word taken (D234 §7).
	Exchanges *nodes.Exchanges
	// Loudness says the gain a sound's chunks carry, as a prepare's (D226 §4).
	Loudness Loudness
	// ReadThrough reads a film without a grid through for a player waiting on it (D247 §2).
	ReadThrough ReadThrough
}

// Service cuts films into chunks made on request (D247 §10): the grid each is cut on, the
// playlists that list them, the runs that make them on their node, and which of a store's chunks are
// made or on their way.
type Service struct {
	database    *store.Database
	cache       *store.Cache
	settings    Settings
	queue       Queue
	nodes       library.NodeConnections
	exchanges   *nodes.Exchanges
	loudness    Loudness
	readThrough ReadThrough
	// runs are the chunks known made and the runs making more, by store: in memory, since a node's
	// answer for a chunk is the truth and a restart only asks it again.
	runs *runTracker
}

// NewService returns the chunks service.
func NewService(parts Parts) *Service {
	return &Service{
		database: parts.Database, cache: parts.Cache, settings: parts.Settings, queue: parts.Queue, nodes: parts.Nodes,
		exchanges: parts.Exchanges, loudness: parts.Loudness, readThrough: parts.ReadThrough,
		runs: newRunTracker(),
	}
}
