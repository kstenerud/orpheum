// SPDX-License-Identifier: GPL-3.0-or-later

package subtitles

import (
	"context"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/core/preferences"
	"github.com/kstenerud/orpheum/internal/nodeproto"
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

// Want lists a video's text subtitle tracks, its own and its subtitle files', and has
// each extracted that is not already, whatever the decision: an embedded track means reading the
// whole file, so it is done when the file is first played (D217 §6). A track that failed is tried
// again. Bitmap tracks are not offered (cut-a §1).
func (service *Service) Want(ctx context.Context, videoID int64) ([]Track, error) {
	rows, err := service.textTrackRows(ctx, service.database.Read(), videoID)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		want := wantedText{key: textKey(row.BytesPayloadHash, row.StreamIndex), kind: jobs.KindExtractSubtitle}
		want.target = jobs.Target{StreamID: row.ID}
		if wantErr := service.wantText(ctx, want); wantErr != nil {
			return nil, wantErr
		}
	}
	fetched, err := service.database.Read().ListFetchedSubtitlesOfVideo(ctx, videoID)
	if err != nil {
		return nil, fmt.Errorf("read the fetched subtitles of file %d: %w", videoID, err)
	}
	for _, row := range fetched {
		want := wantedText{
			key: fetchedKey(row.BodyGzip), kind: jobs.KindConvertFetchedSubtitle,
			target: jobs.Target{FetchedSubtitleID: row.ID},
		}
		if wantErr := service.wantText(ctx, want); wantErr != nil {
			return nil, wantErr
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
	// target is what the job works on: the stream, or the fetched subtitle.
	target jobs.Target
}

// wantText has one track's text made unless it already is. One that could not be made is made
// again: a play is the request, and there is no other (D231 section 4).
func (service *Service) wantText(ctx context.Context, want wantedText) error {
	key := want.key
	row, err := store.OrNotFound(service.database.Read().GetSubtitleText(ctx, key))
	switch {
	case err == nil && row.Failure == nil:
		return nil
	case err == nil:
		err = service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
			return queries.ForgetSubtitleTextFailure(ctx, sqlcgen.ForgetSubtitleTextFailureParams(key))
		})
	case errors.Is(err, store.ErrNotFound):
		err = nil
	}
	if err != nil {
		return fmt.Errorf("want %s %+v: %w", want.kind, want.target, err)
	}
	_, err = service.queue.Enqueue(ctx, jobs.Request{
		Kind: want.kind, Target: want.target, Priority: jobs.PriorityPlayNow,
		Reason: "someone is watching it, and its subtitles are offered",
	})
	return err
}

// TextOf is the text, in format, of a subtitle track of video, which a playback of it offers
// (cut-a §5). A track of any other file is ErrNoSubtitle.
func (service *Service) TextOf(
	ctx context.Context, track preferences.TrackRef, videoFileID int64, format Format,
) (Text, error) {
	key, err := service.textKeyOfTrack(ctx, track, videoFileID)
	if err != nil {
		return Text{}, err
	}
	row, err := store.OrNotFound(service.database.Read().GetSubtitleText(ctx, key))
	switch {
	case errors.Is(err, store.ErrNotFound):
		return Text{Readiness: jobs.Preparing}, nil // asked for when the playback began, and not made yet
	case err != nil:
		return Text{}, fmt.Errorf("read the subtitle: %w", err)
	case row.Failure != nil:
		return Text{Readiness: jobs.Failed, Failure: *row.Failure}, nil
	}
	text := row.ASS
	if format == FormatWebVTT {
		text = row.VTT
	}
	return Text{Readiness: jobs.Complete, Text: text}, nil
}

