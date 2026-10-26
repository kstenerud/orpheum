// SPDX-License-Identifier: AGPL-3.0-or-later

package subtitle

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/kstenerud/orpheum/internal/node/execx"
)

// TrackText is one track of a video: its text, or why this track alone could not be made.
type TrackText struct {
	StreamIndex int
	Text        Text
	// Err is why the track could not be made: FFmpeg's word on its bytes. Nil when Text is made.
	Err error
}

// formatMatroska is the container one read of a film writes its tracks into, each as ASS: the one
// format FFmpeg writes to a single stdout that holds several subtitle tracks.
const formatMatroska = "matroska"

// ExtractTracks extracts the text tracks at streamIndexes of film, a video the caller has checked,
// from one read of it, however many there are (D340). FFmpeg writes every track as ASS into one
// Matroska on stdout; each track is taken out of it in memory and converted to WebVTT, and a track
// that cannot be taken out or converted is the only one to fail. When the one read fails, which a single track FFmpeg
// cannot convert makes it do, each track is read on its own, so that track fails and the others are
// made. The error is the whole extraction's, which says nothing of a track's bytes: a stall, which
// stops it, or ctx ending.
func (extractor *Extractor) ExtractTracks(
	ctx context.Context, film execx.Film, streamIndexes []int,
) ([]TrackText, error) {
	if len(streamIndexes) == 1 {
		return extractor.extractEach(ctx, film, streamIndexes)
	}
	together, err := extractor.readFilm(ctx, film, tracksArguments(film.Input(), streamIndexes),
		MaxTextBytes*len(streamIndexes))
	if endsWholeExtraction(ctx, err) {
		return nil, fmt.Errorf("extract streams %v as ASS: %w", streamIndexes, err)
	}
	if err != nil {
		return extractor.extractEach(ctx, film, streamIndexes)
	}
	tracks := make([]TrackText, 0, len(streamIndexes))
	for place, streamIndex := range streamIndexes {
		text, takeErr := extractor.takeTrack(ctx, together, place)
		if endsWholeExtraction(ctx, takeErr) {
			return nil, fmt.Errorf("take stream %d out of the film's tracks: %w", streamIndex, takeErr)
		}
		tracks = append(tracks, TrackText{StreamIndex: streamIndex, Text: text, Err: takeErr})
	}
	return tracks, nil
}

// extractEach extracts each track on a read of its own, a track that cannot be made failing alone.
func (extractor *Extractor) extractEach(
	ctx context.Context, film execx.Film, streamIndexes []int,
) ([]TrackText, error) {
	tracks := make([]TrackText, 0, len(streamIndexes))
	for _, streamIndex := range streamIndexes {
		text, err := extractor.extractTrack(ctx, film, streamIndex)
		if endsWholeExtraction(ctx, err) {
			return nil, err
		}
		tracks = append(tracks, TrackText{StreamIndex: streamIndex, Text: text, Err: err})
	}
	return tracks, nil
}

// takeTrack takes the track at place out of the Matroska one read of the film wrote, as ASS, and
// converts it to WebVTT.
func (extractor *Extractor) takeTrack(ctx context.Context, together []byte, place int) (Text, error) {
	ass, err := extractor.convert(ctx,
		arguments(execx.PipeInput(), "0:"+strconv.Itoa(place), codecASS, "-f", formatMatroska), together)
	if err != nil {
		return Text{}, fmt.Errorf("take track %d of the film's tracks as ASS: %w", place, err)
	}
	return extractor.withVTT(ctx, ass)
}

// endsWholeExtraction is whether err ends the whole extraction rather than one track: a stall,
// whose run says nothing of the bytes, or ctx ending.
func endsWholeExtraction(ctx context.Context, err error) bool {
	return err != nil && (ctx.Err() != nil || errors.As(err, new(*execx.StalledError)))
}

// tracksArguments read input, execx.Film's Input, and write every stream at streamIndexes as ASS,
// in that order, into one Matroska on stdout.
func tracksArguments(input []string, streamIndexes []int) []string {
	read := append([]string{"-v", "error"}, input...)
	for _, streamIndex := range streamIndexes {
		read = append(read, "-map", "0:"+strconv.Itoa(streamIndex))
	}
	return append(read, "-c:s", codecASS, "-f", formatMatroska, "pipe:1")
}
