// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
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

// SubtitleTrack is one text subtitle a playback offers: a track inside the video, by its stream
// index, with what its container says of it.
type SubtitleTrack struct {
	StreamIndex int
	// Language is BCP 47, or empty when the container named none.
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

// textTracks are the text subtitle tracks of a probed video, in stream order (D217 §6).
func textTracks(probe nodeproto.Probe) []SubtitleTrack {
	var tracks []SubtitleTrack
	for _, stream := range probe.Streams {
		if stream.Kind != "subtitle" || !decide.IsTextSubtitle(stream.Codec) {
			continue
		}
		tracks = append(tracks, SubtitleTrack{
			StreamIndex: stream.Index, Language: stream.LanguageTag, Title: stream.Title,
			Default:         slices.Contains(stream.Disposition, "default"),
			Forced:          slices.Contains(stream.Disposition, "forced"),
			HearingImpaired: slices.Contains(stream.Disposition, "hearing_impaired"),
		})
	}
	return tracks
}

// wantSubtitles has every text track of a video extracted that is not already, whatever the
// decision: an embedded track means reading the whole file, so it is done when the file is first
// played (D217 §6). A track that failed is tried again.
func (service *Service) wantSubtitles(
	ctx context.Context, file sqlcgen.GetPlayableFileRow, tracks []SubtitleTrack,
) error {
	if len(tracks) == 0 {
		return nil
	}
	streams, err := service.database.Read().ListSubtitleStreamsOfFile(ctx, file.ID)
	if err != nil {
		return fmt.Errorf("read the subtitle tracks of file %d: %w", file.ID, err)
	}
	streamIDByIndex := map[int64]int64{}
	for _, stream := range streams {
		if stream.StreamIndex != nil {
			streamIDByIndex[*stream.StreamIndex] = stream.ID
		}
	}
	now := service.clock.Now().UnixMilli()
	for _, track := range tracks {
		streamID, found := streamIDByIndex[int64(track.StreamIndex)]
		if !found {
			continue // the probe and the stream rows disagree until the next scan
		}
		var state string
		key := cachegen.GetSubtitleTextParams{PayloadHash: file.PayloadHash, StreamIndex: int64(track.StreamIndex)}
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
			return fmt.Errorf("want stream %d of file %d extracted: %w", track.StreamIndex, file.ID, err)
		}
		if state == subtitleComplete {
			continue
		}
		_, err = service.queue.Enqueue(ctx, jobs.New{
			Kind: jobs.KindExtractSubtitle, Subject: streamID, Priority: jobs.PriorityPlayNow,
			Reason: "someone is watching it, and its subtitles are offered",
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// Subtitle is the text, in format, of the subtitle at streamIndex of the file a media token
// refers to (cut-a §5).
func (service *Service) Subtitle(
	ctx context.Context, token string, streamIndex int, format SubtitleFormat, userID int64, sessionKey string,
) (Subtitle, error) {
	idle, err := service.idle(ctx)
	if err != nil {
		return Subtitle{}, err
	}
	playback, found := service.tokens.use(token, userID, sessionKey, service.clock.Now(), idle)
	if !found {
		return Subtitle{}, ErrNoToken
	}
	file, err := service.file(ctx, playback.fileID)
	if err != nil {
		return Subtitle{}, err
	}
	row, err := store.One(service.cache.Read().GetSubtitleText(ctx, cachegen.GetSubtitleTextParams{
		PayloadHash: file.PayloadHash, StreamIndex: int64(streamIndex),
	}))
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

// HandleSubtitle runs one extraction job (jobs.KindExtractSubtitle): the node that holds the video
// extracts the track, and this waits for its answer and keeps the text by the video's payload
// hash. FFmpeg's failure is not retried.
func (service *Service) HandleSubtitle(ctx context.Context, job jobs.Job) error {
	stream, err := store.One(service.database.Read().GetSubtitleStream(ctx, job.Subject))
	if errors.Is(err, store.ErrNotFound) {
		return jobs.Permanent(fmt.Errorf("no subtitle stream %d: the video was scanned again since", job.Subject))
	}
	if err != nil {
		return fmt.Errorf("read stream %d: %w", job.Subject, err)
	}
	file, err := service.file(ctx, stream.BytesFile)
	if err != nil {
		return jobs.Permanent(err)
	}
	video, err := service.source(ctx, file)
	if err != nil {
		return jobs.Permanent(err)
	}
	if stream.StreamIndex == nil {
		return jobs.Permanent(fmt.Errorf("stream %d is a sidecar, not a track", job.Subject))
	}
	key := cachegen.GetSubtitleTextParams{PayloadHash: file.PayloadHash, StreamIndex: *stream.StreamIndex}
	answers := service.subtitles.await(job.ID)
	defer service.subtitles.forget(job.ID)
	service.nodes.Enqueue(file.NodeID, nodeproto.Command{
		Type: nodeproto.CommandExtractSubtitle,
		ExtractSubtitle: &nodeproto.ExtractSubtitle{
			Job: job.ID, Video: &video, StreamIndex: int(*stream.StreamIndex),
		},
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
		return fmt.Errorf("node %d said nothing about stream %d for %s", file.NodeID, job.Subject, service.maxSilence)
	case <-ctx.Done():
		return fmt.Errorf("wait for stream %d: %w", job.Subject, ctx.Err())
	}
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
