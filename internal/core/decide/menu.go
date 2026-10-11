// SPDX-License-Identifier: AGPL-3.0-or-later

package decide

import (
	"errors"
	"fmt"
	"slices"
)

// Video is a video entry of D21's menu: codec, profile, bit depth and transfer, named together.
type Video string

// The video entries (D21). H264SDR is the baseline every client lists.
const (
	H264SDR   Video = "h264-sdr"
	HEVCSDR   Video = "hevc-sdr"
	HEVCHDR10 Video = "hevc-hdr10"
	AV1SDR    Video = "av1-sdr"
	AV1HDR10  Video = "av1-hdr10"
)

// Audio is an audio entry of D21's menu.
type Audio string

// The audio entries (D21). AACStereo is the baseline every client lists.
const (
	AACStereo Audio = "aac-stereo"
	AACMulti  Audio = "aac-multi"
	AC3       Audio = "ac3"
	EAC3      Audio = "eac3"
	Opus      Audio = "opus"
	FLAC      Audio = "flac"
)

// Subtitle is a subtitle format a client can draw (cut-a §6: ASS for JASSUB, WebVTT elsewhere).
type Subtitle string

// The subtitle formats.
const (
	ASS    Subtitle = "ass"
	WebVTT Subtitle = "webvtt"
)

// Container is a container a client can play as it is (D217 §1). It is asked only of the
// original: a prepared rendition is always fMP4 (D21).
type Container string

// The containers.
const (
	MP4      Container = "mp4"
	Matroska Container = "mkv"
	MPEGTS   Container = "mpegts"
)

func videos() []Video         { return []Video{H264SDR, HEVCSDR, HEVCHDR10, AV1SDR, AV1HDR10} }
func audios() []Audio         { return []Audio{AACStereo, AACMulti, AC3, EAC3, Opus, FLAC} }
func subtitles() []Subtitle   { return []Subtitle{ASS, WebVTT} }
func containers() []Container { return []Container{MP4, Matroska, MPEGTS} }

// PlayableFormats is what a client can play, one list per axis of D21's menu, most preferred
// first, with no weights (D217 §1): the API's playableFormats.
type PlayableFormats struct {
	VideoCodecs     []Video
	AudioCodecs     []Audio
	SubtitleFormats []Subtitle
	Containers      []Container
}

// ErrBadPlayableFormats is a list of playable formats that names something the menu does not
// have, or lacks a baseline.
var ErrBadPlayableFormats = errors.New("the playable formats")

// Validate refuses playable formats without both baselines (D21: every client lists them, so negotiation
// cannot fail) or with a name the menu does not have.
func (formats PlayableFormats) Validate() error {
	if !slices.Contains(formats.VideoCodecs, H264SDR) || !slices.Contains(formats.AudioCodecs, AACStereo) {
		return fmt.Errorf("%w must list %s and %s, the baselines every client plays (D21)",
			ErrBadPlayableFormats, H264SDR, AACStereo)
	}
	if err := checkAllOnMenu("video", formats.VideoCodecs, videos()); err != nil {
		return err
	}
	if err := checkAllOnMenu("audio", formats.AudioCodecs, audios()); err != nil {
		return err
	}
	if err := checkAllOnMenu("subtitle", formats.SubtitleFormats, subtitles()); err != nil {
		return err
	}
	return checkAllOnMenu("container", formats.Containers, containers())
}

func checkAllOnMenu[T comparable](axis string, listed, defined []T) error {
	for _, name := range listed {
		if !slices.Contains(defined, name) {
			return fmt.Errorf("%w has no %s entry %v", ErrBadPlayableFormats, axis, name)
		}
	}
	return nil
}
