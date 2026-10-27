// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kstenerud/orpheum/internal/node/container/containertest"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/nodetest"
	"github.com/kstenerud/orpheum/internal/node/readthrough"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// fakeThroughReader answers with each track's stream index as its loudness, and the path it was
// given as the source's range, so a test sees which file was read; a picture's timeline, when asked,
// has one keyframe at its stream index's seconds. Stream unmeasurable fails, as FFmpeg does on a
// file it cannot decode.
type fakeThroughReader struct{}

// readHalfwayMs is how far into the film the fake reader says it is, once, as it starts.
const readHalfwayMs = 30_000

// unmeasurable is the stream index FFmpeg cannot measure.
const unmeasurable = 99

func (fakeThroughReader) Read(
	_ context.Context, film execx.Film, tracks []nodeproto.LoudnessTrack, timeline *nodeproto.VideoTimelineAsk,
	_ time.Duration, onProgress func(readThroughMs int64),
) (readthrough.Findings, error) {
	if onProgress != nil {
		onProgress(readHalfwayMs)
	}
	var findings readthrough.Findings
	if timeline != nil {
		findings.VideoTimeline = &nodeproto.VideoTimeline{
			KeyframesMs: []int64{int64(timeline.VideoStreamIndex) * 1000}, BytesInEachSecond: []int64{1},
		}
	}
	if len(tracks) == 0 {
		return findings, nil
	}
	if tracks[0].StreamIndex == stallingStreamIndex {
		return readthrough.Findings{}, fmt.Errorf("read the video through: %w", errStalled)
	}
	if tracks[0].StreamIndex == unmeasurable {
		return readthrough.Findings{}, errors.New("ffmpeg: Invalid data found when processing input")
	}
	measured := make([]nodeproto.TrackLoudness, 0, len(tracks))
	for _, track := range tracks {
		measured = append(measured, nodeproto.TrackLoudness{
			StreamIndex: track.StreamIndex,
			Stereo: nodeproto.Measurement{
				IntegratedLUFS: -float64(track.StreamIndex), RangeLU: float64(len(film.Path)),
			},
		})
	}
	findings.Loudness = measured
	return findings, nil
}

// jobReadThrough is a read-through's answer with the job id its result carried.
type jobReadThrough struct {
	JobID int64
	nodeproto.ReadThroughResult
}

func receiveReadThrough(t *testing.T, core *fakeCore) jobReadThrough {
	t.Helper()
	ctx, stop := context.WithTimeout(t.Context(), answerTimeout)
	defer stop()
	select {
	case result := <-core.readThroughs:
		return jobReadThrough{JobID: result.JobID, ReadThroughResult: *result.ReadThrough}
	case <-ctx.Done():
		t.Fatal("no read-through reached the core")
	}
	return jobReadThrough{}
}

// sendReadThrough sends the core's command to read a video through as job.
func (core *fakeCore) sendReadThrough(job int64, command nodeproto.ReadThrough) {
	core.commands <- nodeproto.Command{
		CommandSeq: job,
		JobID:      job,
		Work:       nodeproto.Work{Type: nodeproto.CommandReadThrough, ReadThrough: &command},
	}
}

func TestAgent_ReadsThroughOnlyTheVideoTheCoreNamed(t *testing.T) {
	t.Parallel()
	video, _, core := startExtractionAgent(t)
	measure := func(job int64, named nodeproto.BytesFile, streamIndex int) jobReadThrough {
		core.sendReadThrough(job, nodeproto.ReadThrough{
			Video: named, LoudnessTracks: []nodeproto.LoudnessTrack{{StreamIndex: streamIndex, ChannelCount: 6}},
		})
		return receiveReadThrough(t, core)
	}
	measured := measure(1, video, 1)
	wantPath := video.RootPath + "/film.mkv"
	if measured.JobID != 1 || measured.Refusal != "" || len(measured.Loudness) != 1 ||
		measured.Loudness[0].Stereo.IntegratedLUFS != -1 ||
		measured.Loudness[0].Stereo.RangeLU != float64(len(wantPath)) || measured.VideoTimeline != nil {
		t.Errorf("measuring the named video: %+v, want stream 1 of %s, and no timeline, which was not asked for",
			measured, wantPath)
	}
	changed := video
	changed.PayloadHash = strings.Repeat("0", 64)
	refused := measure(2, changed, 1)
	if refused.Refusal == "" || !strings.Contains(refused.Refusal, "not the one named") || refused.Failure != "" ||
		refused.RefusalCode != nodeproto.RefusedChanged {
		t.Errorf("measuring a changed video: %+v, want it refused, not failed (D100)", refused)
	}
	failed := measure(3, video, unmeasurable)
	if failed.Failure == "" || !strings.Contains(failed.Failure, "Invalid data") || failed.Refusal != "" {
		t.Errorf("a video FFmpeg cannot measure: %+v, want FFmpeg's failure, which is the file's", failed)
	}
}

// A run reads a video named under a library folder inside one of the node's roots, by its path there
// (D359 §4): a folder narrower than a configured root plays as the root did.
func TestAgent_ReadsThroughAVideoNamedUnderALibraryFolderInsideARoot(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	root, _ := startAgent(t, socket)
	folder := filepath.Join(root.Path(), "video", "movies")
	if err := os.MkdirAll(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "film.mkv"), []byte("a film"), 0o600); err != nil {
		t.Fatal(err)
	}
	video := nodeproto.BytesFile{
		RootPath: folder, Path: []byte("film.mkv"), PayloadHash: nodetest.PayloadHashHex(t, "a film"),
	}
	core.sendReadThrough(1, nodeproto.ReadThrough{
		Video: video, LoudnessTracks: []nodeproto.LoudnessTrack{{StreamIndex: 1, ChannelCount: 2}},
	})
	measured := receiveReadThrough(t, core)
	wantPath := folder + "/film.mkv"
	if measured.Refusal != "" || measured.Failure != "" || len(measured.Loudness) != 1 ||
		measured.Loudness[0].Stereo.RangeLU != float64(len(wantPath)) {
		t.Errorf("reading through a video under a library folder: %+v, want %s read", measured, wantPath)
	}
}

