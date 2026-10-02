// SPDX-License-Identifier: GPL-3.0-or-later

package nodeproto

// PathResult is where the node posts its word on a job: a Result.
const PathResult = "/node/v1/result"

// Result is the node's word on a job the core gave it, by the job's id (Command.JobID): one of a
// prepare's reports, a subtitle's text, a loudness measurement or an artwork re-encode. Exactly
// one is set, the one that answers the command the job sent, or none and a Refusal.
type Result struct {
	JobID int64 `json:"jobId"`
	// Refusal says why the node took no part of the command: a command it does not know, from a
	// core of another build, which the core reads as the node needing an update (D233 §9).
	Refusal  string          `json:"refusal,omitempty"`
	Prepare  *PrepareReport  `json:"prepare,omitempty"`
	Subtitle *SubtitleText   `json:"subtitle,omitempty"`
	Loudness *LoudnessResult `json:"loudness,omitempty"`
	Artwork  *ArtworkResult  `json:"artwork,omitempty"`
}
