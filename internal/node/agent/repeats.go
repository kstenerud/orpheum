// SPDX-License-Identifier: AGPL-3.0-or-later

package agent

import (
	"context"
	"errors"
	"time"

	"github.com/kstenerud/orpheum/internal/node/execx"
	"github.com/kstenerud/orpheum/internal/node/fingerprint"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// SoundFingerprinter fingerprints a stretch of a video's sound: fingerprint.Fingerprinter.
type SoundFingerprinter interface {
	Fingerprint(
		ctx context.Context, film execx.Film, audioStreamIndex int, from, length time.Duration,
	) ([]uint32, error)
}

// findRepeatsWork fingerprints the opening and closing minutes of each video of a set the core named,
// a season's episodes, one after another in the node's read-through slot, and answers the stretch of
// each that the others share (D101, D115).
func (agent *Agent) findRepeatsWork(
	ctx context.Context, nodeID int64, held attempt, command nodeproto.FindRepeats,
) heldWork {
	return agent.answeringWork(ctx, nodeID, answeringJob{
		attempt: held, jobKind: "a season's repeats",
		stalledAnswer: func(stall nodeproto.Stall) nodeproto.Result {
			return nodeproto.Result{Repeats: &nodeproto.Repeats{Stall: &stall}}
		},
		makeAnswer: func(jobCtx context.Context) (nodeproto.Result, string) {
			answer := agent.findRepeats(jobCtx, held, command)
			return nodeproto.Result{Repeats: &answer}, answer.Refusal + stallDetail(answer.Stall)
		},
	})
}

// findRepeats fingerprints each video checked to be the one the core named (D100), its opening and its
// closing, and compares those it could with each other: a video refused or failed is said so and
// compared with none. A stall anywhere answers the set as stalled, to be asked again.
func (agent *Agent) findRepeats(
	ctx context.Context, held attempt, command nodeproto.FindRepeats,
) nodeproto.Repeats {
	if agent.workers.SoundFingerprinter == nil {
		return nodeproto.Repeats{Refusal: "this node does not fingerprint sound"}
	}
	answers := make([]nodeproto.VideoRepeats, len(command.Videos))
	for index, video := range command.Videos {
		answers[index].FileID = video.FileID
	}
	var openings, closings [][]uint32
	var heard []int // the index in command.Videos of each fingerprinted video
	var closingStarts []time.Duration
	for index, video := range command.Videos {
		if err := agent.work.waitWhilePaused(ctx, held.token); err != nil {
			return nodeproto.Repeats{Refusal: err.Error()}
		}
		ends, err := agent.fingerprintEnds(ctx, video, command)
		if stall := stallOf(err); stall != nil {
			return nodeproto.Repeats{Stall: stall}
		}
		var refused refusedVideoError
		switch {
		case errors.As(err, &refused):
			answers[index].Refusal, answers[index].RefusalCode = refused.err.Error(), refusalCodeOf(refused.err)
			continue
		case err != nil:
			answers[index].Failure = err.Error()
			continue
		}
		openings, closings = append(openings, ends.opening), append(closings, ends.closing)
		heard, closingStarts = append(heard, index), append(closingStarts, ends.closingStart)
	}
	openingRepeats, closingRepeats := fingerprint.Repeats(openings), fingerprint.Repeats(closings)
	for at, index := range heard {
		answers[index].Opening = repeatedSpan(openingRepeats[at], 0, len(heard)-1)
		answers[index].Closing = repeatedSpan(closingRepeats[at], closingStarts[at], len(heard)-1)
	}
	return nodeproto.Repeats{Videos: answers}
}

// refusedVideoError is a video the node did not read: changed since the scan, or gone.
type refusedVideoError struct{ err error }

func (refused refusedVideoError) Error() string { return refused.err.Error() }

// videoEnds are the fingerprints of a video's opening and closing, and where its closing starts.
type videoEnds struct {
	opening, closing []uint32
	closingStart     time.Duration
}

// fingerprintEnds fingerprints a video's opening and closing.
func (agent *Agent) fingerprintEnds(
	ctx context.Context, video nodeproto.RepeatVideo, command nodeproto.FindRepeats,
) (videoEnds, error) {
	film, closeFilm, err := agent.openFilm(ctx, video.Video)
	if stall := stallOf(err); stall != nil {
		return videoEnds{}, err
	}
	if err != nil {
		return videoEnds{}, refusedVideoError{err: err}
	}
	defer closeFilm()
	duration := time.Duration(video.DurationMs) * time.Millisecond
	openingLength := min(time.Duration(command.OpeningMs)*time.Millisecond, duration)
	closingLength := min(time.Duration(command.ClosingMs)*time.Millisecond, duration)
	ends := videoEnds{closingStart: duration - closingLength}
	fingerprinter := agent.workers.SoundFingerprinter
	ends.opening, err = fingerprinter.Fingerprint(ctx, film, video.AudioStreamIndex, 0, openingLength)
	if err == nil {
		ends.closing, err = fingerprinter.Fingerprint(ctx, film, video.AudioStreamIndex, ends.closingStart,
			closingLength)
	}
	return ends, err
}

// repeatedSpan is a repeat as the answer says it, in the video's time from where its fingerprint
// started; nil when no other video shares it.
func repeatedSpan(repeat fingerprint.Repeat, from time.Duration, comparedWith int) *nodeproto.RepeatedSpan {
	if repeat.HeardIn == 0 {
		return nil
	}
	start, end := repeat.Times()
	return &nodeproto.RepeatedSpan{
		StartMs: (from + start).Milliseconds(), EndMs: (from + end).Milliseconds(),
		HeardIn: repeat.HeardIn, ComparedWith: comparedWith,
	}
}