func TestAgent_AReadThroughAnswersEachPartItWasAskedFor(t *testing.T) {
	t.Parallel()
	video, _, core := startExtractionAgent(t)
	core.sendReadThrough(1, nodeproto.ReadThrough{
		Video: video, LoudnessTracks: []nodeproto.LoudnessTrack{{StreamIndex: 1, ChannelCount: 2}},
		VideoTimeline: &nodeproto.VideoTimelineAsk{VideoStreamIndex: 3},
	})
	both := receiveReadThrough(t, core)
	if len(both.Loudness) != 1 || both.VideoTimeline == nil ||
		!slices.Equal(both.VideoTimeline.KeyframesMs, []int64{3000}) {
		t.Errorf("a read-through asked for the loudness and stream 3's timeline answered %+v; want both", both)
	}
	core.sendReadThrough(2, nodeproto.ReadThrough{
		Video: video, VideoTimeline: &nodeproto.VideoTimelineAsk{VideoStreamIndex: 4},
	})
	timelineOnly := receiveReadThrough(t, core)
	if timelineOnly.Loudness != nil || timelineOnly.VideoTimeline == nil ||
		!slices.Equal(timelineOnly.VideoTimeline.KeyframesMs, []int64{4000}) {
		t.Errorf("a read-through asked for stream 4's timeline alone answered %+v; want only it", timelineOnly)
	}
}

// A read-through asked for a picture's timeline from the index alone, as a player waiting on its
// film's grid asks, is answered from the file's own index, its keyframes without its bytes in each
// second, and the film is not read through; a file with no index says so at once, read no further
// (D259 §2, D269 §2); and a timeline not asked from the index alone is read through, its bytes
// with it, though the file has an index.
func TestAgent_ATimelineAloneIsReadFromTheFilesIndex(t *testing.T) {
	t.Parallel()
	core, socket := startFakeCore(t)
	root, _ := startAgent(t, socket)
	contents := containertest.MatroskaWithCues(0, 2002, 4004)
	if err := os.WriteFile(filepath.Join(root.Path(), "indexed.mkv"), contents, 0o600); err != nil {
		t.Fatal(err)
	}
	indexed := nodeproto.BytesFile{
		RootPath: root.Path(), Path: []byte("indexed.mkv"), PayloadHash: nodetest.PayloadHashHex(t, string(contents)),
	}
	core.sendReadThrough(1, nodeproto.ReadThrough{
		Video: indexed, VideoTimeline: &nodeproto.VideoTimelineAsk{IsFromIndexOnly: true},
	})
	fromIndex := receiveReadThrough(t, core)
	if fromIndex.VideoTimeline == nil || !slices.Equal(fromIndex.VideoTimeline.KeyframesMs, []int64{0, 2002, 4004}) ||
		fromIndex.VideoTimeline.BytesInEachSecond != nil {
		t.Errorf("a timeline alone of an indexed film answered %+v; want its cues' keyframes, and no bytes", fromIndex)
	}
	if err := os.WriteFile(filepath.Join(root.Path(), "plain.avi"), []byte("an AVI"), 0o600); err != nil {
		t.Fatal(err)
	}
	plain := nodeproto.BytesFile{
		RootPath: root.Path(), Path: []byte("plain.avi"), PayloadHash: nodetest.PayloadHashHex(t, "an AVI"),
	}
	core.sendReadThrough(2, nodeproto.ReadThrough{
		Video: plain, VideoTimeline: &nodeproto.VideoTimelineAsk{VideoStreamIndex: 2, IsFromIndexOnly: true},
	})
	if noIndex := receiveReadThrough(t, core); !noIndex.HasNoIndex || noIndex.VideoTimeline != nil {
		t.Errorf("a timeline from the index alone of a film with no index answered %+v; want that it has none",
			noIndex)
	}
	core.sendReadThrough(3, nodeproto.ReadThrough{
		Video: indexed, VideoTimeline: &nodeproto.VideoTimelineAsk{VideoStreamIndex: 2},
	})
	readThrough := receiveReadThrough(t, core)
	if readThrough.VideoTimeline == nil || !slices.Equal(readThrough.VideoTimeline.KeyframesMs, []int64{2000}) {
		t.Errorf("a timeline of an indexed film, not from its index alone, answered %+v; want it read through",
			readThrough)
	}
}

// A running read-through tells the core how far into its film it is, as FFmpeg says, for how long
// the rest will take (D324 §6).
func TestAgent_ARunningReadThroughSaysHowFarIntoItsFilmItIs(t *testing.T) {
	t.Parallel()
	video, _, core := startExtractionAgent(t)
	core.sendReadThrough(1, nodeproto.ReadThrough{
		Video: video, LoudnessTracks: []nodeproto.LoudnessTrack{{StreamIndex: 1, ChannelCount: 2}}, DurationMs: 60_000,
	})
	for {
		select {
		case progress := <-core.progress:
			if progress.JobID != 1 || progress.ReadThroughMs == 0 {
				continue
			}
			if progress.State != nodeproto.ProgressRunning || progress.ReadThroughMs != readHalfwayMs {
				t.Errorf("the read's progress = %+v; want running, %d ms in", progress, readHalfwayMs)
			}
			receiveReadThrough(t, core)
			return
		case <-t.Context().Done():
			t.Fatal("the read-through never said how far it was")
		}
	}
}
