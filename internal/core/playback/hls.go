// SPDX-License-Identifier: AGPL-3.0-or-later

package playback

import (
	"context"
	"errors"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/renditions"
	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// HLSSource is where one file of a prepared playback's rendition is, and how far the rendition is.
type HLSSource struct {
	Source
	Readiness jobs.Readiness
	// fileID and profile name the rendition, for PrepareAgain.
	fileID  int64
	profile nodeproto.PrepareProfile
}

// HLSSourceOf is where the named file of the rendition a media token refers to is (cut-a §5).
func (service *Service) HLSSourceOf(
	ctx context.Context, token, name string, holder Holder,
) (HLSSource, error) {
	idle, err := service.tokenIdleExpiry(ctx)
	if err != nil {
		return HLSSource{}, err
	}
	playback, found := service.tokens.use(token, holder, service.clock.Now(), idle)
	if !found || playback.profile == "" {
		return HLSSource{}, ErrNoToken
	}
	file, err := service.renditions.FileOf(ctx, playback.fileID, playback.profile, name)
	if errors.Is(err, renditions.ErrNoRendition) {
		return HLSSource{}, ErrNoToken
	}
	if err != nil {
		return HLSSource{}, err
	}
	return HLSSource{
		Source:    Source{NodeID: file.NodeID, Target: file.Target, ContentType: file.ContentType},
		Readiness: file.Readiness, fileID: playback.fileID, profile: playback.profile,
	}, nil
}

// PrepareAgain prepares a complete rendition again when its node no longer has it: evicted, or its
// directory lost (D217 §5). The player is told it is too early, and waits as for a new one.
func (service *Service) PrepareAgain(ctx context.Context, source HLSSource) error {
	return service.renditions.PrepareAgain(ctx, source.fileID, source.profile)
}
