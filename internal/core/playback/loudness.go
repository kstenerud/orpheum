// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// The level a sound is brought towards and the limits on its gain (D226 §3): EBU R128's target,
// chosen from the owner's films so typical ones change little; a true peak no higher than -1
// dBTP; and no sound raised more than 20 dB into its own noise.
const (
	targetLUFS      = -23.0
	peakCeilingDBTP = -1.0
	maxRaiseDB      = 20.0
)

// The mixes of a track library.db's loudness table names (measured_mix): the multichannel sound
// as it is, which a receiver plays, and the stereo a stereo device hears (D226 §1).
const (
	mixMultichannel = "multichannel"
	mixStereo       = "stereo"
)

// Levelling is what brings a sound to the level: the gain, once the sound is measured. A sound not
// measured yet plays as it is (D226 §6).
type Levelling struct {
	IsMeasured bool
	GainDB     float64
}

// soundKey names one measured sound of a file: a track, as its multichannel sound or as stereo.
type soundKey struct {
	streamIndex int64
	mix         string
}

// levellings are the gains that level each measured sound of the bytes with payloadHash.
func (service *Service) levellings(ctx context.Context, payloadHash []byte) (map[soundKey]Levelling, error) {
	rows, err := service.database.Read().ListLoudnessOfPayload(ctx, payloadHash)
	if err != nil {
		return nil, fmt.Errorf("read the loudness: %w", err)
	}
	levels := make(map[soundKey]Levelling, len(rows))
	for _, row := range rows {
		if level, isMeasured := levellingOf(row); isMeasured {
			levels[soundKey{row.StreamIndex, row.MeasuredMix}] = level
		}
	}
	return levels, nil
}

// levellingOf is the gain that brings a measured sound to the level, within its limits; a sound
// that could not be measured has none, and silence, which has no peak, is left as it is.
func levellingOf(row sqlcgen.Loudness) (Levelling, bool) {
	if row.Failure != nil || row.IntegratedLufs == nil {
		return Levelling{}, false
	}
	if row.TruePeakDbtp == nil {
		return Levelling{IsMeasured: true}, true
	}
	gain := min(targetLUFS-*row.IntegratedLufs, peakCeilingDBTP-*row.TruePeakDbtp, maxRaiseDB)
	return Levelling{IsMeasured: true, GainDB: math.Round(gain*10) / 10}, true
}

// directLevelling is the gain for a file played as it is: its main track's, as it is when it has
// more than two channels, which only a receiver is sent as it is (D222), else as stereo.
func directLevelling(levels map[soundKey]Levelling, probe nodeproto.Probe) Levelling {
	main, found := decide.MainAudio(probe)
	if !found {
		return Levelling{}
	}
	sound := mixStereo
	if main.Audio.ChannelCount > 2 {
		sound = mixMultichannel
	}
	return levels[soundKey{int64(main.StreamIndex), sound}]
}

// audioStreams are a file's audio tracks, in order: a track's place among them is how a prepare
// and a playback's audio renditions name it.
func audioStreams(probe nodeproto.Probe) []nodeproto.ProbeStream {
	var tracks []nodeproto.ProbeStream
	for _, stream := range probe.Streams {
		if stream.Kind == "audio" {
			tracks = append(tracks, stream)
		}
	}
	return tracks
}

// audioStreamIndexes are the stream indexes of a file's audio tracks, by their place among them.
func audioStreamIndexes(probe nodeproto.Probe) []int {
	var indexes []int
	for _, stream := range audioStreams(probe) {
		indexes = append(indexes, stream.StreamIndex)
	}
	return indexes
}

// levelAudio gives each audio rendition of a prepared playback its levelling, which is its track's
// stereo sound's, and the gain its sound carries, by its track's place in carried: what the
// rendition's making put in (D226 §4).
func levelAudio(renditions []AudioRendition, levels map[soundKey]Levelling, probe nodeproto.Probe, carried string) {
	indexes := audioStreamIndexes(probe)
	var gains []float64
	_ = json.Unmarshal([]byte(carried), &gains) //nolint:errcheck // a rendition's own record; unreadable is none
	for index := range renditions {
		rendition := &renditions[index]
		if rendition.Track < len(indexes) {
			rendition.Levelling = levels[soundKey{int64(indexes[rendition.Track]), mixStereo}]
		}
		if rendition.Track < len(gains) {
			rendition.CarriedGainDB = gains[rendition.Track]
		}
	}
}

