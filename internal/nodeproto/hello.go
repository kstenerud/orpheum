// SPDX-License-Identifier: AGPL-3.0-or-later

package nodeproto

import (
	"github.com/kstenerud/orpheum/internal/platform/build"
	"github.com/kstenerud/orpheum/internal/platform/host"
	"github.com/kstenerud/orpheum/internal/platform/pathtree"
)

// The node channel's paths on the core's socket (cut-a §4). The node dials; the core answers.
const (
	PathHello    = "/node/v1/hello"
	PathCommands = "/node/v1/commands"
	// PathStatus is where the node posts its Status, each tick.
	PathStatus = "/node/v1/status"
	// PathLocked is where a locked node posts its LockedReport, each tick, until the core lets it run
	// unconfined (D450 §1).
	PathLocked = "/node/v1/locked"
)

// QueryAfterCommandSeq is the commands long-poll's parameter: the sequence number of the last command
// the node took, so the core answers with the ones after it.
const QueryAfterCommandSeq = "afterCommandSeq"

// ProtocolVersion is the version of the node protocol this build speaks: its paths, commands,
// records and results. It is raised with any change a node or core of another build would misread,
// so the two refuse each other at hello instead (D233 §9). Core and node ship as one build today,
// so the two match unless a node is updated apart from its core.
const ProtocolVersion = 61

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
	// ByteSocketPath is where the node's byte socket is, in its instance's run directory, from which
	// the core reads its media bytes and the results it holds (cut-a §4, D242 §7).
	ByteSocketPath string `json:"byteSocketPath"`
	// RootPaths are the media roots configured on the node, never remotely (D107).
	RootPaths []string `json:"rootPaths"`
	// SystemFolder is the folder of the node's own the server's system videos are made into (D435
	// §4): one of RootPaths, in the node's persistent directory, never a media root's.
	SystemFolder  string `json:"systemFolder,omitempty"`
	FFmpegVersion string `json:"ffmpegVersion"`
	// FFmpegSha256 is the SHA-256 of the ffmpeg binary, so the core can tell which build ran.
	FFmpegSha256 string `json:"ffmpegSha256"`
	// Capabilities are what the node found it can do (cut-a §4): remux and aac always, and the
	// encoders it can use, h264_qsv, h264_nvenc, h264_vaapi and x264.
	Capabilities []string `json:"capabilities"`
	// Confinement is what the node got, per mechanism (D112).
	Confinement []Confinement `json:"confinement"`
	// IsUnconfinedByLeave is a node that requires confinement, lacks a mechanism, and runs without it
	// because an admin let it (D450 §1). The leave stands: an admin who no longer wants the node to run
	// stops it.
	IsUnconfinedByLeave bool `json:"isUnconfinedByLeave,omitempty"`
	UID                 int  `json:"uid"`
	GID                 int  `json:"gid"`
	// Hardware is what the node found for hardware encoding (D225 §3).
	Hardware Hardware `json:"hardware"`
	// Capacity is how much work the node takes on at once, and how fast it reads, from its config.
	Capacity Capacity `json:"capacity"`
	// Devices are what its roots and prepared directory are on, and of what kind (D240 §5).
	Devices Devices `json:"devices,omitempty"`
}

// Capacity is what a node's machine can do, as its config says: facts about the deployment, not
// the admin's opinions, so they are the node's config keys, not the core's settings (cut-a §5.3 as
// plan-deviations P36 records).
type Capacity struct {
	// EncodeSlotCount is how many encodes the node runs at once, which its processor or GPU bounds;
	// RemuxSlotCount is how many copies, which its disk bounds. The node holds a run beyond
	// its slots and says so (ProgressWaiting), and the core runs as many at once as the
	// nodes' slots add up to.
	EncodeSlotCount int `json:"encodeSlotCount"`
	RemuxSlotCount  int `json:"remuxSlotCount"`
	// SlowestReadBytesPerSecond is the slowest the node expects to read a whole file: a busy
	// spinning disk, or a network mount. Its watchdog allows a run that reads one that long, and the
	// core waits that long for a read-through.
	SlowestReadBytesPerSecond int64 `json:"slowestReadBytesPerSecond"`
}

// Hardware is what a node found for hardware encoding: the GPU devices it sees, whether it may
// open them, each hardware encoder's trial and the encoder it chose (D225 §3).
type Hardware struct {
	GPUs []GPU `json:"gpus"`
	// Trials are the hardware encoders tried at startup, confined as a run of chunks is, in order.
	Trials []EncoderTrial `json:"trials"`
	// ChosenEncoder is the encoder its encodes use: one of the encoders below.
	ChosenEncoder string `json:"chosenEncoder"`
	// ReadsFilmsThroughNode is whether the node's FFmpeg has the fdproxy protocol, so each run reads
	// its film through the node, which a share that goes away cannot hang (D330); false is the stock
	// build, each run reading its film by its path.
	ReadsFilmsThroughNode bool `json:"readsFilmsThroughNode"`
}

// The encoders a node reports, its trials' and its choice, the hardware ones in the order it tries them
// (cut-a §6, D332).
const (
	EncoderQuickSync = "h264_qsv"
	EncoderNVENC     = "h264_nvenc"
	EncoderVAAPI     = "h264_vaapi"
	// EncoderX264 is the processor's, which a node uses when no hardware encoder works.
	EncoderX264 = "x264"
)

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

