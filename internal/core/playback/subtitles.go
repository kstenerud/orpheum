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

// SubtitleTrack is one text subtitle a playback offers: a track inside the video, or a subtitle
// file beside it, named by its stream row, with what the container or the file's name says of it.
type SubtitleTrack struct {
	StreamID int64
	// Language is BCP 47, or empty when nothing named one.
	Language        string
	Title           string
	Default         bool
	Forced          bool
	HearingImpaired bool
}

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
	// Error is why its extraction failed.
	Error string
}

// textKey is where a track's text is kept: the payload hash of the file its bytes are in, and its
// stream index there. A subtitle file's one track is its stream 0, as FFmpeg numbers it.
func textKey(payloadHash []byte, streamIndex *int64) cachegen.GetSubtitleTextParams {
	key := cachegen.GetSubtitleTextParams{PayloadHash: payloadHash}
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
		if err := service.wantText(ctx, row.ID, textKey(row.BytesPayloadHash, row.StreamIndex), now); err != nil {
			return nil, err
		}
	}
	return tracksOf(rows), nil
}

// textTracksOf lists a video's text subtitle tracks, its own and its subtitle files'.
func (service *Service) textTracksOf(ctx context.Context, videoID int64) ([]SubtitleTrack, error) {
	rows, err := service.textTrackRows(ctx, videoID)
	if err != nil {
		return nil, err
	}
	return tracksOf(rows), nil
}

// textTrackRows are a video's subtitle tracks that are text in a file with a payload hash.
func (service *Service) textTrackRows(
	ctx context.Context, videoID int64,
) ([]sqlcgen.ListSubtitleTracksOfVideoRow, error) {
	rows, err := service.database.Read().ListSubtitleTracksOfVideo(ctx, videoID)
	if err != nil {
		return nil, fmt.Errorf("read the subtitle tracks of file %d: %w", videoID, err)
	}
	text := rows[:0]
	for _, row := range rows {
		if row.Codec != nil && decide.IsTextSubtitle(*row.Codec) && row.BytesPayloadHash != nil {
			text = append(text, row)
		}
	}
	return text, nil
}

func tracksOf(rows []sqlcgen.ListSubtitleTracksOfVideoRow) []SubtitleTrack {
	tracks := make([]SubtitleTrack, 0, len(rows))
	for _, row := range rows {
		tracks = append(tracks, SubtitleTrack{
			StreamID: row.ID, Language: ptr.ValueOrZero(row.Lang), Title: ptr.ValueOrZero(row.TitleRaw),
			Default:         row.Disposition&library.DispositionBit("default") != 0,
			Forced:          row.Disposition&library.DispositionBit("forced") != 0,
			HearingImpaired: row.Disposition&library.DispositionBit("hearing_impaired") != 0,
		})
	}
	return tracks
}

// wantText has one track's text extracted unless it already is.
func (service *Service) wantText(
	ctx context.Context, streamID int64, key cachegen.GetSubtitleTextParams, now int64,
) error {
	var state string
	err := service.cache.Write(ctx, func(queries *cachegen.Queries) error {
		wantErr := queries.WantSubtitleText(ctx, cachegen.WantSubtitleTextParams{
			PayloadHash: key.PayloadHash, StreamIndex: key.StreamIndex, CreatedAt: now,
		})
		if wantErr != nil {
			return wantErr
		}
		row, readErr := queries.GetSubtitleText(ctx, key)
		state = row.State
		return readErr
	})
	if err != nil {
		return fmt.Errorf("want stream %d extracted: %w", streamID, err)
	}
	if state == subtitleComplete {
		return nil
	}
	_, err = service.queue.Enqueue(ctx, jobs.New{
		Kind: jobs.KindExtractSubtitle, Subject: streamID, Priority: jobs.PriorityPlayNow,
		Reason: "someone is watching it, and its subtitles are offered",
	})
	return err
}

// Subtitle is the text, in format, of the subtitle track streamID of the file a media token refers
// to (cut-a §5). A track of any other file is no subtitle of this playback.
func (service *Service) Subtitle(
	ctx context.Context, token string, streamID int64, format SubtitleFormat, userID int64, sessionKey string,
) (Subtitle, error) {
	idle, err := service.idle(ctx)
	if err != nil {
		return Subtitle{}, err
	}
	playback, found := service.tokens.use(token, userID, sessionKey, service.clock.Now(), idle)
	if !found {
		return Subtitle{}, ErrNoToken
	}
	stream, err := store.One(service.database.Read().GetSubtitleStream(ctx, streamID))
	if errors.Is(err, store.ErrNotFound) || (err == nil && stream.VideoFile != playback.fileID) {
		return Subtitle{}, ErrNoSubtitle
	}
	if err != nil {
		return Subtitle{}, fmt.Errorf("read stream %d: %w", streamID, err)
	}
	row, err := store.One(service.cache.Read().GetSubtitleText(ctx, textKey(stream.PayloadHash, stream.StreamIndex)))
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
		if row.Error != nil {
			reason = *row.Error
		}
		return Subtitle{Readiness: Failed, Error: reason}, nil
	}
	return Subtitle{Readiness: Preparing}, nil
}

