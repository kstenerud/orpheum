// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/nodetest"
	"github.com/kstenerud/orpheum/internal/node/subtitle"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// fakeSubtitler answers with text that names what it was given.
type fakeSubtitler struct{}

// An extraction asking for track slowStreamIndex runs until it is stopped, and one asking for
// stallingStreamIndex stalls; track unconvertibleStreamIndex fails alone, as FFmpeg would.
func (fakeSubtitler) ExtractTracks(
	ctx context.Context, film execx.Film, streamIndexes []int,
) ([]subtitle.TrackText, error) {
	if slices.Contains(streamIndexes, slowStreamIndex) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if slices.Contains(streamIndexes, stallingStreamIndex) {
		return nil, fmt.Errorf("extract streams %v as ASS: %w", streamIndexes, errStalled)
	}
	tracks := make([]subtitle.TrackText, 0, len(streamIndexes))
	for _, streamIndex := range streamIndexes {
		track := subtitle.TrackText{
			StreamIndex: streamIndex,
			Text:        subtitle.Text{ASS: []byte(film.Path), VTT: []byte(strconv.Itoa(streamIndex))},
		}
		if streamIndex == unconvertibleStreamIndex {
			track = subtitle.TrackText{
				StreamIndex: streamIndex, Err: errors.New("ffmpeg exited 1: Subtitle codec 94213 is not supported"),
			}
		}
		tracks = append(tracks, track)
	}
	return tracks, nil
}

// slowStreamIndex is the stream index whose extraction runs until it is stopped, and
// unconvertibleStreamIndex the one FFmpeg cannot convert.
const (
	slowStreamIndex          = 99
	unconvertibleStreamIndex = 98
)

func (fakeSubtitler) ConvertText(_ context.Context, source []byte) (subtitle.Text, error) {
	return subtitle.Text{ASS: source, VTT: source}, nil
}

// jobSubtitleText is a subtitle's text with the job id its result carried.
type jobSubtitleText struct {
	JobID int64
	nodeproto.SubtitleText
}

func receiveSubtitle(t *testing.T, core *fakeCore) jobSubtitleText {
	t.Helper()
	ctx, stop := context.WithTimeout(t.Context(), answerTimeout)
	defer stop()
	select {
	case result := <-core.subtitles:
		return jobSubtitleText{JobID: result.JobID, SubtitleText: *result.Subtitle}
	case <-ctx.Done():
		t.Fatal("no subtitle reached the core")
	}
	return jobSubtitleText{}
}

