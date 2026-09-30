// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/nodetest"
	"github.com/kstenerud/orpheum/internal/node/subtitle"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// fakeSubtitler answers with text that names what it was given.
type fakeSubtitler struct{}

// Track slowStreamIndex runs until it is stopped, and track unconvertibleStreamIndex fails as FFmpeg would.
func (fakeSubtitler) ExtractTrack(ctx context.Context, videoPath string, streamIndex int) (subtitle.Text, error) {
	if streamIndex == slowStreamIndex {
		<-ctx.Done()
		return subtitle.Text{}, ctx.Err()
	}
	if streamIndex == stallingStreamIndex {
		return subtitle.Text{}, fmt.Errorf("extract stream %d as ASS: %w", streamIndex, errStalled)
	}
	if streamIndex == unconvertibleStreamIndex {
		return subtitle.Text{}, errors.New("ffmpeg exited 1: Subtitle codec 94213 is not supported")
	}
	return subtitle.Text{ASS: []byte(videoPath), VTT: []byte(strconv.Itoa(streamIndex))}, nil
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

func receiveSubtitle(t *testing.T, core *fakeCore) nodeproto.SubtitleText {
	t.Helper()
	ctx, stop := context.WithTimeout(t.Context(), answerTimeout)
	defer stop()
	select {
	case answer := <-core.subtitles:
		return answer
	case <-ctx.Done():
		t.Fatal("no subtitle reached the core")
	}
	return nodeproto.SubtitleText{}
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
	extract.JobID = job
	core.commands <- nodeproto.Command{
		CommandSeq: job, Type: nodeproto.CommandExtractSubtitle,
		ExtractSubtitle: &extract,
	}
}

func TestAgent_ExtractsATrackOnlyFromTheVideoTheCoreNamedAndASidecarUnderItsRoot(t *testing.T) {
	t.Parallel()
	video, sidecar, core := startExtractionAgent(t)
	send := func(job int64, extract nodeproto.ExtractSubtitle) nodeproto.SubtitleText {
		core.queueExtraction(job, extract)
		return receiveSubtitle(t, core)
	}
	track := send(1, nodeproto.ExtractSubtitle{Video: &video, StreamIndex: 2})
	if track.JobID != 1 || track.Refusal != "" || string(track.ASS) != video.RootPath+"/film.mkv" ||
		string(track.VTT) != "2" {
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
			nodeproto.ExtractSubtitle{Video: &changedVideo, StreamIndex: 2},
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
	} {
		answer := send(int64(index+3), refusal.command)
		if answer.Refusal == "" || !strings.Contains(answer.Refusal, refusal.why) || answer.Failure != "" {
			t.Errorf("%s: %+v, want refused: %s", refusal.what, answer, refusal.why)
		}
	}
	failed := send(9, nodeproto.ExtractSubtitle{Video: &video, StreamIndex: unconvertibleStreamIndex})
	if !strings.Contains(failed.Failure, "not supported") || failed.Refusal != "" {
		t.Errorf("a track FFmpeg cannot convert: %+v, want a failure in FFmpeg's words, not a refusal", failed)
	}
}

func TestAgent_ACancelStopsARunningExtractionAndNothingIsReportedForIt(t *testing.T) {
	t.Parallel()
	video, _, core := startExtractionAgent(t)
	// One extraction runs at a time, so the next runs only once the cancel frees the slot.
	core.queueExtraction(1, nodeproto.ExtractSubtitle{Video: &video, StreamIndex: slowStreamIndex})
	core.commands <- nodeproto.Command{
		CommandSeq: 2, Type: nodeproto.CommandCancel,
		Cancel: &nodeproto.Cancel{JobID: 1},
	}
	core.queueExtraction(3, nodeproto.ExtractSubtitle{Video: &video, StreamIndex: 2})
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
	core.refuseFirstPosts(nodeproto.PathSubtitle, 1)
	core.queueExtraction(1, nodeproto.ExtractSubtitle{Sidecar: &sidecar})
	if answer := receiveSubtitle(t, core); answer.JobID != 1 || string(answer.ASS) != "the sidecar" {
		t.Errorf("the answer after one refusal = %+v, want job 1's text", answer)
	}
}
