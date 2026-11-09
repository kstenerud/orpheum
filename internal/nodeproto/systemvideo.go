// SPDX-License-Identifier: AGPL-3.0-or-later

package nodeproto

// CommandMakeSystemVideos asks the node to make the server's own videos into its system folder
// (Hello.SystemFolder), on their first use or for a rebuild (D435 §4, §7): each a short clip FFmpeg
// draws from nothing, which the node then scans as any file.
const CommandMakeSystemVideos CommandType = "makeSystemVideos"

// SystemVideo is one of the server's own videos, named by what it is (D435 §6): what it is for, and
// the codecs it is made in. The set is closed, the node knowing each one's recipe.
type SystemVideo struct {
	// Purpose is what the video is for: SystemVideoSoundTiming.
	Purpose string `json:"purpose"`
	// Codec is the video and audio codecs it is made in: SystemVideoH264AAC.
	Codec string `json:"codec"`
	// FileName is its file's name in the system folder, which the core chooses, so it finds the file a
	// scan reports by what it is: a bare name, never a path.
	FileName string `json:"fileName"`
}

// The system videos' purposes and codecs.
const (
	// SystemVideoSoundTiming is the device lab's sound timing clip (lab-sound-clip.ts): four seconds,
	// a rising beep at the start of each, the picture flashing the second's number as it starts.
	SystemVideoSoundTiming = "soundTiming"
	// SystemVideoH264AAC is H.264 video and stereo AAC audio in MP4, which every client plays.
	SystemVideoH264AAC = "h264-aac"
)

// MakeSystemVideos names the videos to make. The node makes them into its own system folder, never a
// folder the core names, so the command can write nowhere else.
type MakeSystemVideos struct {
	Videos []SystemVideo `json:"videos"`
	// IsRebuild makes each again even when its file is there: a changed recipe or a damaged file
	// (D435 §7). Otherwise a video whose file is there is kept.
	IsRebuild bool `json:"isRebuild,omitempty"`
}

// SystemVideosMade is the node's answer: its system folder, and each video made or kept there, or why
// one could not be made.
type SystemVideosMade struct {
	Folder string `json:"folder"`
	// FileNames are the videos' files in the folder, made or kept, in the order asked.
	FileNames []string `json:"fileNames,omitempty"`
	// Failure says why a video could not be made: FFmpeg's or the platform's words. The videos made
	// before it are kept.
	Failure string `json:"failure,omitempty"`
}
