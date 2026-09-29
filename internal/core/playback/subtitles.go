// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// The states of an extracted subtitle, as cache.db names them.
const (
	subtitleQueued   = "queued"
	subtitleComplete = "complete"
	subtitleFailed   = "failed"
)

// SubtitleTrack is one text subtitle a playback offers: a track inside the video or a subtitle
// file beside it, named by its stream row, with what the container or the file's name says of it;
// or a subtitle fetched on request, named by its own row, with the provider's name for it (D224).
type SubtitleTrack struct {
	TrackRef
	// Language is BCP 47, or empty when nothing named one.
	Language          string
	Title             string
	IsDefault         bool
	IsForced          bool
	IsHearingImpaired bool
}

// TrackRef names a subtitle track: a stream of the file's, or a subtitle fetched for it. Exactly
// one is set.
type TrackRef struct {
	StreamID int64 `json:"streamId,omitempty"`
	// FetchedSubtitleID is a subtitle fetched on request; it is unverified, as every fetched one is in
	// Cut A (D224 §4, D178 §4).
	FetchedSubtitleID int64 `json:"fetchedSubtitleId,omitempty"`
}

// IsFetched is whether the track is a subtitle fetched on request.
func (ref TrackRef) IsFetched() bool { return ref.FetchedSubtitleID != 0 }

// SubtitleFormat is one of the two forms a subtitle is served in (cut-a §6).
type SubtitleFormat string

// The formats: ASS for JASSUB, WebVTT for the cast receiver.
const (
	FormatASS    SubtitleFormat = "ass"
	FormatWebVTT SubtitleFormat = "vtt"
)

// Subtitle is a playback's subtitle in one format, or how far its extraction is.
type Subtitle struct {
	Readiness Readiness
	Text      []byte
	// Failure is why its extraction failed.
	Failure string
}

// textKey is where a track's text is kept: the payload hash of the file its bytes are in, and its
// stream index there. A subtitle file's one track is its stream 0, as FFmpeg numbers it.
func textKey(payloadHash []byte, streamIndex *int64) cachegen.GetSubtitleTextParams {
	key := cachegen.GetSubtitleTextParams{BytesPayloadHash: payloadHash}
	if streamIndex != nil {
		key.StreamIndex = *streamIndex
	}
	return key
}

