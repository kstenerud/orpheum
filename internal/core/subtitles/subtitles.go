// SPDX-License-Identifier: AGPL-3.0-or-later

package subtitles

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/preferences"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// Track is one text subtitle a playback offers: a track inside the video or a subtitle
// file beside it, named by its stream row, with what the container or the file's name says of it;
// or a subtitle fetched on request, named by its own row, with the provider's name for it (D224).
type Track struct {
	preferences.TrackRef
	// Language is BCP 47, or empty when nothing named one.
	Language string
	// ContainerTitle is the title the container gives a track in or beside the file; empty for
	// a fetched subtitle, or when the container gives none.
	ContainerTitle string
	// ReleaseName is the release a fetched subtitle was made for, as the provider names it;
	// empty for a track in or beside the file.
	ReleaseName       string
	IsDefault         bool
	IsForced          bool
	IsHearingImpaired bool
}

// Format is one of the two forms a subtitle is served in (cut-a §6).
type Format string

// The formats: ASS for JASSUB, WebVTT for the cast receiver.
const (
	FormatASS    Format = "ass"
	FormatWebVTT Format = "vtt"
)

// Text is a subtitle's text in one format, or how far its making is.
type Text struct {
	Readiness jobs.Readiness
	Text      []byte
	// Failure is why its extraction failed.
	Failure string
}

// textKey is where a track's text is kept in persistent.db: the payload hash of the file its bytes
// are in, and its stream index there. A subtitle file's one track is its stream 0, as FFmpeg
// numbers it.
func textKey(payloadHash []byte, streamIndex *int64) sqlcgen.GetSubtitleTextParams {
	return sqlcgen.GetSubtitleTextParams{SourceHash: payloadHash, StreamIndex: ptr.ValueOrZero(streamIndex)}
}

// Want lists a video's text subtitle tracks, its own and its subtitle files', and has each
// extracted that is not already, whatever the decision: an embedded track means reading the whole
// file, so it is done when the file is first played (D217 §6), every track not yet made from one
// read of it, one job per file the tracks are in (D340). A track that failed is tried again. Bitmap
// tracks are not offered (cut-a §1).
func (service *Service) Want(ctx context.Context, videoID int64) ([]Track, error) {
	rows, err := service.textTrackRows(ctx, service.database.Read(), videoID)
	if err != nil {
		return nil, err
	}
	var filesToExtract []int64
	for _, row := range rows {
		isNeeded, needErr := service.needsText(ctx, textKey(row.BytesPayloadHash, row.StreamIndex))
		if needErr != nil {
			return nil, needErr
		}
		if isNeeded && !slices.Contains(filesToExtract, row.BytesFile) {
			filesToExtract = append(filesToExtract, row.BytesFile)
		}
	}
	for _, fileID := range filesToExtract {
		if queueErr := service.queueText(ctx, jobs.KindExtractSubtitle, jobs.Target{FileID: fileID}); queueErr != nil {
			return nil, queueErr
		}
	}
	fetched, err := service.database.Read().ListFetchedSubtitlesOfVideo(ctx, videoID)
	if err != nil {
		return nil, fmt.Errorf("read the fetched subtitles of file %d: %w", videoID, err)
	}
	for _, row := range fetched {
		isNeeded, needErr := service.needsText(ctx, fetchedKey(row.BodyGzip))
		if needErr != nil {
			return nil, needErr
		}
		if !isNeeded {
			continue
		}
		target := jobs.Target{FetchedSubtitleID: row.ID}
		if queueErr := service.queueText(ctx, jobs.KindConvertFetchedSubtitle, target); queueErr != nil {
			return nil, queueErr
		}
	}
	return append(streamTracksOf(rows), fetchedTracksOf(fetched)...), nil
}

// Tracks lists a video's text subtitle tracks: its own, its subtitle files' and those
// fetched for it.
func (service *Service) Tracks(
	ctx context.Context, queries *sqlcgen.Queries, videoID int64,
) ([]Track, error) {
	rows, err := service.textTrackRows(ctx, queries, videoID)
	if err != nil {
		return nil, err
	}
	fetched, err := queries.ListFetchedSubtitlesOfVideo(ctx, videoID)
	if err != nil {
		return nil, fmt.Errorf("read the fetched subtitles of file %d: %w", videoID, err)
	}
	return append(streamTracksOf(rows), fetchedTracksOf(fetched)...), nil
}

// fetchedTracksOf are a video's fetched subtitles as tracks, named as the provider named them.
func fetchedTracksOf(rows []sqlcgen.ListFetchedSubtitlesOfVideoRow) []Track {
	tracks := make([]Track, 0, len(rows))
	for _, row := range rows {
		tracks = append(tracks, Track{
			TrackRef: preferences.TrackRef{FetchedSubtitleID: row.ID}, Language: row.Language,
			ReleaseName: ptr.ValueOrZero(row.ReleaseName),
		})
	}
	return tracks
}

// textTrackRows are a video's subtitle tracks that are text in a file with a payload hash.
func (service *Service) textTrackRows(
	ctx context.Context, queries *sqlcgen.Queries, videoID int64,
) ([]sqlcgen.ListSubtitleTracksOfVideoRow, error) {
	rows, err := queries.ListSubtitleTracksOfVideo(ctx, videoID)
	if err != nil {
		return nil, fmt.Errorf("read the subtitle tracks of file %d: %w", videoID, err)
	}
	textRows := rows[:0]
	for _, row := range rows {
		if row.Codec != nil && decide.IsTextSubtitle(*row.Codec) && row.BytesPayloadHash != nil {
			textRows = append(textRows, row)
		}
	}
	return textRows, nil
}

