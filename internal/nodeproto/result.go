// SPDX-License-Identifier: GPL-3.0-or-later

package nodeproto

// PathResult is where the node posts its word on a job: a Result.
const PathResult = "/node/v1/result"

// PathBytesResult is the byte socket's route for a result held on the node, too large to post:
// PathBytesResult + its attempt's token (D234 §5).
const PathBytesResult = "/bytes/v1/result/"

// MaxPostedResultBytes is the largest result the node posts whole, encoded: one larger is held on
// the node and posted as a HeldResult, which the core fetches from the byte socket. A mebibyte
// carries any prepare, measurement or ordinary subtitle; an artwork re-encode's sizes are larger.
const MaxPostedResultBytes = 1 << 20

// HeldResult names a result the node holds, too large to post: the core fetches it whole, its JSON
// as the node encoded it, from PathBytesResult + its token, and takes it only at this size and
// SHA-256. The node holds it until the core acknowledges it.
type HeldResult struct {
	SizeBytes int64 `json:"sizeBytes"`
	// Sha256 is the SHA-256 of the result's JSON, in hex.
	Sha256 string `json:"sha256"`
}

// Result is the node's word on a job the core gave it, by the job's id and the attempt's token: one of a
// prepare's reports, a subtitle's text, a loudness measurement or an artwork re-encode. Exactly
// one is set, the one that answers the command the job sent, or none and a Refusal.
type Result struct {
	JobID int64 `json:"jobId"`
	// AttemptToken is the command's (Command.AttemptToken), echoed: the attempt this answers.
	AttemptToken string `json:"attemptToken"`
	// Work is the work this answers, as the node holds it, without the bytes it carried (an image, a
	// fetched text): what the core records the result by, whether or not a job still waits for it
	// (D238 §2, S81, S84).
	Work *Work `json:"work"`
	// Held, set, says the result is held on the node, too large to post (MaxPostedResultBytes): the
	// post carries only its job, attempt and work, and the core fetches the rest.
	Held *HeldResult `json:"held,omitempty"`
	// Refusal says why the node took no part of the command: a command it does not know, from a
	// core of another build, which the core reads as the node needing an update (D233 §9).
	Refusal  string          `json:"refusal,omitempty"`
	Prepare  *PrepareReport  `json:"prepare,omitempty"`
	Subtitle *SubtitleText   `json:"subtitle,omitempty"`
	Loudness *LoudnessResult `json:"loudness,omitempty"`
	Artwork  *ArtworkResult  `json:"artwork,omitempty"`
}
