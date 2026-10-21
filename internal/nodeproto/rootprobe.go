// SPDX-License-Identifier: AGPL-3.0-or-later

package nodeproto

import "net/url"

// PathBytesProbe is the node byte socket's route for a probe of one of its roots: whether it can be
// read now, answered outside every queue, without a slot or a reader (D327).
const PathBytesProbe = "/bytes/v1/probe"

// ProbeRequest names the root a probe asks about: one of the node's roots, as its hello named it.
type ProbeRequest struct {
	RootPath string
}

// Target is the probe's path and query on the byte socket.
func (request ProbeRequest) Target() string {
	return PathBytesProbe + "?" + url.Values{"rootPath": {request.RootPath}}.Encode()
}

// ProbeState is what a probe found of a root.
type ProbeState string

const (
	// ProbeReadable is a root whose filesystem answered, with its facts.
	ProbeReadable ProbeState = "readable"
	// ProbeNotAnswering is a root whose disk did not answer within the probe's window, or holds a
	// reading that has not returned (D233 §10).
	ProbeNotAnswering ProbeState = "notAnswering"
	// ProbeMissing is a root whose directory is not there.
	ProbeMissing ProbeState = "missing"
	// ProbeFailed is a root its filesystem answered with another failure.
	ProbeFailed ProbeState = "failed"
)

// RootProbe is a node's answer to a probe of one of its roots (D327 §2).
type RootProbe struct {
	RootPath string     `json:"rootPath"`
	State    ProbeState `json:"state"`
	// Device and IsMountPoint are a readable root's facts, as a scan's first record says them (D42,
	// D208): what the core compares with the root's last complete scan.
	Device       uint64 `json:"device,omitempty"`
	IsMountPoint bool   `json:"isMountPoint,omitempty"`
	// StuckSinceMs is, for a root holding a reading that has not returned, since when, in
	// milliseconds since the epoch, and WaitingOnPath what it waits on; zero for a disk that failed
	// the probe at once.
	StuckSinceMs  int64  `json:"stuckSinceMs,omitempty"`
	WaitingOnPath string `json:"waitingOnPath,omitempty"`
	// Failure says why it is not readable, for any state but readable.
	Failure string `json:"failure,omitempty"`
}
