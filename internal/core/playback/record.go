// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/nodes"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// recordResults has exchanges record what a node's results on playback's work establish, by the
// content each names (D238 §2): a making of a rendition by its run, a subtitle's text by its
// source's hash. Each is recorded as it arrives, whether or not its
// job still waits, before the node is told it was taken (S81).
func (service *Service) recordResults(exchanges *nodes.Exchanges) {
	exchanges.Record(nodeproto.CommandPrepare, service.prepares.recordPrepare)
	exchanges.Record(nodeproto.CommandExtractSubtitle, service.recordSubtitle)
}

// recordPrepare records a making of a rendition done, or failed with FFmpeg's word on its source,
// by its run; a rendition made again since, under another run, is not touched (G13).
func (all *prepares) recordPrepare(ctx context.Context, nodeID int64, result nodeproto.Result) error {
	report, command := result.Prepare, result.Work.Prepare
	if report == nil || command == nil {
		return fmt.Errorf("a prepare's result carries no report, or names no prepare: %w", nodes.ErrMalformedResult)
	}
	var done *nodeproto.PrepareReport
	switch report.State {
	case nodeproto.PrepareDone:
		done = report
	case nodeproto.PrepareFailed:
	case nodeproto.PrepareRefused, nodeproto.PrepareStalled:
		return nil // says nothing of the source, and the job decides
	}
	run, err := parseRenditionDir(command.RenditionDir)
	if err != nil {
		return fmt.Errorf("%w: %w", nodes.ErrMalformedResult, err)
	}
	isCurrent, err := all.finish(ctx, run, done, report.Failure)
	if err == nil && !isCurrent {
		all.logger.InfoContext(ctx, "a node's answer on a making of a rendition no longer current was not recorded",
			"nodeId", nodeID, "jobId", result.JobID, "renditionDir", command.RenditionDir)
	}
	return err
}

// recordSubtitle keeps a subtitle's text, or FFmpeg's word that it cannot be made, by its source.
// A command naming no source to keep it by keeps nothing.
func (service *Service) recordSubtitle(ctx context.Context, _ int64, result nodeproto.Result) error {
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
