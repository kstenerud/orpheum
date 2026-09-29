// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kstenerud/orpheum/internal/node/hash"
	"github.com/kstenerud/orpheum/internal/node/subtitle"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// fakeSubtitles answers with text that names what it was given.
type fakeSubtitles struct{}

// Track slowTrack runs until it is stopped.
func (fakeSubtitles) ExtractTrack(ctx context.Context, videoPath string, streamIndex int) (subtitle.Text, error) {
	if streamIndex == slowTrack {
		<-ctx.Done()
		return subtitle.Text{}, ctx.Err()
	}
	return subtitle.Text{ASS: []byte(videoPath), VTT: []byte(strconv.Itoa(streamIndex))}, nil
}

// slowTrack is the stream index whose extraction runs until it is stopped.
const slowTrack = 99

func (fakeSubtitles) ConvertFile(_ context.Context, source []byte) (subtitle.Text, error) {
	return subtitle.Text{ASS: source, VTT: source}, nil
}

func receiveSubtitle(t *testing.T, core *fakeCore) nodeproto.SubtitleText {
	t.Helper()
	ctx, stop := context.WithTimeout(t.Context(), patience)
	defer stop()
	select {
	case answer := <-core.subtitles:
		return answer
	case <-ctx.Done():
		t.Fatal("no subtitle reached the core")
	}
	return nodeproto.SubtitleText{}
}

// extracting starts an agent whose root holds film.mkv and film.en.srt, and returns them named
// by root, path and payload hash, and the core it talks to.
func extracting(t *testing.T) (video, sidecar nodeproto.BytesFile, core *fakeCore) {
	t.Helper()
	core, socket := startFakeCore(t)
	root, _ := running(t, socket)
	named := map[string]nodeproto.BytesFile{}
	for name, contents := range map[string]string{"film.mkv": "a film", "film.en.srt": "the sidecar"} {
		if err := os.WriteFile(filepath.Join(root.Path(), name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		identity, err := hash.Payload(strings.NewReader(contents), int64(len(contents)))
		if err != nil {
			t.Fatal(err)
		}
		named[name] = nodeproto.BytesFile{
			Root: root.Path(), Path: []byte(name), PayloadHash: hex.EncodeToString(identity.Hash[:]),
		}
	}
	return named["film.mkv"], named["film.en.srt"], core
}

// extract sends an extraction as job, without waiting for the answer.
func (core *fakeCore) extract(job int64, extract nodeproto.ExtractSubtitle) {
	extract.JobID = job
	core.commands <- nodeproto.Command{Seq: job, Type: nodeproto.CommandExtractSubtitle, ExtractSubtitle: &extract}
}

func TestAgent_ExtractsATrackOnlyFromTheVideoTheCoreNamedAndASidecarUnderItsRoot(t *testing.T) {
	t.Parallel()
	video, sidecar, core := extracting(t)
	send := func(job int64, extract nodeproto.ExtractSubtitle) nodeproto.SubtitleText {
		core.extract(job, extract)
		return receiveSubtitle(t, core)
	}
	track := send(1, nodeproto.ExtractSubtitle{Video: &video, StreamIndex: 2})
	if track.JobID != 1 || track.Refusal != "" || string(track.ASS) != video.Root+"/film.mkv" ||
		string(track.VTT) != "2" {
		t.Errorf("a track of the named video: %+v", track)
	}
	converted := send(2, nodeproto.ExtractSubtitle{Sidecar: &sidecar})
	if converted.Refusal != "" || string(converted.ASS) != "the sidecar" {
		t.Errorf("a sidecar: %+v", converted)
	}
	changedVideo, changedSidecar, outside, escaping := video, sidecar, sidecar, sidecar
	changedVideo.PayloadHash = strings.Repeat("0", 64)
	changedSidecar.PayloadHash = strings.Repeat("0", 64)
	outside.Root = t.TempDir()
	escaping.Path = []byte("../film.en.srt")
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
		{"a track and a sidecar at once", nodeproto.ExtractSubtitle{Video: &video, Sidecar: &sidecar}, "only one"},
	} {
		answer := send(int64(index+3), refusal.command)
		if answer.Refusal == "" || !strings.Contains(answer.Refusal, refusal.why) {
			t.Errorf("%s: %+v, want refused: %s", refusal.what, answer, refusal.why)
		}
	}
}

func TestAgent_ACancelStopsARunningExtractionAndNothingIsReportedForIt(t *testing.T) {
	t.Parallel()
	video, _, core := extracting(t)
	// One extraction runs at a time, so the next runs only once the cancel frees the slot.
	core.extract(1, nodeproto.ExtractSubtitle{Video: &video, StreamIndex: slowTrack})
	core.commands <- nodeproto.Command{Seq: 2, Type: nodeproto.CommandCancel, Cancel: &nodeproto.Cancel{JobID: 1}}
	core.extract(3, nodeproto.ExtractSubtitle{Video: &video, StreamIndex: 2})
	if next := receiveSubtitle(t, core); next.JobID != 3 || next.Refusal != "" {
		t.Errorf("the first answer = %+v, want job 3's text and nothing for the cancelled job 1", next)
	}
}

func TestAgent_ConvertsAFetchedSubtitlesTextAndRefusesItBesideAFile(t *testing.T) {
	t.Parallel()
	video, _, core := extracting(t)
	core.extract(1, nodeproto.ExtractSubtitle{FetchedSubRip: []byte("1\n00:00:01,000 --> 00:00:02,000\nHallo\n")})
	converted := receiveSubtitle(t, core)
	if converted.Refusal != "" || !strings.Contains(string(converted.ASS), "Hallo") {
		t.Errorf("a fetched subtitle: %+v, want its text converted as a sidecar's is (D224 §5)", converted)
	}
	core.extract(2, nodeproto.ExtractSubtitle{Video: &video, FetchedSubRip: []byte("text")})
	if both := receiveSubtitle(t, core); !strings.Contains(both.Refusal, "only one") {
		t.Errorf("a fetched text beside a video: %+v, want refused", both)
	}
}
