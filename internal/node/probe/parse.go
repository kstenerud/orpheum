// SPDX-License-Identifier: GPL-3.0-or-later

package probe

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/langtag"
)

// Bounds on what one file may make the node report. A real film has a few dozen streams and
// chapters; a file past these is hostile or broken, and is refused rather than truncated, so
// the core never stores a record that silently lost part of a file.
const (
	maxStreams     = 1024
	maxChapters    = 4096
	maxStringBytes = 1024
	maxFormatNames = 16
)

// ErrTooLarge is returned for a file whose probe exceeds the bounds above.
var ErrTooLarge = errors.New("the file's probe exceeds the node's bounds")

// rawOutput is the part of ffprobe's JSON (-show_format -show_streams -show_chapters) the node
// reads. Its keys are ffprobe's.
type rawOutput struct {
	Format   rawFormat    `json:"format"`
	Streams  []rawStream  `json:"streams"`
	Chapters []rawChapter `json:"chapters"`
}

type rawFormat struct {
	FormatName string            `json:"format_name"`
	Duration   string            `json:"duration"`
	BitRate    string            `json:"bit_rate"`
	Tags       map[string]string `json:"tags"`
}

type rawStream struct {
	Index          int               `json:"index"`
	CodecType      string            `json:"codec_type"`
	CodecName      string            `json:"codec_name"`
	Profile        string            `json:"profile"`
	Width          int               `json:"width"`
	Height         int               `json:"height"`
	PixFmt         string            `json:"pix_fmt"`
	RFrameRate     string            `json:"r_frame_rate"`
	ColorTransfer  string            `json:"color_transfer"`
	ColorPrimaries string            `json:"color_primaries"`
	Channels       int               `json:"channels"`
	ChannelLayout  string            `json:"channel_layout"`
	SampleRate     string            `json:"sample_rate"`
	Duration       string            `json:"duration"`
	BitRate        string            `json:"bit_rate"`
	Disposition    map[string]int    `json:"disposition"`
	Tags           map[string]string `json:"tags"`
}

type rawChapter struct {
	StartTime string            `json:"start_time"`
	EndTime   string            `json:"end_time"`
	Tags      map[string]string `json:"tags"`
}

func (raw rawOutput) record() (nodeproto.Probe, error) {
	if len(raw.Streams) > maxStreams || len(raw.Chapters) > maxChapters {
		return nodeproto.Probe{}, fmt.Errorf("%w: %d streams (at most %d), %d chapters (at most %d)",
			ErrTooLarge, len(raw.Streams), maxStreams, len(raw.Chapters), maxChapters)
	}
	names := strings.Split(raw.Format.FormatName, ",")
	if len(names) > maxFormatNames {
		return nodeproto.Probe{}, fmt.Errorf("%w: %d container names", ErrTooLarge, len(names))
	}
	record := nodeproto.Probe{
		Format: nodeproto.ProbeFormat{
			FormatNames:    make([]string, 0, len(names)),
			DurationMs:     milliseconds(raw.Format.Duration),
			BitRateBps:     integer(raw.Format.BitRate),
			ContainerTitle: tag(raw.Format.Tags, "title"),
		},
		Streams:  make([]nodeproto.ProbeStream, 0, len(raw.Streams)),
		Chapters: make([]nodeproto.ProbeChapter, 0, len(raw.Chapters)),
	}
	for _, name := range names {
		if name = bounded(name); name != "" {
			record.Format.FormatNames = append(record.Format.FormatNames, name)
		}
	}
	for _, stream := range raw.Streams {
		record.Streams = append(record.Streams, stream.record())
	}
	for _, chapter := range raw.Chapters {
		record.Chapters = append(record.Chapters, nodeproto.ProbeChapter{
			StartMs:        milliseconds(chapter.StartTime),
			EndMs:          milliseconds(chapter.EndTime),
			ContainerTitle: tag(chapter.Tags, "title"),
		})
	}
	return record, nil
}

