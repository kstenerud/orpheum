// SPDX-License-Identifier: GPL-3.0-or-later

package prepare

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/langtag"
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
// StandardMix and StereoAsIsMix are what a stereo device hears, so internal/node/loudness measures
// them (D226 §1).
const (
	StandardMix   = "aformat=channel_layouts=5.1,pan=stereo|FL<FL+0.707*FC+0.707*BL|FR<FR+0.707*FC+0.707*BR"
	boostedMix    = "aformat=channel_layouts=5.1,pan=stereo|FL<FL+1.414*FC+0.707*BL|FR<FR+1.414*FC+0.707*BR"
	StereoAsIsMix = "aformat=channel_layouts=stereo"
)

// mix is one audio rendition: its name in the stream map, which is also its directory, and its
// filter.
type mix struct {
	name, filter string
}

// mixes are a track's renditions: standard and boosted when it has a centre, else one. Each
// carries the track's levelling gain, after the mix it was measured as (D226 §4).
func mixes(track nodeproto.PrepareAudio) []mix {
	name := "audio" + strconv.Itoa(track.AudioTrackIndex)
	gainFilter := ""
	if track.CarriedGainDB != 0 {
		gainFilter = ",volume=" + strconv.FormatFloat(track.CarriedGainDB, 'f', 1, 64) + "dB"
	}
	if track.ChannelCount > 2 {
		return []mix{{name, StandardMix + gainFilter}, {name + "-boost", boostedMix + gainFilter}}
	}
	return []mix{{name, StereoAsIsMix + gainFilter}}
}

// Arguments is FFmpeg's command line for one prepare, after execx's hardening flags: the source's
// absolute path, the rendition's absolute directory, and, for an encode, the encoder it uses.
func Arguments(sourcePath, renditionPath string, command nodeproto.Prepare, encoder Encoder) []string {
	arguments := append([]string{"-v", "error", "-nostats", "-progress", "pipe:1"}, execx.FileInput(sourcePath)...)
	arguments = append(arguments, "-map", "0:"+strconv.Itoa(command.VideoStreamIndex))
	arguments = append(arguments, videoArguments(command, encoder)...)
	streams := []string{"v:0,agroup:audio,name:video"}
	audioOutputIndex := 0
	for _, track := range command.AudioTracks {
		for _, rendition := range mixes(track) {
			arguments = append(arguments, "-map", "0:a:"+strconv.Itoa(track.AudioTrackIndex),
				"-filter:a:"+strconv.Itoa(audioOutputIndex), rendition.filter)
			entry := "a:" + strconv.Itoa(audioOutputIndex) + ",agroup:audio,language:" +
				streamMapLanguage(track.Language) +
				",name:" + rendition.name
			if audioOutputIndex == 0 {
				entry += ",default:yes"
			}
			streams = append(streams, entry)
			audioOutputIndex++
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
	const codecFlag = "-c:v"
	if command.Profile == nodeproto.ProfileRemux {
		if command.VideoCodec == "hevc" {
			return []string{codecFlag, videoCopied, "-tag:v", "hvc1"}
		}
		return []string{codecFlag, videoCopied}
	}
	return encoder.encodeArguments()
}

// segmentLengthSeconds is the segment length asked for, or the default when it is out of bounds.
func segmentLengthSeconds(asked int) int {
	if asked < 1 || asked > maxSegmentLengthSeconds {
		return defaultSegmentLengthSeconds
	}
	return asked
}

// streamMapLanguage is a track's tag as the stream map may carry it, or langtag.Undetermined: a
// well-formed tag has only letters, digits and hyphens.
func streamMapLanguage(tag string) string {
	if langtag.IsWellFormed(tag) {
		return tag
	}
	return langtag.Undetermined
}
