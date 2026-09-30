// SPDX-License-Identifier: GPL-3.0-or-later

package loudness

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kstenerud/orpheum/internal/node/confine"
	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/prepare"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// maxLogBytes is how much of FFmpeg's log a measurement keeps: its end, where each ebur128 filter
// writes a summary of about 400 bytes as it closes; enough for a file with dozens of tracks.
const maxLogBytes = 64 << 10

// Runner runs FFmpeg, watched for a stall, and returns the end of its log: execx.Runner.
type Runner interface {
	RunWatched(ctx context.Context, run execx.WatchedRun) (execx.Ran, error)
}

// measureStartGrace is how long a measurement's position may take to first move (execx.Watch):
// opening the video on a network mount and probing it.
const measureStartGrace = 2 * time.Minute

// measureStallWindow is how long a measurement's position may stand still. Every audio frame
// decoded moves it, so it stands still only while FFmpeg reads nothing; three minutes outlasts an
// NFS client's retries (a 60-second timeout, retransmitted twice by default).
const measureStallWindow = 3 * time.Minute

// Measurer measures loudness with the bundled FFmpeg.
type Measurer struct {
	runner Runner
	// slowestReadBytesPerSecond is the slowest the node reads a whole file (nodeproto.Capacity).
	slowestReadBytesPerSecond int64
}

// NewMeasurer returns the measurer, whose watchdog allows a measurement as long as reading its file
// at slowestReadBytesPerSecond takes.
func NewMeasurer(runner Runner, slowestReadBytesPerSecond int64) *Measurer {
	return &Measurer{runner: runner, slowestReadBytesPerSecond: slowestReadBytesPerSecond}
}

// The two sounds of a track a measurement names: as it is, and as a stereo device hears it.
const (
	mixMultichannel = "multichannel"
	mixStereo       = "stereo"
)

