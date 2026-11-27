// SPDX-License-Identifier: AGPL-3.0-or-later

package nodeproto

// CommandMakeTrickplay asks the node for a video's trickplay pictures: a picture every interval of
// its time, the keyframe at or before each mark, scaled to a width and laid in sheets of columns ×
// rows, each sheet a WebP (trickplay.md §3). The core chooses the version's sizes; the node only
// makes them.
const CommandMakeTrickplay CommandType = "makeTrickplay"

// PathTrickplaySheet is where the node posts each sheet a run of CommandMakeTrickplay has finished,
// a TrickplaySheet, in order, as it is made: each is a piece, kept by the core whatever becomes of
// the run, so a run stopped partway resumes at the next sheet (trickplay.md §3, work-scheduling.md
// §6). The node posts each until the core takes it, and only then the next; the run's end goes to
// PathResult, as a TrickplayResult.
const PathTrickplaySheet = "/node/v1/trickplay-sheet"

// MakeTrickplay is the video to make the pictures of, and the version's sizes.
type MakeTrickplay struct {
	// Video is the video, named by payload hash and checked before FFmpeg reads it (D100).
	Video BytesFile `json:"video"`
	// VideoStreamIndex is the picture's stream index, as ffprobe numbers every stream.
	VideoStreamIndex int `json:"videoStreamIndex"`
	// IntervalMs is the time between marks: the pictures fall at 0, IntervalMs, 2·IntervalMs, … of
	// the video's time, each the keyframe at or before its mark.
	IntervalMs int64 `json:"intervalMs"`
	// PictureWidth is each picture's width in square pixels; its height keeps the picture's shape,
	// rounded to even.
	PictureWidth int `json:"pictureWidth"`
	// SheetColumns and SheetRows are a sheet's pictures across and down, laid row by row: sheet n
	// holds the pictures of marks n·SheetColumns·SheetRows onwards. The last sheet may hold fewer.
	SheetColumns int `json:"sheetColumns"`
	SheetRows    int `json:"sheetRows"`
	// WebPQuality is libwebp's, 0 to 100.
	WebPQuality int `json:"webpQuality"`
	// SheetsMade are the indexes of the sheets the core already holds: the run starts at the first
	// sheet not among them and posts none of them again.
	SheetsMade []int `json:"sheetsMade,omitempty"`
}

// TrickplaySheet is one finished sheet of a run (PathTrickplaySheet).
type TrickplaySheet struct {
	JobID int64 `json:"jobId"`
	// AttemptToken is the command's (Command.AttemptToken): the attempt that made the sheet.
	AttemptToken string `json:"attemptToken"`
	// Index is the sheet's, from 0 at the video's start.
	Index int `json:"index"`
	// WebP is the sheet's image.
	WebP []byte `json:"webp"`
}

// TrickplayResult is how a run of CommandMakeTrickplay ended, its sheets posted: done, with none of
// these set, once every sheet to the video's end is made; or FFmpeg's word that it cannot, a
// refusal, or a stall, as a read-through answers (ReadThroughResult).
type TrickplayResult struct {
	// Failure is FFmpeg's word that it could not make the pictures: a fact about the bytes, which
	// making them again would not change.
	Failure string `json:"failure,omitempty"`
	// Refusal says why the node did not try: a node that makes no trickplay, or a file that changed
	// since the scan or is gone (RefusalCode).
	Refusal     string      `json:"refusal,omitempty"`
	RefusalCode RefusalCode `json:"refusalCode,omitempty"`
	// Stall is set when the run stalled: the sheets posted before it are kept, and the run resumes
	// at the next.
	Stall *Stall `json:"stall,omitempty"`
}

// mediaFile is the video the pictures are made of.
func (command *MakeTrickplay) mediaFile() (BytesFile, bool) {
	if command == nil {
		return BytesFile{}, false
	}
	return command.Video, true
}
