// SPDX-License-Identifier: GPL-3.0-or-later

package prepare

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// The HLS shape of every rendition (cut-a §6): fMP4 segments of about two seconds, so switching
// between the standard and boosted mix takes about a second (D217 §3), and an EVENT playlist that
// grows while the job runs, so playback starts before it ends.
const (
	segmentSeconds = "2"
	audioBitRate   = "192k"
	masterPlaylist = "master.m3u8"
)

// The two stereo mixes of a track with a centre, from 5.1 (a 7.1 source is first folded to it).
// The standard mix is ITU-R BS.775's; the boosted one raises the centre, where dialogue is, by
// 6 dB over it. "<" renormalises each side's gains to sum to one, so neither can clip. The
// boost's gain is provisional until the listening test on a television sets it (cut-a §6).
const (
	standardMix = "aformat=channel_layouts=5.1,pan=stereo|FL<FL+0.707*FC+0.707*BL|FR<FR+0.707*FC+0.707*BR"
	boostedMix  = "aformat=channel_layouts=5.1,pan=stereo|FL<FL+1.414*FC+0.707*BL|FR<FR+1.414*FC+0.707*BR"
	stereoMix   = "aformat=channel_layouts=stereo"
)

// safeLanguage is a language tag FFmpeg's stream map can carry: letters and hyphens only.
var safeLanguage = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{1,8})*$`)

// mix is one audio rendition: its name in the stream map, which is also its directory, and its
// filter.
type mix struct {
	name, filter string
}

// mixes are a track's renditions: standard and boosted when it has a centre, else one.
func mixes(track nodeproto.PrepareAudio) []mix {
	name := "audio" + strconv.Itoa(track.Index)
	if track.Channels > 2 {
		return []mix{{name, standardMix}, {name + "-boost", boostedMix}}
	}
	return []mix{{name, stereoMix}}
}

// Arguments is FFmpeg's command line for one prepare, after execx's hardening flags: input is
// the source's absolute path, output the rendition's absolute directory.
func Arguments(input, output string, command nodeproto.Prepare) []string {
	arguments := []string{
		"-v", "error", "-nostats", "-progress", "pipe:1",
		"-protocol_whitelist", "file", "-i", "file:" + input,
		"-map", "0:" + strconv.Itoa(command.VideoStream),
	}
	arguments = append(arguments, videoArguments(command)...)
	streams := []string{"v:0,agroup:audio,name:video"}
	slot := 0
	for _, track := range command.Audio {
		for _, rendition := range mixes(track) {
			arguments = append(arguments, "-map", "0:a:"+strconv.Itoa(track.Index),
				"-filter:a:"+strconv.Itoa(slot), rendition.filter)
			entry := "a:" + strconv.Itoa(slot) + ",agroup:audio,language:" + language(track.Language) +
				",name:" + rendition.name
			if slot == 0 {
				entry += ",default:yes"
			}
			streams = append(streams, entry)
			slot++
		}
	}
	return append(arguments,
		"-c:a", "aac", "-b:a", audioBitRate, "-ac", "2", "-sn", "-dn",
		"-f", "hls", "-hls_time", segmentSeconds, "-hls_playlist_type", "event",
		"-hls_segment_type", "fmp4", "-hls_fmp4_init_filename", "init.mp4",
		"-hls_segment_filename", filepath.Join(output, "%v", "seg%05d.m4s"),
		"-master_pl_name", masterPlaylist,
		"-var_stream_map", strings.Join(streams, " "),
		filepath.Join(output, "%v", "stream.m3u8"),
	)
}

// videoArguments copy the picture for remux, tagging HEVC as hvc1 for Safari, or re-encode it to
// 8-bit H.264 at about 8 Mb/s with a keyframe every two seconds (cut-a §6).
func videoArguments(command nodeproto.Prepare) []string {
	const codec = "-c:v"
	if command.Profile == nodeproto.ProfileRemux {
		if command.VideoCodec == "hevc" {
			return []string{codec, "copy", "-tag:v", "hvc1"}
		}
		return []string{codec, "copy"}
	}
	return []string{
		codec, "libx264", "-preset", "veryfast", "-profile:v", "high", "-pix_fmt", "yuv420p",
		"-b:v", "8M", "-maxrate", "10M", "-bufsize", "16M",
		"-force_key_frames", "expr:gte(t,n_forced*2)",
	}
}

// language is a track's tag as the stream map may carry it, or "und".
func language(tag string) string {
	if safeLanguage.MatchString(tag) {
		return tag
	}
	return "und"
}