// HelloReply is the core's answer: the node's id and the readers each of its devices may have.
type HelloReply struct {
	NodeID int64 `json:"nodeId"`
	// DeviceReaderLimits are how many may read each of the hello's devices at once, by its key: the
	// core's settings for its kind (D240 §5).
	DeviceReaderLimits map[string]int `json:"deviceReaderLimits"`
	// Tuning is how the node behaves, as the core's settings say.
	Tuning Tuning `json:"tuning"`
	// WatchFolders are the folders the node watches for changes: each library folder on it whose
	// library watches for changes, a configured root or a folder inside one (D359), and nothing else,
	// so no folder of a configured root that no library uses is ever walked or watched. The node
	// watches exactly these, starting and stopping watches as they change (Commands.WatchFolders),
	// and refuses one no configured root holds (D107). Nil, from a core that says none, leaves the
	// node's last; empty watches nothing.
	WatchFolders []string `json:"watchFolders"`
}

// LockedReport is what a locked node says, each tick, in place of a hello (D450 §1): a node whose
// config requires confinement, on a kernel that lacks a mechanism of it, reads no media and runs no
// FFmpeg until an admin lets it run without it. It says who it is and what it lacks, so the System page
// can show the admin the risk.
type LockedReport struct {
	// ProtocolVersion is the version of this protocol the node speaks, as a hello's.
	ProtocolVersion int `json:"protocolVersion"`
	// Name is the node's configured name, by which the core enrols it, as a hello's (D207).
	Name string `json:"name"`
	// Build is what the node's binary was built from (D225).
	Build build.Provenance `json:"build"`
	// Confinement is what the node would get, per mechanism (D112): at least one not in force.
	Confinement []Confinement `json:"confinement"`
}

// LockedReply is the core's answer to a LockedReport.
type LockedReply struct {
	// MayRunUnconfined is whether an admin lets the node run without the mechanisms it lacks: once it
	// does, the node starts its media work and says hello (D450 §1).
	MayRunUnconfined bool `json:"mayRunUnconfined"`
}

// Tuning is how a node behaves where the core's settings decide, sent in the hello's reply and each
// poll's answer (the constants sweep, 2026-10-21). A value of zero, from a core that says none, leaves
// the node's last.
type Tuning struct {
	// StatusEverySeconds is how often the node posts its status: the core's nodes.StatusEveryFor.
	StatusEverySeconds int `json:"statusEverySeconds"`
	// ProgressEverySeconds is how often, at most, a running read-through tells the core how far it is
	// (D234 §5): the core's node_progress_every_seconds.
	ProgressEverySeconds int `json:"progressEverySeconds"`
}

// StartingStatusEverySeconds is how often a node posts its status before a core has said: the
// cadence node_silent_after_seconds' starting value gives, which a test of the core's nodes holds.
const StartingStatusEverySeconds = 10

// StartingProgressEverySeconds is how often, at most, a running read-through tells the core how far it is
// before a core has said: node_progress_every_seconds' starting value, defined once here for the
// core's registry and the node.
const StartingProgressEverySeconds = 2

// DeviceKind is how a device reads, which sets how many may read it at once (D240 §5).
type DeviceKind string

// The device kinds.
const (
	// DeviceSpinning is a spinning disk, or a pool, array or volume with one among its data disks:
	// a second reader makes it seek between them.
	DeviceSpinning DeviceKind = "spinning"
	// DeviceSSD is a solid-state disk, or a pool, array or volume of them only.
	DeviceSSD DeviceKind = "ssd"
	// DeviceNetwork is a filesystem mounted over the network.
	DeviceNetwork DeviceKind = "network"
	// DeviceUnknown is one whose disks the node cannot see.
	DeviceUnknown DeviceKind = "unknown"
)

// Device is one thing the node reads its roots and prepared directory from, as the scheduling
// counts readers (D240 §5): a disk, or the pool, array or volume that spreads them over several,
// so that two roots on one disk share it.
type Device struct {
	// Key names it on this node, the same across hellos: "zfs:<pool>", "disk:<disk>[+<disk>…]",
	// "network:<source>", or "unknown:<major>:<minor>".
	Key  string     `json:"key"`
	Kind DeviceKind `json:"kind"`
	// Filesystem is its filesystem's type, as the mount table names it.
	Filesystem string `json:"filesystem"`
	// Disks are the whole disks it reads from, by their kernel names; none when the node cannot see
	// them.
	Disks []string `json:"disks,omitempty"`
	// MountPaths are where it is mounted at or inside the node's roots and prepared directory: a
	// path is on the device of the longest of every device's mount paths it is under.
	MountPaths []string `json:"mountPaths"`
	// WhenGone is how a read of it behaves when what it is shared from goes away, as its mount says:
	// empty for a local disk, or a share mounted to give up (D233 §10).
	WhenGone ShareWhenGone `json:"whenGone,omitempty"`
}

// ShareWhenGone is how a read of a share behaves when its server goes away.
type ShareWhenGone string

// How a share's reads behave when its server goes away.
const (
	// ShareWaitsForever is a share mounted to wait for its server for good, as NFS is by default
	// ("hard"): every read of it hangs in the kernel, unkillable, until the server is back.
	ShareWaitsForever ShareWhenGone = "waits_forever"
	// ShareMayWaitForever is a filesystem passed through from another machine or program (virtiofs,
	// 9p, FUSE), whose own mounts the node cannot see: a hard share behind it hangs as one does.
	ShareMayWaitForever ShareWhenGone = "may_wait_forever"
)

// Devices are the devices a node's roots and prepared directory are on.
type Devices []Device

// KeyOf is the key of the device an absolute path is on: the one with the longest of the mount
// paths it is under, or "" when it is under none.
func (devices Devices) KeyOf(path string) string {
	key, longest := "", -1
	for _, device := range devices {
		for _, mountPath := range device.MountPaths {
			if pathtree.IsAtOrUnder(path, mountPath) && len(mountPath) > longest {
				key, longest = device.Key, len(mountPath)
			}
		}
	}
	return key
}
