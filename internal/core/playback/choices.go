// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"context"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/preferences"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// A person's choices in a file are kept by internal/core/preferences; playback lists the file's
// tracks they are made among and worked out against, and checks the file is playable, in the
// transaction that keeps them.

// ChooseSubtitle keeps a person's subtitle choice in a file they may play, a track of its or off,
// which the file's work's unwatched files follow (D230). What it reads, the file and its tracks, it
// reads in the transaction that keeps the choice, so a rescan committed meanwhile cannot leave it
// deciding on what is gone. A track the file does not have is preferences.ErrNoTrack.
func (service *Service) ChooseSubtitle(ctx context.Context, userID, fileID int64, choice preferences.Used) error {
	return service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		if _, err := service.playableFile(ctx, queries, fileID); err != nil {
			return err
		}
		tracks, err := service.textTracksOf(ctx, queries, fileID)
		if err != nil {
			return err
		}
		return service.preferences.ChooseSubtitle(ctx, queries, userID, fileID, choice, choiceTracks(tracks))
	})
}

// ChooseAudio keeps a person's choice of an audio track in a file they may play, which the file's
// work's unwatched files follow by its language (D230 §5), as ChooseSubtitle does.
func (service *Service) ChooseAudio(ctx context.Context, userID, fileID int64, track preferences.TrackRef) error {
	return service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		if _, err := service.playableFile(ctx, queries, fileID); err != nil {
			return err
		}
		audio, err := audioTracksOf(ctx, queries, fileID)
		if err != nil {
			return err
		}
		return service.preferences.ChooseAudio(ctx, queries, userID, fileID, track, audioChoiceTracks(audio))
	})
}

// SetSubtitleOffset keeps the person's subtitle offset for a file they may play.
func (service *Service) SetSubtitleOffset(ctx context.Context, userID, fileID, offsetMs int64) error {
	err := service.database.Write(ctx, func(queries *sqlcgen.Queries) error {
		return service.keepSubtitleOffset(ctx, queries, userID, fileID, offsetMs)
	})
	if err != nil {
		return fmt.Errorf("keep the subtitle offset for file %d: %w", fileID, err)
	}
	return nil
}

// keepSubtitleOffset keeps a subtitle offset for a file that queries, the transaction that keeps
// it, finds playable: a file a rescan removed meanwhile gets none.
func (service *Service) keepSubtitleOffset(
	ctx context.Context, queries *sqlcgen.Queries, userID, fileID, offsetMs int64,
) error {
	if _, err := service.playableFile(ctx, queries, fileID); err != nil {
		return err
	}
	return service.preferences.KeepSubtitleOffset(ctx, queries, userID, fileID, offsetMs)
}

// AudioTrack is one of a video's own audio tracks, as a person chooses among them.
type AudioTrack struct {
	// StreamID is the track's stream row.
	StreamID int64
	// AudioTrackIndex is the track's place among the file's audio tracks, as a prepared copy's audio
	// renditions name it.
	AudioTrackIndex int
	// StreamIndex is the track's index in its file, as FFmpeg numbers it.
	StreamIndex int64
	// Language is BCP 47, or empty when nothing named one.
	Language       string
	ContainerTitle string
	IsDefault      bool
	Kind           preferences.Kind
}

// audioTracksOf lists a video's own audio tracks, by their place among them.
func audioTracksOf(ctx context.Context, queries *sqlcgen.Queries, videoID int64) ([]AudioTrack, error) {
	rows, err := queries.ListAudioTracksOfVideo(ctx, videoID)
	if err != nil {
		return nil, fmt.Errorf("read the audio tracks of file %d: %w", videoID, err)
	}
	tracks := make([]AudioTrack, 0, len(rows))
	for index, row := range rows {
		kind := preferences.KindPlain
		described := library.DispositionBit("visual_impaired") | library.DispositionBit("descriptions")
		switch {
		case row.DispositionFlags&library.DispositionBit("comment") != 0:
			kind = preferences.KindCommentary
		case row.DispositionFlags&described != 0:
			kind = preferences.KindDescribed
		}
		tracks = append(tracks, AudioTrack{
			StreamID: row.ID, AudioTrackIndex: index, StreamIndex: ptr.ValueOrZero(row.StreamIndex),
			Language: ptr.ValueOrZero(row.Language), ContainerTitle: ptr.ValueOrZero(row.ContainerTitle),
			IsDefault: row.DispositionFlags&library.DispositionBit("default") != 0, Kind: kind,
		})
	}
	return tracks, nil
}

// subtitleKind is a subtitle track's kind, for a choice (D24).
func subtitleKind(track SubtitleTrack) preferences.Kind {
	switch {
	case track.IsForced:
		return preferences.KindForced
	case track.IsHearingImpaired:
		return preferences.KindHearingImpaired
	}
	return preferences.KindPlain
}

// choiceTracks are subtitle tracks as a choice sees them.
func choiceTracks(tracks []SubtitleTrack) []preferences.Track {
	described := make([]preferences.Track, 0, len(tracks))
	for _, track := range tracks {
		described = append(described, preferences.Track{TrackRef: track.TrackRef, Description: preferences.Description{
			Language: track.Language, Kind: subtitleKind(track), ContainerTitle: track.ContainerTitle,
		}})
	}
	return described
}

// audioChoiceTracks are audio tracks as a choice sees them.
func audioChoiceTracks(tracks []AudioTrack) []preferences.Track {
	described := make([]preferences.Track, 0, len(tracks))
	for _, track := range tracks {
		described = append(described, preferences.Track{
			TrackRef: preferences.TrackRef{StreamID: track.StreamID}, Description: preferences.Description{
				Language: track.Language, Kind: track.Kind, ContainerTitle: track.ContainerTitle,
			},
		})
	}
	return described
}
