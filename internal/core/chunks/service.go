// SPDX-License-Identifier: AGPL-3.0-or-later

package chunks

import (
	"context"
	"log/slog"
	"time"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/settings"
	"github.com/kstenerud/orpheum/internal/platform/clock"
	"github.com/kstenerud/orpheum/internal/store"
)

// Settings are the settings the chunks read: the segment length a grid is merged to, the
// qualities a slow link is offered, and the sound's bit rate.
type Settings interface {
	Duration(ctx context.Context, key settings.Key) (time.Duration, error)
	String(ctx context.Context, key settings.Key) (string, error)
	Int(ctx context.Context, key settings.Key) (int64, error)
}

// Queue is the job queue: jobs.Queue.
type Queue interface {
	Enqueue(ctx context.Context, job jobs.Request) (bool, error)
	SetProgress(ctx context.Context, id int64, fraction float64) error
	// KeepWanted says a person still waits for a job's work: a player asking for a store's chunks
	// while a run makes them (D234 §3).
	KeepWanted(ctx context.Context, kind jobs.Kind, target jobs.Target) error
	// ReleaseWanted says no person waits for a job's work any more: a player that left (D258 §2).
	ReleaseWanted(ctx context.Context, kind jobs.Kind, target jobs.Target) error
	// CancelMatching cancels kind's queued and running jobs on target whose arguments match says: a
	// run a seek went far past (D250 §5).
	CancelMatching(
		ctx context.Context, kind jobs.Kind, target jobs.Target, match func(argsJSON string) bool,
	) (int, error)
	// Cancel cancels a queued or running job: a run evicted to keep its node's others faster than
	// playback (D250 §6).
	Cancel(ctx context.Context, id int64) error
}

// Loudness says the gains that level each measured sound of a file's bytes: loudness.Service.
type Loudness interface {
	Levels(ctx context.Context, payloadHash []byte) (decide.Levels, error)
}

// ReadThrough has a film read through at play priority, for its picture's timeline, which its grid
// is cut from: readthrough.Service.
type ReadThrough interface {
	ReadForPlay(ctx context.Context, fileID int64) (bool, error)
	// ReleaseForPlay says no player waits for the file's read-through any more (D258 §2).
	ReleaseForPlay(ctx context.Context, fileID int64) error
	RetryFailedForPlay(
		ctx context.Context, fileID int64, payloadHash []byte, videoStreamIndex int, before time.Time,
	) (bool, error)
	// HasNoIndex is whether a read-through found the file's bytes have no index of the picture's
	// keyframes, so its film is cut on time until it is read through (D269 §2).
	HasNoIndex(payloadHash []byte, videoStreamIndex int) bool
	// ReadFirst queues a read-through of the file ahead of the sweep, for the keyframes its index
	// lacks (D269 §4).
	ReadFirst(ctx context.Context, fileID int64) (bool, error)
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
	// Loudness says the gain a sound's chunks carry (D226 §4).
	Loudness Loudness
	// ReadThrough reads a film without a grid through for a player waiting on it (D247 §2).
	ReadThrough ReadThrough
	// Logger says each run queued, and why.
	Logger *slog.Logger
	// Clock times each run's speed (D250 §6).
	Clock clock.Clock
	// Encoders say whether a film's node encodes on a GPU, which chooses its ladder (D323 §3), and
	// whether it encodes too slowly to offer any quality it encodes (D457); nil encodes every film in
	// software, fast enough.
	Encoders Encoders
	// Absences are the places a read found a film's file not at, which a run reads from last (D363
	// §8); nil knows of none.
	Absences *library.Absences
}

// HoldBack stops the work on a node no one plays, and gives it none until a time, reporting how
// many it stopped: the Exchanges', which a test may stand in for.
type HoldBack interface {
	HoldBackFor(ctx context.Context, nodeID int64, until time.Time) int
}

// Encoders say whether a node encodes on a GPU, and how fast it encodes the lowest quality, in
// percent of real time, zero for unmeasured (D457): nodes.Service.
type Encoders interface {
	EncodesOnAGPU(nodeID int64) bool
	EncodeSpeedPercent(nodeID int64) int
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
	logger      *slog.Logger
	clock       clock.Clock
	encoders    Encoders
	absences    *library.Absences
	// holdBack stops a node's other work while a run players want, encoded in software, is behind
	// playback (D324 §5): the exchanges; nil holds nothing back.
	holdBack HoldBack
	// runs are the chunks known made and the runs making more, by store: in memory, since a node's
	// answer for a chunk is the truth and a restart only asks it again.
	runs *runTracker
	// movePlayers moves a run's players down a quality before the run is evicted; nil moves none
	// (MovePlayersBy).
	movePlayers MovePlayers
	// interests are what the players of each film want, which decide the runs that go on (D309); nil
	// wants every run (InterestsBy).
	interests Interests
}

// MovePlayers moves the players wanting a film's quality, by its stream's name, onto the quality
// below it, and says whether every one moved (D286 §4, D309 §6).
type MovePlayers func(fileID int64, streamName string) bool

// MovePlayersBy is how a run players want is spared eviction: its players moved down a quality
// (D309 §6). Set once, before the server serves.
func (service *Service) MovePlayersBy(move MovePlayers) { service.movePlayers = move }

// NewService returns the chunks service.
func NewService(parts Parts) *Service {
	service := &Service{
		database: parts.Database, cache: parts.Cache, settings: parts.Settings, queue: parts.Queue, nodes: parts.Nodes,
		exchanges: parts.Exchanges, loudness: parts.Loudness, readThrough: parts.ReadThrough, logger: parts.Logger,
		clock: parts.Clock, encoders: parts.Encoders, absences: parts.Absences, runs: newRunTracker(),
	}
	if parts.Exchanges != nil {
		service.holdBack = parts.Exchanges
	}
	return service
}
