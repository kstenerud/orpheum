// SPDX-License-Identifier: GPL-3.0-or-later

package nodeproto

import (
	"time"

	"github.com/kstenerud/orpheum/internal/platform/build"
	"github.com/kstenerud/orpheum/internal/platform/host"
)

// The node channel's paths on the core's socket (cut-a §4). The node dials; the core answers.
const (
	PathHello    = "/node/v1/hello"
	PathReport   = "/node/v1/report"
	PathCommands = "/node/v1/commands"
)

// QueryAfterCommandSeq is the commands long-poll's parameter: the sequence number of the last command
// the node took, so the core answers with the ones after it.
const QueryAfterCommandSeq = "afterCommandSeq"

// ArtworkAnswerWait is how long the core waits for a node's answer to an artwork command, and so
// how long a node keeps trying to send one the core did not take: past it, the core has tried the
// job again and a later answer has nothing to go to. Two minutes is far past an image's encode,
// which takes seconds.
const ArtworkAnswerWait = 2 * time.Minute

// ProtocolVersion is the version of the node protocol this build speaks: its paths, commands,
// records and results. It is raised with any change a node or core of another build would misread,
// so the two refuse each other at hello instead (D233 §9). Core and node ship as one build today,
// so the two match unless a node is updated apart from its core.
const ProtocolVersion = 3

// Hello is the first thing a node says, and it says it again after every reconnection.
type Hello struct {
	// ProtocolVersion is the version of this protocol the node speaks (ProtocolVersion); a core that
	// speaks another refuses the hello, and the node waits for an update (D233 §9).
	ProtocolVersion int `json:"protocolVersion"`
	// Name is the node's configured name; the core enrols a node it has not seen by it (D207).
	Name string `json:"name"`
	// StartID names this start of the node, random, the same in every hello until it starts again: a
	// node that restarted has lost the work it held in memory, which the core then counts as
	// interrupted (D238 §3).
	StartID string `json:"startId"`
	// Build is what the node's binary was built from, so the core can see a mismatch (D225).
	Build build.Provenance `json:"build"`
	// Host is the node's operating system, as lsb_release shows it, and its kernel (D225).
	Host host.System `json:"host"`
	// RootPaths are the media roots configured on the node, never remotely (D107).
	RootPaths     []string `json:"rootPaths"`
	FFmpegVersion string   `json:"ffmpegVersion"`
	// FFmpegSha256 is the SHA-256 of the ffmpeg binary, so the core can tell which build ran.
	FFmpegSha256 string `json:"ffmpegSha256"`
	// Capabilities are what the node found it can do (cut-a §4): remux and aac always, and the
	// encoders it can use, h264_qsv, h264_nvenc and x264.
	Capabilities []string `json:"capabilities"`
	// Confinement is what the node got, per mechanism (D112).
	Confinement []Confinement `json:"confinement"`
	UID         int           `json:"uid"`
	GID         int           `json:"gid"`
	// Hardware is what the node found for hardware encoding (D225 §3).
	Hardware Hardware `json:"hardware"`
	// Capacity is how much work the node takes on at once, and how fast it reads, from its config.
	Capacity Capacity `json:"capacity"`
}

// Capacity is what a node's machine can do, as its config says: facts about the deployment, not
// the admin's opinions, so they are the node's config keys, not the core's settings (cut-a §5.3 as
// plan-deviations P36 records).
type Capacity struct {
	// EncodeSlotCount is how many encodes the node runs at once, which its processor or GPU bounds;
	// RemuxSlotCount is how many remuxes, which its disk bounds. The node holds a prepare beyond
	// its slots and says so (ProgressWaiting), and the core runs as many prepares at once as the
	// nodes' slots add up to.
	EncodeSlotCount int `json:"encodeSlotCount"`
	RemuxSlotCount  int `json:"remuxSlotCount"`
	// SlowestReadBytesPerSecond is the slowest the node expects to read a whole file: a busy
	// spinning disk, or a network mount. Its watchdog allows a run that reads one that long, and the
	// core waits that long for a loudness measurement.
	SlowestReadBytesPerSecond int64 `json:"slowestReadBytesPerSecond"`
}

// Hardware is what a node found for hardware encoding: the GPU devices it sees, whether it may
// open them, each hardware encoder's trial and the encoder it chose (D225 §3).
type Hardware struct {
	GPUs []GPU `json:"gpus"`
	// Trials are the hardware encoders tried at startup, confined as a prepare is, in order.
	Trials []EncoderTrial `json:"trials"`
	// ChosenEncoder is the encoder its encodes use: h264_qsv, h264_nvenc or x264.
	ChosenEncoder string `json:"chosenEncoder"`
}

// GPU is one GPU device node the node found.
type GPU struct {
	// DevicePath is its path: /dev/dri/renderD128, /dev/nvidia0.
	DevicePath string `json:"devicePath"`
	// Driver is the kernel driver behind it (i915, xe, amdgpu, nvidia), empty when unknown.
	Driver string `json:"driver,omitempty"`
	// NoAccessReason is why the node's user may not open it for reading and writing, in the system's
	// words; empty when it may.
	NoAccessReason string `json:"noAccessReason,omitempty"`
}

// EncoderTrial is one hardware encoder's trial: it encoded a second of test picture, or Failure
// says why not, in FFmpeg's words.
type EncoderTrial struct {
	Encoder string `json:"encoder"`
	Failure string `json:"failure,omitempty"`
}

// Confinement is one mechanism's state on the node: in force, or not and why (cut-a §7).
type Confinement struct {
	Mechanism string `json:"mechanism"`
	IsInForce bool   `json:"isInForce"`
	// CauseCode names why it is not in force, from confine's closed set, for a client's own words.
	CauseCode string `json:"causeCode,omitempty"`
	// Reason says why a mechanism is not in force, and Remedy what would put it there.
	Reason string `json:"reason"`
	Remedy string `json:"remedy"`
}

// HelloReply is the core's answer: the node's id, and the last report sequence number the core
// ingested, from which the node continues numbering.
type HelloReply struct {
	NodeID                int64 `json:"nodeId"`
	LastIngestedReportSeq int64 `json:"lastIngestedReportSeq"`
}
