// SPDX-License-Identifier: AGPL-3.0-or-later

package systemvideo

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// Runner runs FFmpeg: execx.Runner's RunStreaming.
type Runner interface {
	RunStreaming(
		ctx context.Context, tool execx.Tool, access confine.Access, arguments []string, stdout io.Writer,
	) error
}

// Maker makes the system videos into its folder.
type Maker struct {
	runner Runner
	folder string
}

// NewMaker returns a maker writing into folder, the node's system folder.
func NewMaker(runner Runner, folder string) *Maker {
	return &Maker{runner: runner, folder: folder}
}

// Folder is the system folder the maker writes into.
func (maker *Maker) Folder() string { return maker.folder }

// Make makes each video asked into the folder, keeping one whose file is there unless it is a
// rebuild, and says each file's name, or why one could not be made.
func (maker *Maker) Make(ctx context.Context, command nodeproto.MakeSystemVideos) nodeproto.SystemVideosMade {
	made := nodeproto.SystemVideosMade{Folder: maker.folder}
	for _, video := range command.Videos {
		if err := maker.makeOne(ctx, video, command.IsRebuild); err != nil {
			made.Failure = err.Error()
			return made
		}
		made.FileNames = append(made.FileNames, FileName(video))
	}
	return made
}

// makeOne makes one video, or keeps its file when it is there and this is not a rebuild.
func (maker *Maker) makeOne(ctx context.Context, video nodeproto.SystemVideo, isRebuild bool) error {
	recipe, ok := recipes()[video]
	if !ok {
		return fmt.Errorf("no recipe for the system video %s/%s", video.Purpose, video.Codec)
	}
	path := filepath.Join(maker.folder, FileName(video))
	if !isRebuild {
		_, err := os.Stat(path)
		if err == nil {
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("look for %s: %w", path, err)
		}
	}
	arguments := append([]string{"-v", "error", "-y"}, recipe...)
	arguments = append(arguments, "file:"+path)
	err := maker.runner.RunStreaming(ctx, execx.FFmpeg, confine.Access{Write: []string{maker.folder}}, arguments,
		io.Discard)
	if err != nil {
		return fmt.Errorf("make %s: %w", FileName(video), err)
	}
	return nil
}

// All are every system video a server has, in the order they are made.
func All() []nodeproto.SystemVideo {
	return []nodeproto.SystemVideo{{Purpose: nodeproto.SystemVideoSoundTiming, Codec: nodeproto.SystemVideoH264AAC}}
}

// FileName is a video's file's name in the system folder: its purpose and codecs, so a video is found
// by what it is.
func FileName(video nodeproto.SystemVideo) string {
	return video.Purpose + "." + video.Codec + ".mp4"
}

// recipes are each system video's FFmpeg arguments, its inputs to its output's format: every input a
// lavfi source, so FFmpeg reads nothing.
func recipes() map[nodeproto.SystemVideo][]string {
	return map[nodeproto.SystemVideo][]string{
		{Purpose: nodeproto.SystemVideoSoundTiming, Codec: nodeproto.SystemVideoH264AAC}: soundTimingH264AAC(),
	}
}

// soundTimingH264AAC is the device lab's sound timing clip (lab-sound-clip.ts) as a file: four
// seconds, a beep for the first tenth of each, rising 440, 554, 659 and 880 Hz, and the picture
// flashing white with the beep, a bar along its foot growing a quarter each second, so which beep is
// which is seen. A keyframe every second, so a player can start at any beep.
func soundTimingH264AAC() []string {
	const (
		flash = "lt(mod(t,1),0.1)"
		pitch = "440*eq(floor(t),0)+554*eq(floor(t),1)+659*eq(floor(t),2)+880*eq(floor(t),3)"
		bar   = "y=ih-60:w=iw/4:h=60:color=0x2f80ed:t=fill"
	)
	picture := "color=c=black:s=1280x720:r=25:d=4," +
		"drawbox=x=0:y=0:w=iw:h=ih:color=white:t=fill:enable='" + flash + "'," +
		"drawbox=x=0:" + bar + "," +
		"drawbox=x=iw/4:" + bar + ":enable='gte(t,1)'," +
		"drawbox=x=iw/2:" + bar + ":enable='gte(t,2)'," +
		"drawbox=x=3*iw/4:" + bar + ":enable='gte(t,3)'"
	sound := "aevalsrc=exprs='0.5*" + flash + "*sin(2*PI*t*(" + pitch + "))':s=48000:c=stereo:d=4"
	return []string{
		"-f", "lavfi", "-i", picture, "-f", "lavfi", "-i", sound,
		"-c:v", "libx264", "-profile:v", "high", "-pix_fmt", "yuv420p", "-g", "25", "-keyint_min", "25",
		"-c:a", "aac", "-b:a", "128k", "-ac", "2", "-shortest", "-movflags", "+faststart", "-f", "mp4",
	}
}