// textKeyOfTrack is where a track of video's text is kept, or ErrNoSubtitle for a track the
// playback does not offer: another video's, or one that is not text.
func (service *Service) textKeyOfTrack(
	ctx context.Context, track preferences.TrackRef, videoFileID int64,
) (sqlcgen.GetSubtitleTextParams, error) {
	if track.IsFetched() {
		fetched, err := store.OrNotFound(service.database.Read().GetFetchedSubtitle(ctx, track.FetchedSubtitleID))
		if errors.Is(err, store.ErrNotFound) || (err == nil && fetched.VideoFile != videoFileID) {
			return sqlcgen.GetSubtitleTextParams{}, ErrNoSubtitle
		}
		if err != nil {
			return sqlcgen.GetSubtitleTextParams{}, fmt.Errorf("read fetched subtitle %d: %w", track.FetchedSubtitleID,
				err)
		}
		return fetchedKey(fetched.BodyGzip), nil
	}
	stream, err := store.OrNotFound(service.database.Read().GetSubtitleStream(ctx, track.StreamID))
	isOffered := err == nil && stream.VideoFile == videoFileID && stream.PayloadHash != nil &&
		stream.Codec != nil && decide.IsTextSubtitle(*stream.Codec)
	if errors.Is(err, store.ErrNotFound) || (err == nil && !isOffered) {
		return sqlcgen.GetSubtitleTextParams{}, ErrNoSubtitle
	}
	if err != nil {
		return sqlcgen.GetSubtitleTextParams{}, fmt.Errorf("read stream %d: %w", track.StreamID, err)
	}
	return textKey(stream.PayloadHash, stream.StreamIndex), nil
}

// ErrNoSubtitle is a subtitle a playback does not offer.
var ErrNoSubtitle = errors.New("no such subtitle")

// HandleExtractSubtitle runs one extraction job (jobs.KindExtractSubtitle): the node that holds the
// track's file extracts it, a track from its video or a subtitle file whole, and this waits for its
// answer and keeps the text by that file's payload hash. FFmpeg's failure is not retried.
func (service *Service) HandleExtractSubtitle(ctx context.Context, job jobs.Job) error {
	stream, err := store.OrNotFound(service.database.Read().GetSubtitleStream(ctx, job.StreamID))
	if errors.Is(err, store.ErrNotFound) {
		return jobs.MarkPermanent(
			fmt.Errorf("no subtitle stream %d: its file was scanned again or is gone", job.StreamID))
	}
	if err != nil {
		return fmt.Errorf("read stream %d: %w", job.StreamID, err)
	}
	if stream.PayloadHash == nil {
		return jobs.MarkPermanent(fmt.Errorf("stream %d is in a file with no payload hash", job.StreamID))
	}
	file := library.Located{
		Subject: fmt.Sprintf("stream %d", job.StreamID), LibraryID: stream.LibraryID, NodeID: stream.NodeID,
		ReportedPathRaw: stream.ReportedPathRaw, PayloadHash: stream.PayloadHash,
	}
	if parked := library.ParkIfUnavailable(ctx, service.database.Read(), service.nodes, job, file); parked != nil {
		return parked
	}
	bytesFile, err := library.BytesFileOf(ctx, service.database.Read(), file)
	if err != nil {
		return jobs.MarkPermanent(err)
	}
	command := &nodeproto.ExtractSubtitle{Sidecar: &bytesFile}
	if stream.StreamIndex != nil {
		command = &nodeproto.ExtractSubtitle{Video: &bytesFile, StreamIndex: int(*stream.StreamIndex)}
	}
	return service.makeSubtitleText(ctx, job, stream.NodeID, command)
}

// makeSubtitleText gives a node the making of one subtitle's text, which says later how it ended:
// its recorder keeps the text where the command's source says (record), and the job is
// resolved by Assignee.
func (service *Service) makeSubtitleText(
	ctx context.Context, job jobs.Job, nodeID int64, command *nodeproto.ExtractSubtitle,
) error {
	if _, _, err := subtitleTextKey(*command); err != nil {
		return jobs.MarkPermanent(err)
	}
	return service.exchanges.Assign(ctx, nodeID, job, nodeproto.Command{
		Work: nodeproto.Work{Type: nodeproto.CommandExtractSubtitle, ExtractSubtitle: command},
	})
}

