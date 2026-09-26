// SPDX-License-Identifier: GPL-3.0-or-later

package playback

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strconv"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
)

// AudioRendition is one audio rendition of a prepared playback: a source track's standard mix,
// or its boosted one (D217 §3).
type AudioRendition struct {
	// Track is the source audio track's place among its audio tracks.
	Track    int
	Language string
	Boost    bool
}

// audioRenditions lists a prepare's audio renditions in the order the master playlist has them.
func audioRenditions(command nodeproto.Prepare) []AudioRendition {
	var renditions []AudioRendition
	for _, track := range command.Audio {
		renditions = append(renditions, AudioRendition{Track: track.AudioTrackIndex, Language: track.Language})
		if track.Channels > 2 {
			boosted := AudioRendition{Track: track.AudioTrackIndex, Language: track.Language, Boost: true}
			renditions = append(renditions, boosted)
		}
	}
	return renditions
}

// Readiness is how far a prepared playback's rendition is.
type Readiness string

// The readinesses: still being prepared, complete, or failed.
const (
	Preparing Readiness = "preparing"
	Complete  Readiness = "complete"
	Failed    Readiness = "failed"
)

// HLSSource is where one file of a prepared playback's rendition is, and how far the rendition is.
type HLSSource struct {
	Source
	Readiness Readiness
	// fileID and profile name the rendition, for Lost.
	fileID  int64
	profile nodeproto.PrepareProfile
}

// HLS is where the named file of the rendition a media token refers to is (cut-a §5).
func (service *Service) HLS(
	ctx context.Context, token, name string, userID int64, sessionKey string,
) (HLSSource, error) {
	idle, err := service.idle(ctx)
	if err != nil {
		return HLSSource{}, err
	}
	playback, found := service.tokens.use(token, userID, sessionKey, service.clock.Now(), idle)
	if !found || playback.profile == "" {
		return HLSSource{}, ErrNoToken
	}
	rendition, err := store.One(service.prepares.cache.Read().GetRendition(ctx, cachegen.GetRenditionParams{
		SourceFile: playback.fileID, Profile: string(playback.profile),
	}))
	if errors.Is(err, store.ErrNotFound) {
		return HLSSource{}, ErrNoToken
	}
	if err != nil {
		return HLSSource{}, fmt.Errorf("read the rendition: %w", err)
	}
	readiness := Preparing
	switch rendition.State {
	case renditionComplete:
		readiness = Complete
	case renditionFailed:
		readiness = Failed
	}
	target := nodeproto.PathBytesPrepared + strconv.FormatInt(playback.fileID, 10) + "/" +
		string(playback.profile) + "/" + rendition.RunID + "/" + name
	return HLSSource{
		Source:    Source{NodeID: rendition.NodeID, Target: target, ContentType: hlsType(name)},
		Readiness: readiness, fileID: playback.fileID, profile: playback.profile,
	}, nil
}

// Lost prepares a complete rendition again when its node no longer has it: evicted, or its
// directory lost (D217 §5). The player is told it is too early, and waits as for a new one.
func (service *Service) Lost(ctx context.Context, source HLSSource) error {
	file, err := service.file(ctx, source.fileID)
	if err != nil {
		return err
	}
	return service.prepares.redo(ctx, file, source.profile)
}

// hlsType is a rendition file's media type, by its name.
func hlsType(name string) string {
	switch path.Ext(name) {
	case ".m3u8":
		return "application/vnd.apple.mpegurl"
	case ".m4s":
		return "video/iso.segment"
	}
	return "video/mp4"
}

// HandlePrepare runs one prepare job (jobs.KindPrepare): the node that holds the source makes the
// rendition, and this waits for it.
func (service *Service) HandlePrepare(ctx context.Context, job jobs.Job) error {
	rendition, err := store.One(service.prepares.cache.Read().GetRenditionByID(ctx, job.Subject))
	if errors.Is(err, store.ErrNotFound) {
		return jobs.Permanent(fmt.Errorf("no rendition %d", job.Subject))
	}
	if err != nil {
		return fmt.Errorf("read rendition %d: %w", job.Subject, err)
	}
	command, nodeID, err := service.commandFor(ctx, rendition)
	if errors.Is(err, ErrNoFile) {
		return jobs.Permanent(errors.Join(err, service.prepares.finish(ctx, rendition.ID, nil, err.Error())))
	}
	if err != nil {
		return err
	}
	return service.prepares.handle(ctx, job, command, nodeID)
}

// commandFor is the node's command for a rendition, and the node that holds its source.
func (service *Service) commandFor(
	ctx context.Context, rendition cachegen.PreparedRendition,
) (nodeproto.Prepare, int64, error) {
	file, err := service.file(ctx, rendition.SourceFile)
	if err != nil {
		return nodeproto.Prepare{}, 0, err
	}
	probe, err := decodeProbe(file.Probe)
	if err != nil {
		return nodeproto.Prepare{}, 0, err
	}
	source, err := service.source(ctx, locatedOf(file))
	if err != nil {
		return nodeproto.Prepare{}, 0, err
	}
	command := prepareCommand(source, file.ID, nodeproto.PrepareProfile(rendition.Profile), rendition.RunID, probe)
	command.SegmentLengthSeconds = int(rendition.SegmentLengthS)
	return command, file.NodeID, nil
}

// Reported takes a node's report on a prepare to the job waiting for it.
func (service *Service) Reported(ctx context.Context, nodeID int64, report nodeproto.PrepareReport) {
	service.prepares.reported(ctx, nodeID, report)
}
