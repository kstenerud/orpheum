// SPDX-License-Identifier: AGPL-3.0-or-later

package nodeproto

// PathProgress is where the node posts where an attempt it holds is: a Progress. Progress is best
// effort: the newest supersedes the rest, and one lost is not sent again (D234 §5). How the work
// ended goes to PathResult instead.
const PathProgress = "/node/v1/progress"

// ProgressState is where an attempt the node holds is.
type ProgressState string

// The states progress names.
const (
	// ProgressWaiting is an attempt waiting for a slot of its kind (cut-a §5.3).
	ProgressWaiting ProgressState = "waiting"
	// ProgressRunning is an attempt whose work runs.
	ProgressRunning ProgressState = "running"
	// ProgressPaused is an attempt whose run the node paused for play-driven work on its device
	// (D240 §6), said at each status tick, since a paused run says nothing of itself.
	ProgressPaused ProgressState = "paused"
)

// Progress is the node's word on where an attempt it holds is, said when it starts to wait or to
// run, at each status tick while it does, and, for a prepare, as FFmpeg moves on, at most every
// node.progress_every_seconds. The core hears from it that the node still holds the work.
type Progress struct {
	JobID        int64         `json:"jobId"`
	AttemptToken string        `json:"attemptToken"`
	State        ProgressState `json:"state"`
	// PreparedThroughMs is how much of a running prepare's source is prepared, in milliseconds of
	// its time; zero for other work.
	PreparedThroughMs int64 `json:"preparedThroughMs,omitempty"`
	// ChunksMadeThrough is the last chunk a running run of chunks has made, absent until it has
	// made one (CommandMakeChunks).
	ChunksMadeThrough *int `json:"chunksMadeThrough,omitempty"`
}