// levelPrepare puts each audio track's levelling gain into a prepare's command, and returns them
// as its rendition records them (D226 §4). A track not measured yet carries none.
func levelPrepare(command *nodeproto.Prepare, levels map[soundKey]Levelling, probe nodeproto.Probe) string {
	indexes := audioStreamIndexes(probe)
	gains := make([]float64, len(command.Audio))
	for index := range command.Audio {
		track := &command.Audio[index]
		if track.AudioTrackIndex < len(indexes) {
			track.CarriedGainDB = levels[soundKey{int64(indexes[track.AudioTrackIndex]), mixStereo}].GainDB
			gains[index] = track.CarriedGainDB
		}
	}
	encoded, err := json.Marshal(gains)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

// sweepSpan is how many file ids a sweep for files to measure reads at a time.
const sweepSpan = 1000

// QueueMeasurements queues a measurement for every live video with sound whose bytes have no
// loudness yet (D226 §2), behind everything a person is waiting on, and returns how many it queued.
func (service *Service) QueueMeasurements(ctx context.Context) (int, error) {
	last, err := service.database.Read().GetMaxFileID(ctx)
	if err != nil {
		return 0, fmt.Errorf("read the last file id: %w", err)
	}
	queued := 0
	for after := int64(0); after < last; after += sweepSpan {
		ids, listErr := service.database.Read().ListFilesToMeasure(ctx, sqlcgen.ListFilesToMeasureParams{
			AfterID: after, ThroughID: after + sweepSpan,
		})
		if listErr != nil {
			return queued, fmt.Errorf("list files to measure: %w", listErr)
		}
		for _, id := range ids {
			added, enqueueErr := service.queue.Enqueue(ctx, jobs.New{
				Kind: jobs.KindMeasureLoudness, Target: jobs.Target{FileID: id},
				Reason: "measures how loud it is, so it plays at the level of the others",
			})
			if enqueueErr != nil {
				return queued, enqueueErr
			}
			if added {
				queued++
			}
		}
	}
	return queued, nil
}

// slowestReadBytesPerSecond is the slowest a node is expected to read a file it measures: a busy
// disk. A measurement says nothing until it ends, so its node is waited on for the silence the
// other jobs allow and the time to read the file at this rate.
const slowestReadBytesPerSecond = 20 << 20

// HandleLoudness runs one measurement job (jobs.KindMeasureLoudness): the node that holds the file
// measures its audio tracks, and this waits for its answer and keeps it by the file's payload
// hash. A file whose bytes are already measured, as a second copy's are, is not measured again,
// and FFmpeg's failure is kept and not retried.
func (service *Service) HandleLoudness(ctx context.Context, job jobs.Job) error {
	file, err := service.file(ctx, job.FileID)
	if errors.Is(err, ErrNoFile) {
		return jobs.Permanent(err)
	}
	if err != nil {
		return err
	}
	levels, err := service.database.Read().ListLoudnessOfPayload(ctx, file.PayloadHash)
	if err != nil {
		return fmt.Errorf("read the loudness of file %d: %w", file.ID, err)
	}
	if len(levels) > 0 {
		return nil
	}
	probe, err := decodeProbe(file.ProbeJson)
	if err != nil {
		return jobs.Permanent(err)
	}
	command := nodeproto.MeasureLoudness{JobID: job.ID}
	for _, stream := range audioStreams(probe) {
		command.Tracks = append(command.Tracks, nodeproto.LoudnessTrack{
			StreamIndex: stream.StreamIndex, ChannelCount: stream.Audio.ChannelCount,
		})
	}
	if len(command.Tracks) == 0 {
		return nil
	}
	if command.Video, err = service.source(ctx, locatedOf(file)); err != nil {
		return jobs.Permanent(err)
	}
	reading := time.Duration(ptr.ValueOrZero(file.SizeBytes)/slowestReadBytesPerSecond) * time.Second
	patience := service.maxSilence + reading
	return service.measure(ctx, job, file, command, patience)
}

// measure has the node measure, waits for its answer, and keeps it.
func (service *Service) measure(
	ctx context.Context, job jobs.Job, file sqlcgen.GetPlayableFileRow, command nodeproto.MeasureLoudness,
	patience time.Duration,
) error {
	answers := service.loudness.await(job.ID)
	defer service.loudness.forget(job.ID)
	service.nodes.Enqueue(file.NodeID, nodeproto.Command{
		Type: nodeproto.CommandMeasureLoudness, MeasureLoudness: &command,
	})
	timer := time.NewTimer(patience)
	defer timer.Stop()
	select {
	case answer := <-answers:
		if answer.Refusal != "" {
			// Nothing is kept: the next sweep asks again, of the file as it is then.
			return jobs.Permanent(errors.New(answer.Refusal))
		}
		keepErr := service.keepLoudness(ctx, file.PayloadHash, command.Tracks, answer)
		if answer.Failure != "" {
			return jobs.Permanent(errors.Join(errors.New(answer.Failure), keepErr))
		}
		return keepErr
	case <-timer.C:
		return fmt.Errorf("node %d said nothing about measuring file %d for %s", file.NodeID, file.ID, patience)
	case <-ctx.Done():
		if errors.Is(context.Cause(ctx), jobs.ErrCancelled) {
			service.nodes.Enqueue(file.NodeID, cancelCommand(job.ID))
			return jobs.Permanent(jobs.ErrCancelled)
		}
		return fmt.Errorf("wait for measurement job %d: %w", job.ID, ctx.Err())
	}
}

// keepLoudness keeps a node's answer by the file's payload hash: each track's sounds, or, when
// FFmpeg could not measure the file, its words, for each track as stereo, so the sweep does not
// ask again.
func (service *Service) keepLoudness(
	ctx context.Context, payloadHash []byte, tracks []nodeproto.LoudnessTrack, answer nodeproto.LoudnessResult,
) error {
	now := service.clock.Now().UnixMilli()
	var rows []sqlcgen.UpsertLoudnessParams
	if answer.Failure != "" {
		for _, track := range tracks {
			rows = append(rows, sqlcgen.UpsertLoudnessParams{
				PayloadHash: payloadHash, StreamIndex: int64(track.StreamIndex), MeasuredMix: mixStereo,
				Failure: &answer.Failure, MeasuredAt: now,
			})
		}
	}
	for _, track := range answer.Tracks {
		rows = append(rows, loudnessRow(payloadHash, track.StreamIndex, mixStereo, track.Stereo, now))
		if track.Multichannel != nil {
			rows = append(rows, loudnessRow(payloadHash, track.StreamIndex, mixMultichannel, *track.Multichannel, now))
		}
	}
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		for _, row := range rows {
			if err := queries.UpsertLoudness(ctx, row); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("keep the loudness: %w", err)
	}
	return nil
}

func loudnessRow(
	payloadHash []byte, streamIndex int, sound string, measured nodeproto.Measurement, now int64,
) sqlcgen.UpsertLoudnessParams {
	return sqlcgen.UpsertLoudnessParams{
		PayloadHash: payloadHash, StreamIndex: int64(streamIndex), MeasuredMix: sound,
		IntegratedLufs: &measured.IntegratedLUFS, RangeLu: &measured.RangeLU, TruePeakDbtp: measured.TruePeakDBTP,
		MeasuredAt: now,
	}
}

// Measured takes a node's answer to a measure_loudness command to the job waiting for it.
func (service *Service) Measured(ctx context.Context, nodeID int64, answer nodeproto.LoudnessResult) {
	service.loudness.deliver(ctx, nodeID, answer.JobID, answer)
}
