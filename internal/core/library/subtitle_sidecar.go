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
	codec       string
	langRaw     string
	lang        string
	disposition int64
}

// subtitleCodecs name a subtitle file's codec, by its extension, as ffprobe would.
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
			if tag := langtag.BCP47(word); track.lang == "" && tag != "" {
				track.langRaw, track.lang = word, tag
			}
		}
	}
	return track
}

// videoOf is the row of the video owner names among a folder's videos, or nil.
func videoOf(owner *int64, videos []sqlcgen.ListLiveFilesForAttachingRow) *sqlcgen.ListLiveFilesForAttachingRow {
	if owner == nil {
		return nil
	}
	for index := range videos {
		if videos[index].ID == *owner {
			return &videos[index]
		}
	}
	return nil
}

// keepSidecarTrack keeps a subtitle sidecar's one track beside its video's own tracks, as its name
// describes it, so a playback offers it with them (D217 §6). A sidecar that belongs to no video has
// no track. The row is changed in place, so its id, which an extraction job names, holds.
func (work *batch) keepSidecarTrack(
	ctx context.Context, sidecar sqlcgen.ListLiveFilesForAttachingRow, video *sqlcgen.ListLiveFilesForAttachingRow,
) error {
	existing, err := store.One(work.queries.GetSidecarStream(ctx, sidecar.ID))
	found := err == nil
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("read the track of %s: %w", sidecar.Path, err)
	}
	if video == nil {
		if !found {
			return nil
		}
		if dropErr := work.queries.DeleteFileStreams(ctx, sidecar.ID); dropErr != nil {
			return fmt.Errorf("drop the track of %s: %w", sidecar.Path, dropErr)
		}
		return nil
	}
	track := sidecarTrackOf(sidecar.Path, video.Path)
	codec, langRaw, lang := optional(track.codec), optional(track.langRaw), optional(track.lang)
	if !found {
		err = work.queries.InsertSidecarStream(ctx, sqlcgen.InsertSidecarStreamParams{
			BytesFile: sidecar.ID, VideoFile: video.ID, Codec: codec, LangRaw: langRaw, Lang: lang,
			Disposition: track.disposition,
		})
	} else if existing.VideoFile != video.ID || !samePointee(existing.Codec, codec) ||
		!samePointee(existing.LangRaw, langRaw) || !samePointee(existing.Lang, lang) ||
		existing.Disposition != track.disposition {
		err = work.queries.UpdateSidecarStream(ctx, sqlcgen.UpdateSidecarStreamParams{
			VideoFile: video.ID, Codec: codec, LangRaw: langRaw, Lang: lang, Disposition: track.disposition,
			ID: existing.ID,
		})
	}
	if err != nil {
		return fmt.Errorf("keep the track of %s: %w", sidecar.Path, err)
	}
	return nil
}