// Assignee is how a subtitle's making given to a node, an extraction or a fetched subtitle's
// conversion, is resolved. FFmpeg's failure is kept and not retried; a node's refusal, which says
// nothing of the source's bytes, is not kept, and the job is tried again.
func (service *Service) Assignee(runner nodes.Resolver) nodes.Assignee {
	keepFailure := func(ctx context.Context, attempt nodes.Attempt) func(failure string) error {
		return func(failure string) error {
			key, isKept, err := subtitleTextKey(ptr.ValueOrZero(attempt.Work.ExtractSubtitle))
			if err != nil || !isKept {
				return err
			}
			return service.finishSubtitle(ctx, key, nodeproto.SubtitleText{Failure: failure})
		}
	}
	return nodes.Assignee{
		Runner: runner,
		Answer: func(ctx context.Context, job jobs.Job, attempt nodes.Attempt, result nodeproto.Result) error {
			about := fmt.Sprintf("%s job %d", job.Kind, job.ID)
			if result.Subtitle == nil {
				return fmt.Errorf("node %d answered %s with something other than a subtitle", attempt.NodeID, about)
			}
			answer := *result.Subtitle
			switch {
			case answer.Refusal != "":
				return nodes.RefusalError(attempt.NodeID, about, answer.Refusal, answer.RefusalCode)
			case answer.Stall != nil:
				return nodes.FFmpegStallOutcome(job, attempt.NodeID, about, *answer.Stall, keepFailure(ctx, attempt))
			case answer.Failure != "":
				return jobs.MarkPermanent(errors.New(answer.Failure))
			}
			return nil
		},
		Unanswered: func(ctx context.Context, job jobs.Job, attempt nodes.Attempt, err error) error {
			return nodes.UnansweredOutcome(job, err, keepFailure(ctx, attempt))
		},
	}
}

// finishSubtitle keeps a node's answer in persistent.db: the text, or why there is none.
func (service *Service) finishSubtitle(
	ctx context.Context, key sqlcgen.GetSubtitleTextParams, answer nodeproto.SubtitleText,
) error {
	kept := sqlcgen.KeepSubtitleTextParams{
		SourceHash: key.SourceHash, StreamIndex: key.StreamIndex, MadeAt: service.clock.Now().UnixMilli(),
	}
	if answer.Failure != "" {
		kept.Failure = &answer.Failure
	} else {
		kept.ASS, kept.VTT = nonNil(answer.ASS), nonNil(answer.VTT)
	}
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return queries.KeepSubtitleText(ctx, kept)
	})
	if err != nil {
		return fmt.Errorf("keep the text of stream index %d of %x: %w", key.StreamIndex, key.SourceHash, err)
	}
	service.keptTexts.wasKept(key)
	return nil
}

// nonNil is text as kept: made and empty, rather than not made.
func nonNil(text []byte) []byte {
	if text == nil {
		return []byte{}
	}
	return text
}

// RecordFailure records an extract_subtitle or convert_fetched_subtitle job that failed as
// its track's own failure (D231 §4, §10): the playback shows it failed, and the next play of its
// video asks again, a play being the request.
func (service *Service) RecordFailure(ctx context.Context, job jobs.Job, failure string) error {
	var key sqlcgen.GetSubtitleTextParams
	switch job.Kind {
	case jobs.KindExtractSubtitle:
		stream, err := store.OrNotFound(service.database.Read().GetSubtitleStream(ctx, job.StreamID))
		if errors.Is(err, store.ErrNotFound) || (err == nil && stream.PayloadHash == nil) {
			return nil // its file was scanned again or is gone
		}
		if err != nil {
			return fmt.Errorf("read stream %d: %w", job.StreamID, err)
		}
		key = textKey(stream.PayloadHash, stream.StreamIndex)
	case jobs.KindConvertFetchedSubtitle:
		fetched, err := store.OrNotFound(service.database.Read().GetFetchedSubtitle(ctx, job.FetchedSubtitleID))
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read fetched subtitle %d: %w", job.FetchedSubtitleID, err)
		}
		key = fetchedKey(fetched.BodyGzip)
	case jobs.KindFetchMetadata, jobs.KindMakeArtworkSizes, jobs.KindPrepare, jobs.KindMeasureLoudness,
		jobs.KindScan, jobs.KindBackup:
		return fmt.Errorf("job %d: %s is not a subtitle's job", job.ID, job.Kind)
	}
	return service.finishSubtitle(ctx, key, nodeproto.SubtitleText{Failure: "given up: " + failure})
}
