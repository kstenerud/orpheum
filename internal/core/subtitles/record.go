// SPDX-License-Identifier: AGPL-3.0-or-later

package subtitles

import (
	"context"
	"encoding/hex"
	"fmt"
	"slices"

	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// keptText is one subtitle's text as a node made it, or why it could not, and where it is kept.
type keptText struct {
	key    sqlcgen.GetSubtitleTextParams
	answer nodeproto.SubtitleText
}

// record keeps the texts a node made, or FFmpeg's word that they cannot be made, by their source:
// a video's each track's own (D340), or a subtitle file's or a fetched subtitle's one. A command
// naming no source to keep it by keeps nothing.
func (service *Service) record(ctx context.Context, _ int64, result nodeproto.Result) error {
	answer, command := result.Subtitle, result.Work.ExtractSubtitle
	if answer == nil || command == nil {
		return fmt.Errorf("a subtitle's result carries no text, or names no subtitle: %w", nodes.ErrMalformedResult)
	}
	if answer.Refusal != "" || answer.Stall != nil {
		return nil
	}
	keys, err := subtitleTextKeys(*command)
	if err != nil {
		return err
	}
	if command.Video == nil || answer.Failure != "" {
		return service.finishSubtitles(ctx, keptAlike(keys, *answer))
	}
	texts := make([]keptText, 0, len(answer.Tracks))
	for _, track := range answer.Tracks {
		place := slices.Index(command.StreamIndexes, track.StreamIndex)
		if place < 0 {
			return fmt.Errorf("%w: a text for stream %d, which the command did not name", nodes.ErrMalformedResult,
				track.StreamIndex)
		}
		texts = append(texts, keptText{
			key: keys[place], answer: nodeproto.SubtitleText{ASS: track.ASS, VTT: track.VTT, Failure: track.Failure},
		})
	}
	return service.finishSubtitles(ctx, texts)
}

// keptAlike is the one answer kept at every key: a subtitle file's or a fetched subtitle's at its
// one, or a failure at each of a video's tracks.
func keptAlike(keys []sqlcgen.GetSubtitleTextParams, answer nodeproto.SubtitleText) []keptText {
	texts := make([]keptText, 0, len(keys))
	for _, key := range keys {
		texts = append(texts, keptText{key: key, answer: answer})
	}
	return texts
}

// subtitleTextKeys are where the texts a command asks for are kept: each of its video's tracks by
// the video's payload hash and the track's stream index, in the command's order; its subtitle
// file's one by that file's payload hash; or a fetched subtitle's by the hash of its body as
// persistent.db keeps it. A fetched subtitle may name none.
func subtitleTextKeys(command nodeproto.ExtractSubtitle) ([]sqlcgen.GetSubtitleTextParams, error) {
	var hashHex string
	switch {
	case command.Video != nil && len(command.StreamIndexes) == 0:
		return nil, fmt.Errorf("%w: a video's extraction names no track", nodes.ErrMalformedResult)
	case command.Video != nil:
		hashHex = command.Video.PayloadHash
	case command.Sidecar != nil:
		hashHex = command.Sidecar.PayloadHash
	case command.FetchedSha256 != "":
		hashHex = command.FetchedSha256
	default:
		return nil, nil
	}
	hash, err := hex.DecodeString(hashHex)
	if err != nil {
		return nil, fmt.Errorf("%w: a source hash %q: %w", nodes.ErrMalformedResult, hashHex, err)
	}
	if command.Video == nil {
		return []sqlcgen.GetSubtitleTextParams{textKey(hash, nil)}, nil
	}
	keys := make([]sqlcgen.GetSubtitleTextParams, 0, len(command.StreamIndexes))
	for _, streamIndex := range command.StreamIndexes {
		keys = append(keys, textKey(hash, new(int64(streamIndex))))
	}
	return keys, nil
}

// finishSubtitles keeps nodes' answers in persistent.db, in one transaction: each text, or why there
// is none.
func (service *Service) finishSubtitles(ctx context.Context, texts []keptText) error {
	if len(texts) == 0 {
		return nil
	}
	madeAt := service.clock.Now().UnixMilli()
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		for _, text := range texts {
			kept := sqlcgen.KeepSubtitleTextParams{
				SourceHash: text.key.SourceHash, StreamIndex: text.key.StreamIndex, MadeAt: madeAt,
			}
			if text.answer.Failure != "" {
				kept.Failure = &text.answer.Failure
			} else {
				kept.ASS, kept.VTT = nonNil(text.answer.ASS), nonNil(text.answer.VTT)
			}
			if keepErr := queries.KeepSubtitleText(ctx, kept); keepErr != nil {
				return fmt.Errorf("keep the text of stream index %d of %x: %w", text.key.StreamIndex,
					text.key.SourceHash, keepErr)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("keep %d subtitle texts: %w", len(texts), err)
	}
	for _, text := range texts {
		service.keptTexts.wasKept(text.key)
	}
	return nil
}

// nonNil is text as kept: made and empty, rather than not made.
func nonNil(text []byte) []byte {
	if text == nil {
		return []byte{}
	}
	return text
}
