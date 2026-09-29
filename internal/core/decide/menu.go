// SPDX-License-Identifier: GPL-3.0-or-later

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

// Menu is what a client can play, one list per axis, most preferred first, with no weights
// (D217 §1).
type Menu struct {
	VideoCodecs     []Video
	AudioCodecs     []Audio
	SubtitleFormats []Subtitle
	Containers      []Container
}

// ErrBadMenu is a menu that names something the menu does not have, or lacks a baseline.
var ErrBadMenu = errors.New("the menu")

// Validate refuses a menu without both baselines (D21: every client lists them, so negotiation
// cannot fail) or with a name the menu does not have.
func (menu Menu) Validate() error {
	if !slices.Contains(menu.VideoCodecs, H264SDR) || !slices.Contains(menu.AudioCodecs, AACStereo) {
		return fmt.Errorf("%w must list %s and %s, the baselines every client plays (D21)",
			ErrBadMenu, H264SDR, AACStereo)
	}
	if err := checkAllOnMenu("video", menu.VideoCodecs, videos()); err != nil {
		return err
	}
	if err := checkAllOnMenu("audio", menu.AudioCodecs, audios()); err != nil {
		return err
	}
	if err := checkAllOnMenu("subtitle", menu.SubtitleFormats, subtitles()); err != nil {
		return err
	}
	return checkAllOnMenu("container", menu.Containers, containers())
}

func checkAllOnMenu[T comparable](axis string, listed, defined []T) error {
	for _, name := range listed {
		if !slices.Contains(defined, name) {
			return fmt.Errorf("%w has no %s entry %v", ErrBadMenu, axis, name)
		}
	}
	return nil
}