// Measure measures each track of the video at videoPath, an absolute path the caller has checked,
// in one read of the file: a track with more than two channels as it is and as the standard stereo
// mix a prepare makes of it, any other as stereo (D226 §1).
func (measurer *Measurer) Measure(
	ctx context.Context, videoPath string, tracks []nodeproto.LoudnessTrack,
) ([]nodeproto.TrackLoudness, error) {
	if len(tracks) == 0 {
		return nil, errors.New("no audio tracks to measure")
	}
	ran, err := measurer.runner.RunWatched(ctx, execx.WatchedRun{
		Access: confine.Access{Read: []string{videoPath}}, Arguments: arguments(videoPath, tracks),
		LogLimitBytes: maxLogBytes,
		Watch: execx.Watch{
			StartGrace: measureStartGrace, StallWindow: measureStallWindow,
			Deadline: execx.ReadingDeadline(videoPath, measurer.slowestReadBytesPerSecond),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("measure the loudness: %w", err)
	}
	summaries := parseSummaries(ran.Log)
	measured := make([]nodeproto.TrackLoudness, 0, len(tracks))
	for _, track := range tracks {
		stereo, found := summaries[filterInstanceName(mixStereo, track.StreamIndex)]
		if !found {
			return nil, fmt.Errorf("FFmpeg gave no loudness for stream %d", track.StreamIndex)
		}
		trackLoudness := nodeproto.TrackLoudness{StreamIndex: track.StreamIndex, Stereo: stereo}
		if track.ChannelCount > 2 {
			multichannel, isFound := summaries[filterInstanceName(mixMultichannel, track.StreamIndex)]
			if !isFound {
				return nil, fmt.Errorf("FFmpeg gave no loudness for stream %d as its multichannel sound",
					track.StreamIndex)
			}
			trackLoudness.Multichannel = &multichannel
		}
		measured = append(measured, trackLoudness)
	}
	return measured, nil
}

// filterInstanceName names one ebur128 filter, which its summary in the log carries: "stereo_3".
func filterInstanceName(sound string, streamIndex int) string {
	return sound + "_" + strconv.Itoa(streamIndex)
}

// arguments read the video as a local file only and send each sound through its own ebur128
// filter, measuring the true peak, into the null muxer.
func arguments(videoPath string, tracks []nodeproto.LoudnessTrack) []string {
	const ebur128Options = "=peak=true:framelog=quiet"
	var graph, maps []string
	for _, track := range tracks {
		input, stereo := "[0:"+strconv.Itoa(track.StreamIndex)+"]", filterInstanceName(mixStereo, track.StreamIndex)
		if track.ChannelCount > 2 {
			multichannel := filterInstanceName(mixMultichannel, track.StreamIndex)
			graph = append(graph,
				input+"asplit=2["+multichannel+"_in]["+stereo+"_in]",
				"["+multichannel+"_in]ebur128@"+multichannel+ebur128Options+"["+multichannel+"]",
				"["+stereo+"_in]"+prepare.StandardMix+",ebur128@"+stereo+ebur128Options+"["+stereo+"]")
			maps = append(maps, "-map", "["+multichannel+"]")
		} else {
			graph = append(graph, input+prepare.StereoAsIsMix+",ebur128@"+stereo+ebur128Options+"["+stereo+"]")
		}
		maps = append(maps, "-map", "["+stereo+"]")
	}
	// The summaries are logged at info, FFmpeg's own level; -nostats keeps progress out of the log.
	read := append(append([]string{"-nostats"}, execx.FileInput(videoPath)...),
		"-filter_complex", strings.Join(graph, ";"))
	return append(append(read, maps...), "-f", "null", "-")
}

// The lines of an ebur128 summary that a measurement reads. FFmpeg labels the true peak dBFS.
var (
	summaryStart      = regexp.MustCompile(`\[ebur128@(\w+) @ [^\]]*\] Summary:`)
	integratedPattern = regexp.MustCompile(`(?m)^\s+I:\s+(-?[0-9.]+) LUFS`)
	rangePattern      = regexp.MustCompile(`(?m)^\s+LRA:\s+(-?[0-9.]+) LU`)
	truePeakPattern   = regexp.MustCompile(`(?m)^\s+Peak:\s+(-?[0-9.]+|-inf) dBFS`)
)

// parseSummaries reads each ebur128 filter's summary from the log, by the filter's name. A
// summary missing a line is left out, and the caller says which track it lacks.
func parseSummaries(logged string) map[string]nodeproto.Measurement {
	summaries := map[string]nodeproto.Measurement{}
	starts := summaryStart.FindAllStringSubmatchIndex(logged, -1)
	for i, start := range starts {
		end := len(logged)
		if i+1 < len(starts) {
			end = starts[i+1][0]
		}
		name, block := logged[start[2]:start[3]], logged[start[1]:end]
		measurement, isComplete := parseSummary(block)
		if isComplete {
			summaries[name] = measurement
		}
	}
	return summaries
}

// parseSummary reads one summary: its integrated loudness, range and true peak.
func parseSummary(block string) (nodeproto.Measurement, bool) {
	integratedMatch, rangeMatch, peakMatch := integratedPattern.FindStringSubmatch(block),
		rangePattern.FindStringSubmatch(block), truePeakPattern.FindStringSubmatch(block)
	if integratedMatch == nil || rangeMatch == nil || peakMatch == nil {
		return nodeproto.Measurement{}, false
	}
	integratedLUFS, integratedErr := strconv.ParseFloat(integratedMatch[1], 64)
	rangeLU, rangeErr := strconv.ParseFloat(rangeMatch[1], 64)
	if integratedErr != nil || rangeErr != nil {
		return nodeproto.Measurement{}, false
	}
	measurement := nodeproto.Measurement{IntegratedLUFS: integratedLUFS, RangeLU: rangeLU}
	if peakMatch[1] != "-inf" {
		peak, err := strconv.ParseFloat(peakMatch[1], 64)
		if err != nil || math.IsInf(peak, 0) {
			return nodeproto.Measurement{}, false
		}
		measurement.TruePeakDBTP = &peak
	}
	return measurement, true
}
