// SPDX-License-Identifier: AGPL-3.0-or-later

package probe

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/testutil"
)

// recordedFFprobeOutput is ffprobe 8.1.2's output for a synthetic two-second Matroska file made by the
// pinned FFmpeg (never the owner's library, D203): H.264 video, German AAC titled
// "Deutsch (Stereo)", a forced English ASS track titled "Signs & Songs", an attached font and
// two chapters.
func recordedFFprobeOutput(t *testing.T) []byte {
	t.Helper()
	output, err := os.ReadFile("testdata/matroska-h264-aac-ass.json")
	if err != nil {
		t.Fatal(err)
	}
	return output
}

func TestParse_ARecordedMatroskaFile(t *testing.T) {
	t.Parallel()
	record, err := Parse(recordedFFprobeOutput(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Streams) != 4 {
		t.Fatalf("%d streams, want 4", len(record.Streams))
	}
	checks := map[string]func(*testing.T, nodeproto.Probe){
		"format":     checkRecordedFormat,
		"video":      checkRecordedVideo,
		"audio":      checkRecordedAudio,
		"subtitle":   checkRecordedSubtitle,
		"attachment": checkRecordedAttachment,
		"chapters":   checkRecordedChapters,
	}
	for name, check := range checks {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			check(t, record)
		})
	}
}

func checkRecordedFormat(t *testing.T, record nodeproto.Probe) {
	t.Helper()
	wantNames := []string{"matroska", "webm"}
	if !slices.Equal(record.Format.FormatNames, wantNames) || record.Format.DurationMs != 2023 ||
		record.Format.ContainerTitle != "Synthetic Film" {
		t.Errorf("format = %+v", record.Format)
	}
}

func checkRecordedVideo(t *testing.T, record nodeproto.Probe) {
	t.Helper()
	video := record.Streams[0]
	want := nodeproto.ProbeVideo{Width: 320, Height: 240, PixelFormat: "yuv420p", FrameRate: "24000/1001"}
	if video.Kind != "video" || video.Codec != "h264" || video.DurationMs != 2002 ||
		video.Video.Width != want.Width || video.Video.Height != want.Height ||
		video.Video.PixelFormat != want.PixelFormat || video.Video.FrameRate != want.FrameRate {
		t.Errorf("video = %+v", video)
	}
}

func checkRecordedAudio(t *testing.T, record nodeproto.Probe) {
	t.Helper()
	audio := record.Streams[1]
	if audio.Codec != "aac" || audio.ContainerLanguage != "ger" || audio.ContainerTitle != "Deutsch (Stereo)" ||
		audio.Audio.ChannelCount != 2 || audio.Audio.SampleRateHz != 44100 ||
		!slices.Contains(audio.Disposition, "default") {
		t.Errorf("audio = %+v", audio)
	}
}

func checkRecordedSubtitle(t *testing.T, record nodeproto.Probe) {
	t.Helper()
	subtitle := record.Streams[2]
	if subtitle.Codec != "ass" || subtitle.ContainerLanguage != "eng" || subtitle.ContainerTitle != "Signs & Songs" ||
		!slices.Contains(subtitle.Disposition, "forced") {
		t.Errorf("subtitle = %+v", subtitle)
	}
}

func checkRecordedAttachment(t *testing.T, record nodeproto.Probe) {
	t.Helper()
	font := record.Streams[3]
	want := nodeproto.ProbeAttachment{Filename: "Arial.ttf", MimeType: "application/x-truetype-font"}
	if font.Kind != "attachment" || font.Attachment != want {
		t.Errorf("attachment = %+v", font)
	}
}

func checkRecordedChapters(t *testing.T, record nodeproto.Probe) {
	t.Helper()
	want := []nodeproto.ProbeChapter{
		{StartMs: 0, EndMs: 1000, ContainerTitle: "Opening"},
		{StartMs: 1000, EndMs: 2000, ContainerTitle: "Ending"},
	}
	if !reflect.DeepEqual(record.Chapters, want) {
		t.Errorf("chapters = %+v, want %+v", record.Chapters, want)
	}
}

func TestParse_IsTheSameRecordEveryTime(t *testing.T) {
	t.Parallel()
	// Keys that differ only in case, and many flags, would expose any map-order dependence.
	output := []byte(`{"format":{"format_name":"matroska","tags":{"TITLE":"b","Title":"c","tItLe":"d"}},
		"streams":[{"index":0,"codec_type":"audio","disposition":{"original":1,"dub":1,"comment":1,
		"forced":1,"default":1,"karaoke":0},"tags":{"LANGUAGE":"deu","Language":"ger"}}]}`)
	first, err := Parse(output)
	if err != nil {
		t.Fatal(err)
	}
	for range 50 {
		again, _ := Parse(output) //nolint:errcheck // the first parse's error was checked
		if !reflect.DeepEqual(first, again) {
			t.Fatalf("two parses differ: %+v and %+v", first, again)
		}
	}
	if first.Format.ContainerTitle != "b" || first.Streams[0].ContainerLanguage != "deu" {
		t.Errorf("title %q, language %q: want the first key in sorted order",
			first.Format.ContainerTitle, first.Streams[0].ContainerLanguage)
	}
	if !slices.Equal(first.Streams[0].Disposition, []string{"comment", "default", "dub", "forced", "original"}) {
		t.Errorf("disposition = %q, want the set flags, sorted", first.Streams[0].Disposition)
	}
}