// wantSubtitles lists a video's text subtitle tracks, its own and its subtitle files', and has
// each extracted that is not already, whatever the decision: an embedded track means reading the
// whole file, so it is done when the file is first played (D217 §6). A track that failed is tried
// again. Bitmap tracks are not offered (cut-a §1).
func (service *Service) wantSubtitles(ctx context.Context, videoID int64) ([]SubtitleTrack, error) {
	rows, err := service.textTrackRows(ctx, videoID)
	if err != nil {
		return nil, err
	}
	now := service.clock.Now().UnixMilli()
	for _, row := range rows {
		want := wantedText{key: textKey(row.BytesPayloadHash, row.StreamIndex), kind: jobs.KindExtractSubtitle}
		want.target = jobs.Target{StreamID: row.ID}
		if wantErr := service.wantText(ctx, want, now); wantErr != nil {
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
		if wantErr := service.wantText(ctx, want, now); wantErr != nil {
			return nil, wantErr
		}
	}
	return append(streamTracksOf(rows), fetchedTracksOf(fetched)...), nil
}

// textTracksOf lists a video's text subtitle tracks: its own, its subtitle files' and those
// fetched for it.
func (service *Service) textTracksOf(ctx context.Context, videoID int64) ([]SubtitleTrack, error) {
	rows, err := service.textTrackRows(ctx, videoID)
	if err != nil {
		return nil, err
	}
	fetched, err := service.database.Read().ListFetchedSubtitlesOfVideo(ctx, videoID)
	if err != nil {
		return nil, fmt.Errorf("read the fetched subtitles of file %d: %w", videoID, err)
	}
	return append(streamTracksOf(rows), fetchedTracksOf(fetched)...), nil
}

// fetchedTracksOf are a video's fetched subtitles as tracks, named as the provider named them.
func fetchedTracksOf(rows []sqlcgen.ListFetchedSubtitlesOfVideoRow) []SubtitleTrack {
	tracks := make([]SubtitleTrack, 0, len(rows))
	for _, row := range rows {
		tracks = append(tracks, SubtitleTrack{
			TrackRef: TrackRef{FetchedSubtitleID: row.ID}, Language: row.Language,
			Title: ptr.ValueOrZero(row.ReleaseName),
		})
	}
	return tracks
}

// textTrackRows are a video's subtitle tracks that are text in a file with a payload hash.
func (service *Service) textTrackRows(
	ctx context.Context, videoID int64,
) ([]sqlcgen.ListSubtitleTracksOfVideoRow, error) {
	rows, err := service.database.Read().ListSubtitleTracksOfVideo(ctx, videoID)
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

func streamTracksOf(rows []sqlcgen.ListSubtitleTracksOfVideoRow) []SubtitleTrack {
	tracks := make([]SubtitleTrack, 0, len(rows))
	for _, row := range rows {
		tracks = append(tracks, SubtitleTrack{
			TrackRef: TrackRef{StreamID: row.ID}, Language: ptr.ValueOrZero(row.Language),
			Title:             ptr.ValueOrZero(row.ContainerTitle),
			IsDefault:         row.DispositionFlags&library.DispositionBit("default") != 0,
			IsForced:          row.DispositionFlags&library.DispositionBit("forced") != 0,
			IsHearingImpaired: row.DispositionFlags&library.DispositionBit("hearing_impaired") != 0,
		})
	}
	return tracks
}

// wantedText is a track whose text is wanted: where it is kept, and the job that makes it.
type wantedText struct {
	key  cachegen.GetSubtitleTextParams
	kind jobs.Kind
	// target is what the job works on: the stream, or the fetched subtitle.
	target jobs.Target
}

// wantText has one track's text made unless it already is.
func (service *Service) wantText(ctx context.Context, want wantedText, nowMs int64) error {
	key := want.key
	var state string
	err := service.cache.Write(ctx, func(queries *cachegen.Queries) error {
		wantErr := queries.RequestSubtitleText(ctx, cachegen.RequestSubtitleTextParams{
			BytesPayloadHash: key.BytesPayloadHash, StreamIndex: key.StreamIndex, CreatedAt: nowMs,
		})
		if wantErr != nil {
			return wantErr
		}
		row, readErr := queries.GetSubtitleText(ctx, key)
		state = row.State
		return readErr
	})
	if err != nil {
		return fmt.Errorf("want %s %+v: %w", want.kind, want.target, err)
	}
	if state == subtitleComplete {
		return nil
	}
	_, err = service.queue.Enqueue(ctx, jobs.Request{
		Kind: want.kind, Target: want.target, Priority: jobs.PriorityPlayNow,
		Reason: "someone is watching it, and its subtitles are offered",
	})
	return err
}

// Subtitle is the text, in format, of a subtitle track of the file a media token refers to
// (cut-a §5). A track of any other file is no subtitle of this playback.
func (service *Service) Subtitle(
	ctx context.Context, token string, track TrackRef, format SubtitleFormat, userID int64, sessionKey string,
) (Subtitle, error) {
	idle, err := service.tokenIdleExpiry(ctx)
	if err != nil {
		return Subtitle{}, err
	}
	playback, found := service.tokens.use(token, userID, sessionKey, service.clock.Now(), idle)
	if !found {
		return Subtitle{}, ErrNoToken
	}
	key, err := service.textKeyOfTrack(ctx, track, playback.fileID)
	if err != nil {
		return Subtitle{}, err
	}
	row, err := store.OrNotFound(service.cache.Read().GetSubtitleText(ctx, key))
	if errors.Is(err, store.ErrNotFound) {
		return Subtitle{}, ErrNoSubtitle
	}
	if err != nil {
		return Subtitle{}, fmt.Errorf("read the subtitle: %w", err)
	}
	switch row.State {
	case subtitleComplete:
		text := row.Ass
		if format == FormatWebVTT {
			text = row.Vtt
		}
		return Subtitle{Readiness: Complete, Text: text}, nil
	case subtitleFailed:
		reason := ""
		if row.Failure != nil {
			reason = *row.Failure
		}
		return Subtitle{Readiness: Failed, Failure: reason}, nil
	}
	return Subtitle{Readiness: Preparing}, nil
}

// textKeyOfTrack is where a track of video's text is kept, or ErrNoSubtitle for a track of another.
func (service *Service) textKeyOfTrack(
	ctx context.Context, track TrackRef, videoFileID int64,
) (cachegen.GetSubtitleTextParams, error) {
	if track.IsFetched() {
		fetched, err := store.OrNotFound(service.database.Read().GetFetchedSubtitle(ctx, track.FetchedSubtitleID))
		if errors.Is(err, store.ErrNotFound) || (err == nil && fetched.VideoFile != videoFileID) {
			return cachegen.GetSubtitleTextParams{}, ErrNoSubtitle
		}
		if err != nil {
			return cachegen.GetSubtitleTextParams{}, fmt.Errorf("read fetched subtitle %d: %w", track.FetchedSubtitleID,
				err)
		}
		return fetchedKey(fetched.BodyGzip), nil
	}
	stream, err := store.OrNotFound(service.database.Read().GetSubtitleStream(ctx, track.StreamID))
	if errors.Is(err, store.ErrNotFound) || (err == nil && stream.VideoFile != videoFileID) {
		return cachegen.GetSubtitleTextParams{}, ErrNoSubtitle
	}
	if err != nil {
		return cachegen.GetSubtitleTextParams{}, fmt.Errorf("read stream %d: %w", track.StreamID, err)
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
	bytesFile, err := service.bytesFileOf(ctx, located{
		subject: fmt.Sprintf("stream %d", job.StreamID), libraryID: stream.LibraryID, nodeID: stream.NodeID,
		pathKey: stream.ReportedPathBytes, payloadHash: stream.PayloadHash,
	})
	if err != nil {
		return jobs.MarkPermanent(err)
	}
	command := &nodeproto.ExtractSubtitle{JobID: job.ID, Sidecar: &bytesFile}
	if stream.StreamIndex != nil {
		command = &nodeproto.ExtractSubtitle{JobID: job.ID, Video: &bytesFile, StreamIndex: int(*stream.StreamIndex)}
	}
	return service.makeSubtitleText(ctx, job, stream.NodeID, command, textKey(stream.PayloadHash, stream.StreamIndex))
}

// makeSubtitleText has a node make one subtitle's text, waits for its answer and keeps the text where key
// says. FFmpeg's failure is not retried.
func (service *Service) makeSubtitleText(
	ctx context.Context, job jobs.Job, nodeID int64, command *nodeproto.ExtractSubtitle,
	key cachegen.GetSubtitleTextParams,
) error {
	answers := service.subtitleTextAnswers.await(job.ID)
	defer service.subtitleTextAnswers.forget(job.ID)
	service.nodes.Enqueue(nodeID, nodeproto.Command{
		Type: nodeproto.CommandExtractSubtitle, ExtractSubtitle: command,
	})
	timer := time.NewTimer(service.maxSilence)
	defer timer.Stop()
	select {
	case answer := <-answers:
		if answer.Refusal != "" {
			failure := errors.New(answer.Refusal)
			return jobs.MarkPermanent(errors.Join(failure, service.finishSubtitle(ctx, key, answer)))
		}
		return service.finishSubtitle(ctx, key, answer)
	case <-timer.C:
		return fmt.Errorf("node %d said nothing about %s job %d for %s", nodeID, job.Kind, job.ID, service.maxSilence)
	case <-ctx.Done():
		if errors.Is(context.Cause(ctx), jobs.ErrCancelled) {
			// The node stops its FFmpeg; the text stays unmade, so a later play asks again.
			service.nodes.Enqueue(nodeID, cancelCommand(job.ID))
			return jobs.MarkPermanent(jobs.ErrCancelled)
		}
		return fmt.Errorf("wait for %s job %d: %w", job.Kind, job.ID, ctx.Err())
	}
}

// cancelCommand tells a node to stop a job it is working on (cut-a §5.3).
func cancelCommand(jobID int64) nodeproto.Command {
	return nodeproto.Command{Type: nodeproto.CommandCancel, Cancel: &nodeproto.Cancel{JobID: jobID}}
}

// finishSubtitle keeps a node's answer: the text, or why there is none.
func (service *Service) finishSubtitle(
	ctx context.Context, key cachegen.GetSubtitleTextParams, answer nodeproto.SubtitleText,
) error {
	now := service.clock.Now().UnixMilli()
	err := service.cache.Write(ctx, func(queries *cachegen.Queries) error {
		if answer.Refusal != "" {
			return queries.SetSubtitleTextFailed(ctx, cachegen.SetSubtitleTextFailedParams{
				Failure: &answer.Refusal, FinishedAt: &now, BytesPayloadHash: key.BytesPayloadHash,
				StreamIndex: key.StreamIndex,
			})
		}
		return queries.SetSubtitleTextComplete(ctx, cachegen.SetSubtitleTextCompleteParams{
			Ass: answer.ASS, Vtt: answer.VTT, FinishedAt: &now, BytesPayloadHash: key.BytesPayloadHash,
			StreamIndex: key.StreamIndex,
		})
	})
	if err != nil {
		return fmt.Errorf("keep the text of stream index %d of %x: %w", key.StreamIndex, key.BytesPayloadHash, err)
	}
	return nil
}

// TakeSubtitleText takes a node's answer to an extract_subtitle command to the job waiting for it.
func (service *Service) TakeSubtitleText(ctx context.Context, nodeID int64, answer nodeproto.SubtitleText) {
	service.subtitleTextAnswers.deliver(ctx, nodeID, answer.JobID, answer)
}
