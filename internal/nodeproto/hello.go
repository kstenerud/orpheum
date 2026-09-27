// SPDX-License-Identifier: GPL-3.0-or-later

package nodeproto

// The node channel's paths on the core's socket (cut-a §4). The node dials; the core answers.
const (
	PathHello    = "/node/v1/hello"
	PathReport   = "/node/v1/report"
	PathCommands = "/node/v1/commands"
)

// Hello is the first thing a node says, and it says it again after every reconnection.
type Hello struct {
	// Name is the node's configured name; the core enrols a node it has not seen by it (D207).
	Name    string `json:"name"`
	Version string `json:"version"`
	// Roots are the media roots configured on the node, never remotely (D107).
	Roots         []string `json:"roots"`
	FFmpegVersion string   `json:"ffmpegVersion"`
	// FFmpegHash is the SHA-256 of the ffmpeg binary, so the core can tell which build ran.
	FFmpegHash string `json:"ffmpegHash"`
	// Capabilities are what the node found it can do (cut-a §4): remux and aac always, and the
	// encoders it can use, h264_qsv, h264_nvenc and x264.
	Capabilities []string `json:"capabilities"`
	// Confinement is what the node got, per mechanism (D112).
	Confinement []Confinement `json:"confinement"`
	UID         int           `json:"uid"`
	GID         int           `json:"gid"`
}

// Confinement is one mechanism's state on the node: in force, or not and why (cut-a §7).
type Confinement struct {
	Mechanism string `json:"mechanism"`
	InForce   bool   `json:"inForce"`
	// Cause names why it is not in force, from confine's closed set, for a client's own words.
	Cause string `json:"cause,omitempty"`
	// Reason says why a mechanism is not in force, and Remedy what would put it there.
	Reason string `json:"reason"`
	Remedy string `json:"remedy"`
}

// HelloReply is the core's answer: the node's id, and the last report sequence number the core
// ingested, from which the node continues numbering.
type HelloReply struct {
	NodeID    int64 `json:"nodeId"`
	ReportSeq int64 `json:"reportSeq"`
}