// startExtractionAgent starts an agent whose root holds film.mkv and film.en.srt, and returns them named
// by root, path and payload hash, and the core it talks to.
func startExtractionAgent(t *testing.T) (video, sidecar nodeproto.BytesFile, core *fakeCore) {
	t.Helper()
	core, socket := startFakeCore(t)
	root, _ := startAgent(t, socket)
	named := map[string]nodeproto.BytesFile{}
	for name, contents := range map[string]string{"film.mkv": "a film", "film.en.srt": "the sidecar"} {
		if err := os.WriteFile(filepath.Join(root.Path(), name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		named[name] = nodeproto.BytesFile{
			RootPath: root.Path(), Path: []byte(name), PayloadHash: nodetest.PayloadHashHex(t, contents),
		}
	}
	return named["film.mkv"], named["film.en.srt"], core
}

// queueExtraction sends an extraction as job, without waiting for the answer.
func (core *fakeCore) queueExtraction(job int64, extract nodeproto.ExtractSubtitle) {
	core.commands <- nodeproto.Command{
		CommandSeq: job,
		JobID:      job,
		Work:       nodeproto.Work{Type: nodeproto.CommandExtractSubtitle, ExtractSubtitle: &extract},
	}
}

func TestAgent_ExtractsATrackOnlyFromTheVideoTheCoreNamedAndASidecarUnderItsRoot(t *testing.T) {
	t.Parallel()
	video, sidecar, core := startExtractionAgent(t)
	send := func(job int64, extract nodeproto.ExtractSubtitle) jobSubtitleText {
		core.queueExtraction(job, extract)
		return receiveSubtitle(t, core)
	}
	track := send(1, nodeproto.ExtractSubtitle{Video: &video, StreamIndexes: []int{2}})
	if track.JobID != 1 || track.Refusal != "" || len(track.Tracks) != 1 || track.Tracks[0].StreamIndex != 2 ||
		string(track.Tracks[0].ASS) != video.RootPath+"/film.mkv" || string(track.Tracks[0].VTT) != "2" {
		t.Errorf("a track of the named video: %+v", track)
	}
	converted := send(2, nodeproto.ExtractSubtitle{Sidecar: &sidecar})
	if converted.Refusal != "" || string(converted.ASS) != "the sidecar" {
		t.Errorf("a sidecar: %+v", converted)
	}
	changedVideo, changedSidecar, outside, escaping, directory := video, sidecar, sidecar, sidecar, sidecar
	changedVideo.PayloadHash = strings.Repeat("0", 64)
	changedSidecar.PayloadHash = strings.Repeat("0", 64)
	outside.RootPath = t.TempDir()
	escaping.Path = []byte("../film.en.srt")
	if err := os.Mkdir(filepath.Join(sidecar.RootPath, "Subs"), 0o700); err != nil {
		t.Fatal(err)
	}
	directory.Path = []byte("Subs")
	for index, refusal := range []struct {
		what    string
		command nodeproto.ExtractSubtitle
		why     string
	}{
		{
			"a track of a changed video",
			nodeproto.ExtractSubtitle{Video: &changedVideo, StreamIndexes: []int{2}},
			"not the one named",
		},
		{"a changed sidecar", nodeproto.ExtractSubtitle{Sidecar: &changedSidecar}, "not the one named"},
		{
			"a sidecar under another root",
			nodeproto.ExtractSubtitle{Sidecar: &outside},
			"none of this node's roots",
		},
		{"a sidecar path leaving its root", nodeproto.ExtractSubtitle{Sidecar: &escaping}, ""},
		{"a sidecar that is a directory", nodeproto.ExtractSubtitle{Sidecar: &directory}, "not a regular file"},
		{"a track and a sidecar at once", nodeproto.ExtractSubtitle{Video: &video, Sidecar: &sidecar}, "only one"},
		{"a video and no track", nodeproto.ExtractSubtitle{Video: &video}, "names no track"},
	} {
		answer := send(int64(index+3), refusal.command)
		if answer.Refusal == "" || !strings.Contains(answer.Refusal, refusal.why) || answer.Failure != "" {
			t.Errorf("%s: %+v, want refused: %s", refusal.what, answer, refusal.why)
		}
		isChanged := refusal.why == "not the one named"
		if isChanged != (answer.RefusalCode == nodeproto.RefusedChanged) {
			t.Errorf("%s: refused with code %q; want it named changed since the scan only for a changed file",
				refusal.what, answer.RefusalCode)
		}
	}
}

func TestAgent_AnswersEachTrackOfAVideoWithItsTextOrItsOwnFailure(t *testing.T) {
	t.Parallel()
	video, _, core := startExtractionAgent(t)
	core.queueExtraction(1, nodeproto.ExtractSubtitle{Video: &video, StreamIndexes: []int{2, unconvertibleStreamIndex}})
	failed := receiveSubtitle(t, core)
	if failed.Failure != "" || failed.Refusal != "" || len(failed.Tracks) != 2 ||
		!strings.Contains(failed.Tracks[1].Failure, "not supported") || failed.Tracks[1].ASS != nil ||
		failed.Tracks[0].Failure != "" || string(failed.Tracks[0].VTT) != "2" {
		t.Errorf("a track FFmpeg cannot convert beside one it can: %+v, want that track's failure in FFmpeg's "+
			"words and the other's text, not a refusal (D340)", failed)
	}
}

func TestAgent_ACancelStopsARunningExtractionAndNothingIsReportedForIt(t *testing.T) {
	t.Parallel()
	video, _, core := startExtractionAgent(t)
	// One extraction runs at a time, so the next runs only once the cancel frees the slot.
	core.queueExtraction(1, nodeproto.ExtractSubtitle{Video: &video, StreamIndexes: []int{slowStreamIndex}})
	core.commands <- nodeproto.Command{CommandSeq: 2, JobID: 1, Work: nodeproto.Work{Type: nodeproto.CommandCancel}}
	core.queueExtraction(3, nodeproto.ExtractSubtitle{Video: &video, StreamIndexes: []int{2}})
	if next := receiveSubtitle(t, core); next.JobID != 3 || next.Refusal != "" {
		t.Errorf("the first answer = %+v, want job 3's text and nothing for the cancelled job 1", next)
	}
}

func TestAgent_ConvertsAFetchedSubtitlesTextAndRefusesItBesideAFile(t *testing.T) {
	t.Parallel()
	video, _, core := startExtractionAgent(t)
	fetched := []byte("1\n00:00:01,000 --> 00:00:02,000\nHallo\n")
	core.queueExtraction(1, nodeproto.ExtractSubtitle{FetchedSubRip: fetched})
	converted := receiveSubtitle(t, core)
	if converted.Refusal != "" || !strings.Contains(string(converted.ASS), "Hallo") {
		t.Errorf("a fetched subtitle: %+v, want its text converted as a sidecar's is (D224 §5)", converted)
	}
	core.queueExtraction(2, nodeproto.ExtractSubtitle{Video: &video, FetchedSubRip: []byte("text")})
	if both := receiveSubtitle(t, core); !strings.Contains(both.Refusal, "only one") {
		t.Errorf("a fetched text beside a video: %+v, want refused", both)
	}
}

// A subtitle's answer the core did not take is posted again, not dropped.
func TestAgent_ASubtitleAnswerTheCoreRefusedOnceArrivesOnRetry(t *testing.T) {
	t.Parallel()
	_, sidecar, core := startExtractionAgent(t)
	core.refuseFirstPosts(subtitleResult, 1)
	core.queueExtraction(1, nodeproto.ExtractSubtitle{Sidecar: &sidecar})
	if answer := receiveSubtitle(t, core); answer.JobID != 1 || string(answer.ASS) != "the sidecar" {
		t.Errorf("the answer after one refusal = %+v, want job 1's text", answer)
	}
}