func (stream rawStream) record() nodeproto.ProbeStream {
	record := nodeproto.ProbeStream{
		StreamIndex:       stream.Index,
		Kind:              bounded(stream.CodecType),
		Codec:             bounded(stream.CodecName),
		CodecProfile:      bounded(stream.Profile),
		ContainerLanguage: tag(stream.Tags, "language"),
		Language:          langtag.BCP47(tag(stream.Tags, "language")),
		ContainerTitle:    tag(stream.Tags, "title"),
		Disposition:       setFlags(stream.Disposition),
		DurationMs:        streamDuration(stream),
		BitRateBps:        integer(stream.BitRate),
	}
	switch stream.CodecType {
	case "video":
		record.Video = nodeproto.ProbeVideo{
			Width:          stream.Width,
			Height:         stream.Height,
			PixelFormat:    bounded(stream.PixFmt),
			FrameRate:      bounded(stream.RFrameRate),
			ColorTransfer:  bounded(stream.ColorTransfer),
			ColorPrimaries: bounded(stream.ColorPrimaries),
		}
	case "audio":
		record.Audio = nodeproto.ProbeAudio{
			ChannelCount:  stream.Channels,
			ChannelLayout: bounded(stream.ChannelLayout),
			SampleRateHz:  int(integer(stream.SampleRate)),
		}
	case "attachment":
		record.Attachment = nodeproto.ProbeAttachment{
			Filename: tag(stream.Tags, "filename"),
			MimeType: tag(stream.Tags, "mimetype"),
		}
	}
	return record
}

// tag returns a tag's value whatever case the container wrote its key in ("title", "TITLE").
// The exact key wins; otherwise the first match in sorted order, so the record is the same on
// every run however the map iterates.
func tag(tags map[string]string, key string) string {
	if value, found := tags[key]; found {
		return bounded(value)
	}
	for _, name := range slices.Sorted(maps.Keys(tags)) {
		if strings.EqualFold(name, key) {
			return bounded(tags[name])
		}
	}
	return ""
}

// setFlags lists the disposition flags that are set, sorted.
func setFlags(disposition map[string]int) []string {
	flags := make([]string, 0, len(disposition))
	for name, value := range disposition {
		if value != 0 {
			flags = append(flags, bounded(name))
		}
	}
	slices.Sort(flags)
	return flags
}

// milliseconds parses ffprobe's seconds ("12.345000"); "N/A", empty or nonsense is zero.
func milliseconds(seconds string) int64 {
	value, err := strconv.ParseFloat(seconds, 64)
	if err != nil || math.IsNaN(value) || value < 0 || value > math.MaxInt64/1000 {
		return 0
	}
	return int64(math.Round(value * 1000))
}

// streamDuration is the stream's duration field, or, where the container keeps it only as a
// tag (Matroska's DURATION, "00:00:02.002000000"), that tag.
func streamDuration(stream rawStream) int64 {
	if duration := milliseconds(stream.Duration); duration != 0 {
		return duration
	}
	hours, rest, foundHours := strings.Cut(tag(stream.Tags, "duration"), ":")
	minutes, seconds, foundMinutes := strings.Cut(rest, ":")
	if !foundHours || !foundMinutes {
		return 0
	}
	return integer(hours)*3_600_000 + integer(minutes)*60_000 + milliseconds(seconds)
}

// integer parses ffprobe's integers-as-strings; "N/A", empty or nonsense is zero.
func integer(text string) int64 {
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil || value < 0 {
		return 0
	}
	return value
}

// bounded makes a string from the file valid UTF-8 and at most maxStringBytes long, cut at a
// character boundary.
func bounded(text string) string {
	text = strings.ToValidUTF8(text, "�")
	if len(text) <= maxStringBytes {
		return text
	}
	cut := maxStringBytes
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut]
}
