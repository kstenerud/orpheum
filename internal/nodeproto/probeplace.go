// SPDX-License-Identifier: AGPL-3.0-or-later

package nodeproto

import "net/url"

// PathBytesProbe is the node byte socket's route for a probe of a place, one of its roots or a
// directory inside one: whether it can be read now, answered outside every queue, without a slot or
// a reader (D327, D328 §6).
const PathBytesProbe = "/bytes/v1/probe"

// ProbeRequest names the place a probe asks about: one of the node's roots, as its hello named it,
// or a disk's mount point inside one.
type ProbeRequest struct {
	Path string
}

// Target is the probe's path and query on the byte socket.
func (request ProbeRequest) Target() string {
	return PathBytesProbe + "?" + url.Values{"path": {request.Path}}.Encode()
}

// ProbeState is what a probe found of a place.
type ProbeState string

const (
	// ProbeReadable is a place whose filesystem answered, with its facts.
	ProbeReadable ProbeState = "readable"
	// ProbeNotAnswering is a place whose disk did not answer within the probe's window, or whose root
	// holds a reading that has not returned (D233 §10).
	ProbeNotAnswering ProbeState = "notAnswering"
	// ProbeMissing is a place whose directory is not there.
	ProbeMissing ProbeState = "missing"
	// ProbeFailed is a place its filesystem answered with another failure.
	ProbeFailed ProbeState = "failed"
)

// PlaceProbe is a node's answer to a probe of a place (D327 §2, D328 §6).
type PlaceProbe struct {
	Path  string     `json:"path"`
	State ProbeState `json:"state"`
	// Device and IsMountPoint are a readable place's facts, as a scan records them (D42, D208): what
	// the core compares with the place's last complete scan.
	Device       uint64 `json:"device,omitempty"`
	IsMountPoint bool   `json:"isMountPoint,omitempty"`
	// StuckSinceMs is, for a place whose root holds a reading that has not returned, since when, in
	// milliseconds since the epoch, and WaitingOnPath what it waits on; zero for a disk that failed
	// the probe at once.
	StuckSinceMs  int64  `json:"stuckSinceMs,omitempty"`
	WaitingOnPath string `json:"waitingOnPath,omitempty"`
	// Failure says why it is not readable, for any state but readable.
	Failure string `json:"failure,omitempty"`
}
