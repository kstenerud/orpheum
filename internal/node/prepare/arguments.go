// SPDX-License-Identifier: GPL-3.0-or-later

package prepare

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// The HLS shape of every rendition (cut-a §6): fMP4 segments of the length the core asks for, two
// seconds unless the owner changed hls_segment_length_s (D217 §3), and an EVENT playlist that grows
// while the job runs, so playback starts before it ends.
const (
	defaultSegmentLengthSeconds = 2
	maxSegmentLengthSeconds     = 10
	audioBitRate                = "192k"
	masterPlaylist              = "master.m3u8"
)

// The two stereo mixes of a track with a centre, from 5.1 (a 7.1 source is first folded to it).
// The standard mix is ITU-R BS.775's; the boosted one raises the centre, where dialogue is, by
// 6 dB over it. "<" renormalises each side's gains to sum to one, so neither can clip. The
// boost's gain is provisional until the listening test on a television sets it (cut-a §6).
// StandardMix and StereoMix are what a stereo device hears, so internal/node/loudness measures
// them (D226 §1).
const (
	StandardMix = "aformat=channel_layouts=5.1,pan=stereo|FL<FL+0.707*FC+0.707*BL|FR<FR+0.707*FC+0.707*BR"
	boostedMix  = "aformat=channel_layouts=5.1,pan=stereo|FL<FL+1.414*FC+0.707*BL|FR<FR+1.414*FC+0.707*BR"
	StereoMix   = "aformat=channel_layouts=stereo"
)

// safeLanguage is a language tag FFmpeg's stream map can carry: letters and hyphens only.
var safeLanguage = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{1,8})*$`)

// mix is one audio rendition: its name in the stream map, which is also its directory, and its
// filter.
type mix struct {
	name, filter string
}

// mixes are a track's renditions: standard and boosted when it has a centre, else one. Each
// carries the track's levelling gain, after the mix it was measured as (D226 §4).
func mixes(track nodeproto.PrepareAudio) []mix {
	name := "audio" + strconv.Itoa(track.AudioTrackIndex)
	levelled := ""
	if track.GainDB != 0 {
		levelled = ",volume=" + strconv.FormatFloat(track.GainDB, 'f', 1, 64) + "dB"
	}
	if track.Channels > 2 {
		return []mix{{name, StandardMix + levelled}, {name + "-boost", boostedMix + levelled}}
	}
	return []mix{{name, StereoMix + levelled}}
}

// Arguments is FFmpeg's command line for one prepare, after execx's hardening flags: the source's
// absolute path, the rendition's absolute directory, and, for an encode, the encoder it uses.
func Arguments(sourcePath, renditionPath string, command nodeproto.Prepare, encoder Encoder) []string {
	arguments := []string{
		"-v", "error", "-nostats", "-progress", "pipe:1",
		"-protocol_whitelist", "file", "-i", "file:" + sourcePath,
		"-map", "0:" + strconv.Itoa(command.VideoStreamIndex),
	}
	arguments = append(arguments, videoArguments(command, encoder)...)
	streams := []string{"v:0,agroup:audio,name:video"}
	slot := 0
	for _, track := range command.Audio {
		for _, rendition := range mixes(track) {
			arguments = append(arguments, "-map", "0:a:"+strconv.Itoa(track.AudioTrackIndex),
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
		"-f", "hls", "-hls_time", strconv.Itoa(segmentLengthSeconds(command.SegmentLengthSeconds)),
		"-hls_playlist_type", "event",
		"-hls_segment_type", "fmp4", "-hls_fmp4_init_filename", "init.mp4",
		"-hls_segment_filename", filepath.Join(renditionPath, "%v", "seg%05d.m4s"),
		"-master_pl_name", masterPlaylist,
		"-var_stream_map", strings.Join(streams, " "),
		filepath.Join(renditionPath, "%v", "stream.m3u8"),
	)
}

// videoCopied is FFmpeg's name for a video copied, not encoded: a remux's.
const videoCopied = "copy"

// videoArguments copy the picture for remux, tagging HEVC as hvc1 for Safari, or re-encode it with
// the node's encoder (cut-a §6).
func videoArguments(command nodeproto.Prepare, encoder Encoder) []string {
	const codec = "-c:v"
	if command.Profile == nodeproto.ProfileRemux {
		if command.VideoCodec == "hevc" {
			return []string{codec, videoCopied, "-tag:v", "hvc1"}
		}
		return []string{codec, videoCopied}
	}
	return encoder.arguments()
}

// segmentLengthSeconds is the segment length asked for, or the default when it is out of bounds.
func segmentLengthSeconds(asked int) int {
	if asked < 1 || asked > maxSegmentLengthSeconds {
		return defaultSegmentLengthSeconds
	}
	return asked
}

// language is a track's tag as the stream map may carry it, or "und".
func language(tag string) string {
	if safeLanguage.MatchString(tag) {
		return tag
	}
	return "und"
}
