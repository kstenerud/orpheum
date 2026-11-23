// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"context"
	"errors"
	"fmt"

	"github.com/kstenerud/orpheum/internal/core/auth"
	"github.com/kstenerud/orpheum/internal/core/decide"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

var (
	// ErrPlaceOutsideFilm is a key frame asked for at a place before the film's start or past its end.
	ErrPlaceOutsideFilm = errors.New("the place is not in the film")
	// ErrNoPicture is a key frame asked of a file with no picture stream.
	ErrNoPicture = errors.New("the file has no picture")
)

// frameContentType is what a key frame is: the node's own WebP (D545, D212 §1).
const frameContentType = "image/webp"

// FrameSource is where the key frame at or before atMs of a file's main picture is made, for a cast's
// remote (D545): the node of a place reach reaches, the first that can be read now, as a direct play
// reads one (D363 §8), asked for the keyframe on its byte socket. A place before 0 or past the film's
// length is ErrPlaceOutsideFilm, and a file with no picture ErrNoPicture.
func (service *Service) FrameSource(ctx context.Context, reach auth.Reach, fileID, atMs int64) (Source, error) {
	file, err := library.PlayableFile(ctx, service.database.Read(), fileID)
	if err != nil {
		return Source{}, err
	}
	probe, err := library.ProbeOf(file.ProbeJSON)
	if err != nil {
		return Source{}, err
	}
	picture, hasPicture := decide.MainVideo(probe)
	if !hasPicture {
		return Source{}, fmt.Errorf("file %d: %w", fileID, ErrNoPicture)
	}
	if atMs < 0 || atMs > probe.Format.DurationMs {
		return Source{}, fmt.Errorf("%d ms of file %d, %d ms long: %w", atMs, fileID, probe.Format.DurationMs,
			ErrPlaceOutsideFilm)
	}
	place, err := library.PlaceToRead(ctx, service.database.Read(), service.nodes, service.absences, reach, file)
	if err != nil {
		return Source{}, err
	}
	bytesFile, err := library.BytesFileOf(ctx, service.database.Read(), place)
	if err != nil {
		return Source{}, err
	}
	request := nodeproto.FrameRequest{Video: bytesFile, StreamIndex: picture.StreamIndex, AtMs: atMs}
	return Source{
		NodeID: place.NodeID, Target: request.Target(), ContentType: frameContentType,
		PlacePath: place.ReportedPathRaw,
	}, nil
}
