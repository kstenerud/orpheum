// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/kstenerud/orpheum/internal/platform/langtag"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// sidecarTrack is what a subtitle file's name says of its one track.
type sidecarTrack struct {
	codec             string
	containerLanguage string
	languageTag       string
	disposition       int64
}

// subtitleCodec names a subtitle file's codec, by its extension, as ffprobe would.
func subtitleCodec(extension string) string {
	switch strings.ToLower(extension) {
	case "srt":
		return "subrip"
	case "vtt":
		return "webvtt"
	}
	return strings.ToLower(extension)
}

// sidecarTrackOf reads a subtitle file's name after its video's, as Kodi and Jellyfin do:
// "Film (2020).en.forced.srt" beside "Film (2020).mkv" is English and forced. A language code is
// the first word that is one; "forced" marks a forced track, "sdh", "hi" and "cc" one for the deaf
// and hard of hearing, "default" the default. Any other word is a label and says nothing.
func sidecarTrackOf(sidecarPath, videoPath string) sidecarTrack {
	name := path.Base(sidecarPath)
	extension := path.Ext(name)
	track := sidecarTrack{codec: subtitleCodec(strings.TrimPrefix(extension, "."))}
	videoStem := strings.TrimSuffix(path.Base(videoPath), path.Ext(videoPath))
	middle, namedAfterVideo := strings.CutPrefix(strings.TrimSuffix(name, extension), videoStem+".")
	if !namedAfterVideo {
		return track // "Film.srt" says nothing more, and a name not the video's says nothing we read
	}
	for word := range strings.SplitSeq(middle, ".") {
		switch strings.ToLower(word) {
		case "forced":
			track.disposition |= DispositionBit("forced")
		case "sdh", "hi", "cc":
			track.disposition |= DispositionBit("hearing_impaired")
		case "default":
			track.disposition |= DispositionBit("default")
		default:
			if tag := langtag.BCP47(word); track.languageTag == "" && tag != "" {
				track.containerLanguage, track.languageTag = word, tag
			}
		}
	}
	return track
}

// videoOf is the row of the video owner names among a folder's videos, or nil.
func videoOf(ownerVideoID *int64, videos []sqlcgen.ListLiveFilesForAttachingRow) *sqlcgen.ListLiveFilesForAttachingRow {
	if ownerVideoID == nil {
		return nil
	}
	for index := range videos {
		if videos[index].ID == *ownerVideoID {
			return &videos[index]
		}
	}
	return nil
}

// keepSidecarTrack keeps a subtitle sidecar's one track beside its video's own tracks, as its name
// describes it, so a playback offers it with them (D217 §6). A sidecar that belongs to no video has
// no track. The row is changed in place, so its id, which an extraction job names, holds.
func (batch *ingestBatch) keepSidecarTrack(
	ctx context.Context, sidecar sqlcgen.ListLiveFilesForAttachingRow, video *sqlcgen.ListLiveFilesForAttachingRow,
) error {
	existing, err := store.OrNotFound(batch.queries.GetSidecarStream(ctx, sidecar.ID))
	hasTrack := err == nil
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("read the track of %s: %w", sidecar.Path, err)
	}
	if video == nil {
		if !hasTrack {
			return nil
		}
		if dropErr := batch.queries.DeleteFileStreams(ctx, sidecar.ID); dropErr != nil {
			return fmt.Errorf("drop the track of %s: %w", sidecar.Path, dropErr)
		}
		return nil
	}
	track := sidecarTrackOf(sidecar.Path, video.Path)
	codec := nilIfEmpty(track.codec)
	containerLanguage, languageTag := nilIfEmpty(track.containerLanguage), nilIfEmpty(track.languageTag)
	if !hasTrack {
		err = batch.queries.InsertSidecarStream(ctx, sqlcgen.InsertSidecarStreamParams{
			BytesFile: sidecar.ID, VideoFile: video.ID, Codec: codec, ContainerLanguage: containerLanguage,
			Language:         languageTag,
			DispositionFlags: track.disposition,
		})
	} else if existing.VideoFile != video.ID || !samePointee(existing.Codec, codec) ||
		!samePointee(existing.ContainerLanguage, containerLanguage) || !samePointee(existing.Language, languageTag) ||
		existing.DispositionFlags != track.disposition {
		err = batch.queries.UpdateSidecarStream(ctx, sqlcgen.UpdateSidecarStreamParams{
			VideoFile: video.ID, Codec: codec, ContainerLanguage: containerLanguage, Language: languageTag,
			DispositionFlags: track.disposition,
			ID:               existing.ID,
		})
	}
	if err != nil {
		return fmt.Errorf("keep the track of %s: %w", sidecar.Path, err)
	}
	return nil
}
