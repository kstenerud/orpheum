// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"context"
	"fmt"
	"slices"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// DispositionBit is FFmpeg's AV_DISPOSITION_* flag for the name ffprobe prints, or 0: the
// stream table keeps the flags as found (schema.md, stream).
func DispositionBit(name string) int64 {
	bits := []string{
		"default", "dub", "original", "comment", "lyrics", "karaoke", "forced", "hearing_impaired",
		"visual_impaired", "clean_effects", "attached_pic", "timed_thumbnails", "non_diegetic", "", "", "",
		"captions", "descriptions", "metadata", "dependent", "still_image",
	}
	if index := slices.Index(bits, name); index >= 0 && name != "" {
		return 1 << index
	}
	return 0
}

// isTrackKind reports whether the stream table keeps a probe stream kind; data streams are not
// tracks.
func isTrackKind(kind string) bool {
	return kind == "video" || kind == "audio" || kind == "subtitle" || kind == "attachment"
}

// streams replaces a file's stream rows with those its probe describes. They are derived rows
// (class O): re-derived from the stored probe, never edited.
func (work *batch) streams(ctx context.Context, fileID int64, probe nodeproto.Probe) error {
	if err := work.queries.DeleteFileStreams(ctx, fileID); err != nil {
		return fmt.Errorf("clear the streams of file %d: %w", fileID, err)
	}
	for _, stream := range probe.Streams {
		if !isTrackKind(stream.Kind) {
			continue
		}
		var disposition int64
		for _, flag := range stream.Disposition {
			disposition |= DispositionBit(flag)
		}
		index := int64(stream.Index)
		err := work.queries.InsertStream(ctx, sqlcgen.InsertStreamParams{
			FileID: fileID, StreamIndex: &index, Kind: stream.Kind, Codec: optional(stream.Codec),
			LangRaw: optional(stream.Language), Lang: optional(stream.LanguageTag), TitleRaw: optional(stream.Title),
			Disposition: disposition, Channels: optionalInt(int64(stream.Audio.Channels)),
		})
		if err != nil {
			return fmt.Errorf("insert stream %d of file %d: %w", stream.Index, fileID, err)
		}
	}
	return nil
}

func optional(text string) *string {
	if text == "" {
		return nil
	}
	return &text
}

func optionalInt(value int64) *int64 {
	if value == 0 {
		return nil
	}
	return &value
}