func TestParse_BoundsWhatAHostileFileCanMakeTheNodeReport(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("é", maxStringBytes) // two bytes each
	invalid := "Tit\xffle"                      // a raw byte that is not UTF-8, as a hostile container would carry
	output := []byte(`{"format":{"format_name":"matroska","tags":{"title":"` + long + `"}},
		"streams":[{"index":0,"codec_type":"audio","tags":{"title":"` + invalid + `"}}]}`)
	record, err := Parse(output)
	if err != nil {
		t.Fatal(err)
	}
	if title := record.Format.ContainerTitle; len(title) > maxStringBytes || !utf8.ValidString(title) || title == "" {
		t.Errorf("a %d-byte title became %d bytes, valid UTF-8 %v", len(long), len(title), utf8.ValidString(title))
	}
	if title := record.Streams[0].ContainerTitle; title != "Tit�le" {
		t.Errorf("an invalid title became %q, want the bad byte replaced", title)
	}

	var streams strings.Builder
	for index := range maxStreams + 1 {
		if index > 0 {
			streams.WriteString(",")
		}
		fmt.Fprintf(&streams, `{"index":%d,"codec_type":"data"}`, index)
	}
	_, err = Parse([]byte(`{"format":{},"streams":[` + streams.String() + `]}`))
	if !errors.Is(err, ErrTooLarge) {
		t.Errorf("%d streams: error = %v, want ErrTooLarge", maxStreams+1, err)
	}
	if _, err := Parse([]byte(`{"format":`)); err == nil {
		t.Error("truncated JSON parsed")
	}
}

func TestParse_ALanguageTagIsNormalisedBesideTheRawOne(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"ger": "de", "deu": "de", "fre": "fr", "chi": "zh", "dut": "nl", "eng": "en", "pt-BR": "pt-BR",
		"und": "", "": "", "xx": "", "zxx": "zxx",
	}
	for raw, want := range cases {
		output := fmt.Appendf(nil, `{"streams":[{"index":0,"codec_type":"audio","tags":{"language":%q}}]}`, raw)
		record, err := Parse(output)
		if err != nil {
			t.Fatal(err)
		}
		stream := record.Streams[0]
		if stream.ContainerLanguage != raw || stream.Language != want {
			t.Errorf("%q: language %q, tag %q; want the raw tag kept and %q beside it",
				raw, stream.ContainerLanguage, stream.Language, want)
		}
	}
}

func TestParse_NonsenseNumbersAreZero(t *testing.T) {
	t.Parallel()
	record, err := Parse([]byte(`{"format":{"duration":"N/A","bit_rate":"-5"},
		"streams":[{"index":0,"codec_type":"audio","sample_rate":"lots","duration":"NaN"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if record.Format.DurationMs != 0 || record.Format.BitRateBps != 0 || record.Streams[0].Audio.SampleRateHz != 0 ||
		record.Streams[0].DurationMs != 0 {
		t.Errorf("record = %+v, want zeros", record)
	}
	if record.Streams == nil || record.Chapters == nil || record.Format.FormatNames == nil {
		t.Error("an empty list is nil; the stored JSON would say null instead of []")
	}
}

func TestProbe_RunsFFprobeOnTheFileProtocolOnly(t *testing.T) {
	t.Parallel()
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{Stdout: string(recordedFFprobeOutput(t))})
	runner, err := execx.NewRunner(fake.Directory)
	if err != nil {
		t.Fatal(err)
	}
	prober := NewProber(runner, time.Minute)

	record, err := prober.Probe(t.Context(), execx.Film{Path: "/media/http:evil|concat.mkv"})
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Streams) != 4 {
		t.Errorf("%d streams, want the recorded 4", len(record.Streams))
	}
	call := fake.OnlyCall(t)
	if call.Tool != "ffprobe" || call.Arguments[len(call.Arguments)-1] != "file:/media/http:evil|concat.mkv" {
		t.Errorf("ran %s %q; want ffprobe with the input named by the file: protocol", call.Tool, call.Arguments)
	}
	whitelist := slices.Index(call.Arguments, "-protocol_whitelist")
	if whitelist < 0 || call.Arguments[whitelist+1] != "file" {
		t.Errorf("arguments %q do not allow only the file protocol", call.Arguments)
	}

	_, err = prober.Probe(t.Context(), execx.Film{Path: "relative/film.mkv"})
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Errorf("a relative path: error = %v, want a refusal", err)
	}
}

func TestProbe_AFailedProbeSaysWhy(t *testing.T) {
	t.Parallel()
	fake := testutil.InstallFakeFFmpeg(t, testutil.FakeReply{
		Stderr:     "file:/media/broken.mkv: Invalid data found when processing input\n",
		ExitStatus: 1,
	})
	runner, err := execx.NewRunner(fake.Directory)
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewProber(runner, time.Minute).Probe(t.Context(), execx.Film{Path: "/media/broken.mkv"})
	failure, isRunError := errors.AsType[*execx.RunError](err)
	if !isRunError || failure.ExitStatus != 1 || !strings.Contains(err.Error(), "Invalid data found") {
		t.Errorf("error = %v, want ffprobe's status and its reason", err)
	}
}
