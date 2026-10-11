// SPDX-License-Identifier: AGPL-3.0-or-later

package readthrough

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/kstenerud/orpheum/internal/node/prepare"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// maxLogBytes is how much of FFmpeg's log a read keeps: its end, where each ebur128 filter writes
// a summary of about 400 bytes as it closes; enough for a file with dozens of tracks.
const maxLogBytes = 64 << 10

// The two sounds of a track a measurement names: as it is, and as a stereo device hears it.
const (
	mixMultichannel = "multichannel"
	mixStereo       = "stereo"
)

// loudnessOf reads each track's measurements from the summaries in FFmpeg's log, and fails for a
// track FFmpeg said nothing of.
func loudnessOf(logged string, tracks []nodeproto.LoudnessTrack) ([]nodeproto.TrackLoudness, error) {
	summaries := parseSummaries(logged)
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

// loudnessOutput is the output that measures the tracks, none when there are none: each sound
// through its own ebur128 filter, measuring the true peak, into the null muxer. Each filter logs
// its summary at info, FFmpeg's own level, as it closes.
func loudnessOutput(tracks []nodeproto.LoudnessTrack) []string {
	if len(tracks) == 0 {
		return nil
	}
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
	output := append([]string{"-filter_complex", strings.Join(graph, ";")}, maps...)
	return append(output, "-f", "null", "-")
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