// ErrNoSubtitle is a subtitle a playback does not offer.
var ErrNoSubtitle = errors.New("no such subtitle")

// HandleSubtitle runs one extraction job (jobs.KindExtractSubtitle): the node that holds the
// track's file extracts it, a track from its video or a subtitle file whole, and this waits for its
// answer and keeps the text by that file's payload hash. FFmpeg's failure is not retried.
func (service *Service) HandleSubtitle(ctx context.Context, job jobs.Job) error {
	stream, err := store.One(service.database.Read().GetSubtitleStream(ctx, job.Subject))
	if errors.Is(err, store.ErrNotFound) {
		return jobs.Permanent(fmt.Errorf("no subtitle stream %d: its file was scanned again or is gone", job.Subject))
	}
	if err != nil {
		return fmt.Errorf("read stream %d: %w", job.Subject, err)
	}
	if stream.PayloadHash == nil {
		return jobs.Permanent(fmt.Errorf("stream %d is in a file with no payload hash", job.Subject))
	}
	named, err := service.source(ctx, located{
		id: job.Subject, libraryID: stream.LibraryID, nodeID: stream.NodeID, pathKey: stream.PathKey,
		payloadHash: stream.PayloadHash,
	})
	if err != nil {
		return jobs.Permanent(err)
	}
	command := &nodeproto.ExtractSubtitle{Job: job.ID, Sidecar: &named}
	if stream.StreamIndex != nil {
		command = &nodeproto.ExtractSubtitle{Job: job.ID, Video: &named, StreamIndex: int(*stream.StreamIndex)}
	}
	key := textKey(stream.PayloadHash, stream.StreamIndex)
	answers := service.subtitles.await(job.ID)
	defer service.subtitles.forget(job.ID)
	service.nodes.Enqueue(stream.NodeID, nodeproto.Command{
		Type: nodeproto.CommandExtractSubtitle, ExtractSubtitle: command,
	})
	timer := time.NewTimer(service.maxSilence)
	defer timer.Stop()
	select {
	case answer := <-answers:
		if answer.Refused != "" {
			failure := errors.New(answer.Refused)
			return jobs.Permanent(errors.Join(failure, service.finishSubtitle(ctx, key, answer)))
		}
		return service.finishSubtitle(ctx, key, answer)
	case <-timer.C:
		return fmt.Errorf("node %d said nothing about stream %d for %s", stream.NodeID, job.Subject, service.maxSilence)
	case <-ctx.Done():
		if errors.Is(context.Cause(ctx), jobs.ErrCancelled) {
			// The node stops its FFmpeg; the text stays unmade, so a later play asks again.
			service.nodes.Enqueue(stream.NodeID, cancelCommand(job.ID))
			return jobs.Permanent(jobs.ErrCancelled)
		}
		return fmt.Errorf("wait for stream %d: %w", job.Subject, ctx.Err())
	}
}

// cancelCommand tells a node to stop a job it is working on (cut-a §5.3).
func cancelCommand(jobID int64) nodeproto.Command {
	return nodeproto.Command{Type: nodeproto.CommandCancel, Cancel: &nodeproto.Cancel{Job: jobID}}
}

// finishSubtitle keeps a node's answer: the text, or why there is none.
func (service *Service) finishSubtitle(
	ctx context.Context, key cachegen.GetSubtitleTextParams, answer nodeproto.SubtitleText,
) error {
	now := service.clock.Now().UnixMilli()
	err := service.cache.Write(ctx, func(queries *cachegen.Queries) error {
		if answer.Refused != "" {
			return queries.SetSubtitleTextFailed(ctx, cachegen.SetSubtitleTextFailedParams{
				Error: &answer.Refused, FinishedAt: &now, PayloadHash: key.PayloadHash, StreamIndex: key.StreamIndex,
			})
		}
		return queries.SetSubtitleTextComplete(ctx, cachegen.SetSubtitleTextCompleteParams{
			Ass: answer.ASS, Vtt: answer.VTT, FinishedAt: &now, PayloadHash: key.PayloadHash,
			StreamIndex: key.StreamIndex,
		})
	})
	if err != nil {
		return fmt.Errorf("keep stream %d's text: %w", key.StreamIndex, err)
	}
	return nil
}

// Extracted takes a node's answer to an extract_subtitle command to the job waiting for it.
func (service *Service) Extracted(ctx context.Context, nodeID int64, answer nodeproto.SubtitleText) {
	service.subtitles.deliver(ctx, nodeID, answer.Job, answer)
}