func streamTracksOf(rows []sqlcgen.ListSubtitleTracksOfVideoRow) []Track {
	tracks := make([]Track, 0, len(rows))
	for _, row := range rows {
		tracks = append(tracks, Track{
			TrackRef: preferences.TrackRef{StreamID: row.ID}, Language: ptr.ValueOrZero(row.Language),
			ContainerTitle:    ptr.ValueOrZero(row.ContainerTitle),
			IsDefault:         row.DispositionFlags&library.DispositionBit("default") != 0,
			IsForced:          row.DispositionFlags&library.DispositionBit("forced") != 0,
			IsHearingImpaired: row.DispositionFlags&library.DispositionBit("hearing_impaired") != 0,
		})
	}
	return tracks
}

// wantedText is a track whose text is wanted: where it is kept, and the job that makes it.
type wantedText struct {
	key  sqlcgen.GetSubtitleTextParams
	kind jobs.Kind
	// target is what the job works on: the file the track is in, or the fetched subtitle.
	target jobs.Target
}

// needsText is whether a track's text is still to be made. One that could not be made is made
// again, its failure forgotten: a play is the request, and there is no other (D231 section 4).
func (service *Service) needsText(ctx context.Context, key sqlcgen.GetSubtitleTextParams) (bool, error) {
	row, err := store.OrNotFound(service.database.Read().GetSubtitleText(ctx, key))
	switch {
	case err == nil && row.Failure == nil:
		return false, nil
	case err == nil:
		err = service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
			return queries.ForgetSubtitleTextFailure(ctx, sqlcgen.ForgetSubtitleTextFailureParams(key))
		})
	case errors.Is(err, store.ErrNotFound):
		err = nil
	}
	if err != nil {
		return false, fmt.Errorf("want the text of stream index %d of %x: %w", key.StreamIndex, key.SourceHash, err)
	}
	return true, nil
}

// queueText queues the job that makes texts, at play's priority.
func (service *Service) queueText(ctx context.Context, kind jobs.Kind, target jobs.Target) error {
	_, err := service.queue.Enqueue(ctx, jobs.Request{
		Kind: kind, Target: target, Priority: jobs.PriorityPlayNow,
		Reason: "someone is watching it, and its subtitles are offered",
	})
	return err
}

// TextOf is the text, in format, of a subtitle track of video, which a playback of it offers
// (cut-a §5). A track of any other file is ErrNoSubtitle.
func (service *Service) TextOf(
	ctx context.Context, track preferences.TrackRef, videoFileID int64, format Format,
) (Text, error) {
	want, err := service.wantedOfTrack(ctx, track, videoFileID)
	if err != nil {
		return Text{}, err
	}
	row, err := store.OrNotFound(service.database.Read().GetSubtitleText(ctx, want.key))
	switch {
	case errors.Is(err, store.ErrNotFound):
		// Asked for when the playback began, and not made yet. The player asking for it is a person
		// still waiting for it, so its job keeps the priority of play (D234 §3).
		if keepErr := service.queue.KeepWanted(ctx, want.kind, want.target); keepErr != nil {
			return Text{}, keepErr
		}
		return Text{Readiness: jobs.ReadinessPreparing}, nil
	case err != nil:
		return Text{}, fmt.Errorf("read the subtitle: %w", err)
	case row.Failure != nil:
		return Text{Readiness: jobs.ReadinessFailed, Failure: *row.Failure}, nil
	}
	text := row.ASS
	if format == FormatWebVTT {
		text = row.VTT
	}
	return Text{Readiness: jobs.ReadinessComplete, Text: text}, nil
}

// wantedOfTrack is where a track of video's text is kept and the job that makes it, or
// ErrNoSubtitle for a track the playback does not offer: another video's, or one that is not text.
func (service *Service) wantedOfTrack(
	ctx context.Context, track preferences.TrackRef, videoFileID int64,
) (wantedText, error) {
	if track.IsFetched() {
		fetched, err := store.OrNotFound(service.database.Read().GetFetchedSubtitle(ctx, track.FetchedSubtitleID))
		if errors.Is(err, store.ErrNotFound) || (err == nil && fetched.VideoFile != videoFileID) {
			return wantedText{}, ErrNoSubtitle
		}
		if err != nil {
			return wantedText{}, fmt.Errorf("read fetched subtitle %d: %w", track.FetchedSubtitleID, err)
		}
		return wantedText{
			key: fetchedKey(fetched.BodyGzip), kind: jobs.KindConvertFetchedSubtitle,
			target: jobs.Target{FetchedSubtitleID: track.FetchedSubtitleID},
		}, nil
	}
	stream, err := store.OrNotFound(service.database.Read().GetSubtitleStream(ctx, track.StreamID))
	isOffered := err == nil && stream.VideoFile == videoFileID && stream.PayloadHash != nil &&
		stream.Codec != nil && decide.IsTextSubtitle(*stream.Codec)
	if errors.Is(err, store.ErrNotFound) || (err == nil && !isOffered) {
		return wantedText{}, ErrNoSubtitle
	}
	if err != nil {
		return wantedText{}, fmt.Errorf("read stream %d: %w", track.StreamID, err)
	}
	return wantedText{
		key: textKey(stream.PayloadHash, stream.StreamIndex), kind: jobs.KindExtractSubtitle,
		target: jobs.Target{FileID: stream.BytesFile},
	}, nil
}

// ErrNoSubtitle is a subtitle a playback does not offer.
var ErrNoSubtitle = errors.New("no such subtitle")
