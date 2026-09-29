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
	// AudioTrackIndex is the source audio track's place among its audio tracks.
	AudioTrackIndex int
	Language        string
	IsBoosted       bool
	// Levelling is the gain that brings the track's stereo sound to the level; the boosted mix
	// plays with the standard one's (D226 §3).
	Levelling Levelling
	// CarriedGainDB is the gain the rendition's sound already carries, which the player takes
	// from the levelling it applies (D226 §4).
	CarriedGainDB float64
}

// audioRenditions lists a prepare's audio renditions in the order the master playlist has them.
func audioRenditions(command nodeproto.Prepare) []AudioRendition {
	var renditions []AudioRendition
	for _, track := range command.Audio {
		standard := AudioRendition{AudioTrackIndex: track.AudioTrackIndex, Language: track.Language}
		renditions = append(renditions, standard)
		if track.ChannelCount > 2 {
			boosted := AudioRendition{AudioTrackIndex: track.AudioTrackIndex, Language: track.Language, IsBoosted: true}
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
	// fileID and profile name the rendition, for PrepareAgain.
	fileID  int64
	profile nodeproto.PrepareProfile
}

// HLSSourceOf is where the named file of the rendition a media token refers to is (cut-a §5).
func (service *Service) HLSSourceOf(
	ctx context.Context, token, name string, userID int64, sessionKey string,
) (HLSSource, error) {
	idle, err := service.tokenIdleExpiry(ctx)
	if err != nil {
		return HLSSource{}, err
	}
	playback, found := service.tokens.use(token, userID, sessionKey, service.clock.Now(), idle)
	if !found || playback.profile == "" {
		return HLSSource{}, ErrNoToken
	}
	rendition, err := store.OrNotFound(service.prepares.cache.Read().GetRenditionOfFileAndProfile(ctx,
		cachegen.GetRenditionOfFileAndProfileParams{
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
		Source:    Source{NodeID: rendition.NodeID, Target: target, ContentType: hlsContentType(name)},
		Readiness: readiness, fileID: playback.fileID, profile: playback.profile,
	}, nil
}

// PrepareAgain prepares a complete rendition again when its node no longer has it: evicted, or its
// directory lost (D217 §5). The player is told it is too early, and waits as for a new one.
func (service *Service) PrepareAgain(ctx context.Context, source HLSSource) error {
	file, err := service.playableFile(ctx, source.fileID)
	if err != nil {
		return err
	}
	return service.prepares.redo(ctx, file, source.profile)
}

// hlsContentType is a rendition file's media type, by its name.
func hlsContentType(name string) string {
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
	rendition, err := store.OrNotFound(service.prepares.cache.Read().GetRenditionByID(ctx, job.RenditionID))
	if errors.Is(err, store.ErrNotFound) {
		return jobs.MarkPermanent(fmt.Errorf("no rendition %d", job.RenditionID))
	}
	if err != nil {
		return fmt.Errorf("read rendition %d: %w", job.RenditionID, err)
	}
	order, err := service.prepareOrderFor(ctx, rendition)
	if errors.Is(err, ErrNoFile) {
		return jobs.MarkPermanent(errors.Join(err, service.prepares.finish(ctx, rendition.ID, nil, err.Error())))
	}
	if err != nil {
		return err
	}
	return service.prepares.handle(ctx, job, order)
}

// prepareOrder is a rendition to have made: the node's command, the node that holds the source, and
// the source's length in milliseconds, or 0 when its probe did not say.
type prepareOrder struct {
	command    nodeproto.Prepare
	nodeID     int64
	durationMs int64
	// carriedGainsJSON is the gains its audio tracks carry, as JSON, as its rendition records
	// them (D226 §4).
	carriedGainsJSON string
}

// prepareOrderFor is what a rendition's making needs.
func (service *Service) prepareOrderFor(
	ctx context.Context, rendition cachegen.PreparedRendition,
) (prepareOrder, error) {
	file, err := service.playableFile(ctx, rendition.SourceFile)
	if err != nil {
		return prepareOrder{}, err
	}
	probe, err := decodeProbe(file.ProbeJson)
	if err != nil {
		return prepareOrder{}, err
	}
	source, err := service.bytesFileOf(ctx, locatedOf(file))
	if err != nil {
		return prepareOrder{}, err
	}
	command := prepareCommand(source, file.ID, nodeproto.PrepareProfile(rendition.Profile), rendition.RunID, probe)
	command.SegmentLengthSeconds = int(rendition.SegmentLengthSeconds)
	levels, err := service.levellings(ctx, file.PayloadHash)
	if err != nil {
		return prepareOrder{}, err
	}
	gains := levelPrepare(&command, levels, probe)
	return prepareOrder{
		command: command, nodeID: file.NodeID, durationMs: probe.Format.DurationMs, carriedGainsJSON: gains,
	}, nil
}

// TakePrepareReport takes a node's report on a prepare to the job waiting for it.
func (service *Service) TakePrepareReport(ctx context.Context, nodeID int64, report nodeproto.PrepareReport) {
	service.prepares.reported(ctx, nodeID, report)
}
