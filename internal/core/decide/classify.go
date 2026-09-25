// SPDX-License-Identifier: GPL-3.0-or-later

package decide

import (
	"slices"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// The table that sorts what a file holds into menu names. A stream it cannot place is prepared,
// never guessed playable (D217 §1). Level is left out: naming it is the condition D21 warns turns
// the menu into a grammar. Dolby Vision cannot be seen in the probe; a profile 5 file sorts as
// HEVC and fails in the browser, which a person answers with "prepare" (user_choice).

// profileMain is ffprobe's name for the main profile of H.264, HEVC and AV1 alike.
const profileMain = "Main"

// tenBitPixels is the 10-bit 4:2:0 pixel format, the only one HDR10 comes in.
const tenBitPixels = "yuv420p10le"

// eightBit420 and tenBit420 are the pixel formats a browser decodes: 4:2:0, 8 or 10 bits.
func eightBit420() []string { return []string{"yuv420p", "yuvj420p"} }
func tenBit420() []string   { return []string{"yuv420p", "yuvj420p", tenBitPixels} }

// h264Profiles are the H.264 profiles every browser decodes; High 10, 4:2:2 and 4:4:4 are not.
func h264Profiles() []string {
	return []string{"Baseline", "Constrained Baseline", profileMain, "High"}
}

// aacProfiles are the AAC profiles every browser decodes; "" is ffprobe's answer when the
// container does not say, which is LC in practice.
func aacProfiles() []string { return []string{"", "LC", "HE-AAC", "HE-AACv2"} }

// pqTransfer is HDR10's transfer (SMPTE ST 2084); hlgTransfer is HLG's, which no menu entry is.
const (
	pqTransfer  = "smpte2084"
	hlgTransfer = "arib-std-b67"
)

// videoName is the menu entry a video stream is, or false when it is none.
func videoName(stream nodeproto.ProbeStream) (Video, bool) {
	picture := stream.Video
	sdr := picture.ColorTransfer != pqTransfer && picture.ColorTransfer != hlgTransfer
	switch stream.Codec {
	case "h264":
		decodable := slices.Contains(h264Profiles(), stream.Profile) &&
			slices.Contains(eightBit420(), picture.PixelFormat)
		if decodable && sdr {
			return H264SDR, true
		}
	case "hevc":
		return hdrOrSDR(stream, HEVCSDR, HEVCHDR10, []string{profileMain, "Main 10"})
	case "av1":
		return hdrOrSDR(stream, AV1SDR, AV1HDR10, []string{profileMain})
	}
	return "", false
}

// hdrOrSDR places an HEVC or AV1 stream: SDR at 8 or 10 bits, or HDR10 at 10 bits.
func hdrOrSDR(stream nodeproto.ProbeStream, sdr, hdr10 Video, profiles []string) (Video, bool) {
	picture := stream.Video
	if !slices.Contains(profiles, stream.Profile) || !slices.Contains(tenBit420(), picture.PixelFormat) {
		return "", false
	}
	switch picture.ColorTransfer {
	case pqTransfer:
		if picture.PixelFormat == tenBitPixels {
			return hdr10, true
		}
		return "", false
	case hlgTransfer:
		return "", false
	}
	return sdr, true
}

// audioName is the menu entry an audio stream is, or false when it is none (TrueHD, DTS, MP3
// and PCM are not on the menu).
func audioName(stream nodeproto.ProbeStream) (Audio, bool) {
	switch stream.Codec {
	case "aac":
		if !slices.Contains(aacProfiles(), stream.Profile) {
			return "", false
		}
		if stream.Audio.Channels > 2 {
			return AACMulti, true
		}
		return AACStereo, true
	case "ac3":
		return AC3, true
	case "eac3":
		return EAC3, true
	case "opus":
		return Opus, true
	case "flac":
		return FLAC, true
	}
	return "", false
}

// ContainerOf is the container a file is, from the demuxer's names, or false when none of the
// menu's.
func ContainerOf(format nodeproto.ProbeFormat) (Container, bool) {
	switch {
	case slices.Contains(format.Names, "mp4") || slices.Contains(format.Names, "mov"):
		return MP4, true
	case slices.Contains(format.Names, "matroska"):
		return Matroska, true
	case slices.Contains(format.Names, "mpegts"):
		return MPEGTS, true
	}
	return "", false
}

// mainVideo is the file's picture: its first video stream that is not cover art.
func mainVideo(probe nodeproto.Probe) (nodeproto.ProbeStream, bool) {
	for _, stream := range probe.Streams {
		if stream.Kind == "video" && !slices.Contains(stream.Disposition, "attached_pic") {
			return stream, true
		}
	}
	return nodeproto.ProbeStream{}, false
}

// mainAudio is the track a player starts with: the one marked default, else the first.
func mainAudio(probe nodeproto.Probe) (nodeproto.ProbeStream, bool) {
	var first *nodeproto.ProbeStream
	for index := range probe.Streams {
		stream := &probe.Streams[index]
		if stream.Kind != "audio" {
			continue
		}
		if slices.Contains(stream.Disposition, "default") {
			return *stream, true
		}
		if first == nil {
			first = stream
		}
	}
	if first == nil {
		return nodeproto.ProbeStream{}, false
	}
	return *first, true
}
