// SPDX-License-Identifier: AGPL-3.0-or-later

package nodeproto

// PathResult is where the node posts its word on a job: a Result.
const PathResult = "/node/v1/result"

// PathBytesResult is the byte socket's route for a result held on the node, too large to post:
// PathBytesResult + its attempt's token (D234 §5).
const PathBytesResult = "/bytes/v1/result/"

// MaxPostedResultBytes is the largest result the node posts whole, encoded: one larger is held on
// the node and posted as a HeldResult, which the core fetches from the byte socket. A mebibyte
// carries any run of chunks, read-through or ordinary subtitle; an artwork re-encode's sizes are larger.
const MaxPostedResultBytes = 1 << 20

// HeldResult names a result the node holds, too large to post: the core fetches it whole, its JSON
// as the node encoded it, from PathBytesResult + its token, and takes it only at this size and
// SHA-256. The node holds it until the core acknowledges it.
type HeldResult struct {
	SizeBytes int64 `json:"sizeBytes"`
	// Sha256 is the SHA-256 of the result's JSON, in hex.
	Sha256 string `json:"sha256"`
}

// Result is the node's word on a job the core gave it, by the job's id and the attempt's token: one of
// a run of chunks' reports, a subtitle's text, a read-through's findings, an artwork re-encode, a listing or
// an examining. Exactly one is set, the one that answers the command the job sent, or none and a
// Refusal.
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
	Refusal string `json:"refusal,omitempty"`
	// Crashed says the node's own code failed running the work, a panic, with where: the work ended
	// and the node went on. Nothing else in the result is set.
	Crashed     string              `json:"crashed,omitempty"`
	Subtitle    *SubtitleText       `json:"subtitle,omitempty"`
	ReadThrough *ReadThroughResult  `json:"readThrough,omitempty"`
	MakeChunks  *MakeChunksReport   `json:"makeChunks,omitempty"`
	Artwork     *ArtworkBatchResult `json:"artwork,omitempty"`
	Listing     *Listing            `json:"listing,omitempty"`
	Examined    *Examined           `json:"examined,omitempty"`
}

// Stall is what the node saw of the work it answered as stalled, whichever kind it is, or nil.
func (result Result) Stall() *Stall {
	switch {
	case result.Subtitle != nil:
		return result.Subtitle.Stall
	case result.ReadThrough != nil:
		return result.ReadThrough.Stall
	case result.MakeChunks != nil:
		return result.MakeChunks.Stall
	case result.Listing != nil:
		return result.Listing.Stall
	case result.Examined != nil:
		return result.Examined.Stall
	}
	return nil
}
