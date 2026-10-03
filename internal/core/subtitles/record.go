// SPDX-License-Identifier: GPL-3.0-or-later

package subtitles

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// recordSubtitle keeps a subtitle's text, or FFmpeg's word that it cannot be made, by its source.
// A command naming no source to keep it by keeps nothing.
func (service *Service) record(ctx context.Context, _ int64, result nodeproto.Result) error {
	answer, command := result.Subtitle, result.Work.ExtractSubtitle
	if answer == nil || command == nil {
		return fmt.Errorf("a subtitle's result carries no text, or names no subtitle: %w", nodes.ErrMalformedResult)
	}
	if answer.Refusal != "" || answer.Stall != nil {
		return nil
	}
	key, isKept, err := subtitleTextKey(*command)
	if err != nil || !isKept {
		return err
	}
	return service.finishSubtitle(ctx, key, *answer)
}

// subtitleTextKey is where the text a command asks for is kept: by its video's payload hash and the
// track's stream index, by its subtitle file's payload hash, or by the hash of a fetched subtitle's
// body as persistent.db keeps it. It says, as isKept, whether there is one: a fetched subtitle may
// name none.
func subtitleTextKey(
	command nodeproto.ExtractSubtitle,
) (key sqlcgen.GetSubtitleTextParams, isKept bool, err error) {
	var hashHex string
	var streamIndex *int64
	switch {
	case command.Video != nil:
		index := int64(command.StreamIndex)
		hashHex, streamIndex = command.Video.PayloadHash, &index
	case command.Sidecar != nil:
		hashHex = command.Sidecar.PayloadHash
	case command.FetchedSha256 != "":
		hashHex = command.FetchedSha256
	default:
		return sqlcgen.GetSubtitleTextParams{}, false, nil
	}
	hash, err := hex.DecodeString(hashHex)
	if err != nil {
		return sqlcgen.GetSubtitleTextParams{}, false, fmt.Errorf("%w: a source hash %q: %w",
			nodes.ErrMalformedResult, hashHex, err)
	}
	return textKey(hash, streamIndex), true, nil
}
