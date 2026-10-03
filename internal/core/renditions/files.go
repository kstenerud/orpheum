// SPDX-License-Identifier: GPL-3.0-or-later

package renditions

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strconv"

	"github.com/kstenerud/orpheum/internal/core/jobs"
	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/cachegen"
)

// ErrNoRendition is a rendition of a file in a profile that no play has wanted.
var ErrNoRendition = errors.New("no such rendition")

// File is where one file of a rendition is, and how far the rendition is.
type File struct {
	NodeID int64
	// Target is the file's path on its node's byte server.
	Target      string
	ContentType string
	Readiness   jobs.Readiness
}

// FileOf is where the named file of the rendition of a file in profile is (cut-a §5). A rendition
// being made that is asked for keeps its prepare wanted: a player asking for it is a person still
// waiting for it (D234 §3).
func (service *Service) FileOf(
	ctx context.Context, fileID int64, profile nodeproto.PrepareProfile, name string,
) (File, error) {
	rendition, err := store.OrNotFound(service.cache.Read().GetRenditionOfFileAndProfile(ctx,
		cachegen.GetRenditionOfFileAndProfileParams{SourceFile: fileID, Profile: string(profile)}))
	if errors.Is(err, store.ErrNotFound) {
		return File{}, ErrNoRendition
	}
	if err != nil {
		return File{}, fmt.Errorf("read the rendition: %w", err)
	}
	readiness := jobs.Preparing
	switch rendition.State {
	case renditionComplete:
		readiness = jobs.Complete
	case renditionFailed:
		readiness = jobs.Failed
	}
	if readiness == jobs.Preparing {
		err = service.queue.KeepWanted(ctx, jobs.KindPrepare, jobs.Target{RenditionID: rendition.ID})
		if err != nil {
			return File{}, err
		}
	}
	target := nodeproto.PathBytesPrepared + strconv.FormatInt(fileID, 10) + "/" +
		string(profile) + "/" + rendition.RunID + "/" + name
	return File{NodeID: rendition.NodeID, Target: target, ContentType: contentType(name), Readiness: readiness}, nil
}

// PrepareAgain prepares the complete rendition of a file in profile again when its node no longer
// has it: evicted, or its directory lost (D217 §5).
func (service *Service) PrepareAgain(ctx context.Context, fileID int64, profile nodeproto.PrepareProfile) error {
	file, err := library.PlayableFile(ctx, service.database.Read(), fileID)
	if err != nil {
		return err
	}
	return service.redo(ctx, file, profile)
}

// contentType is a rendition file's media type, by its name.
func contentType(name string) string {
	switch path.Ext(name) {
	case ".m3u8":
		return "application/vnd.apple.mpegurl"
	case ".m4s":
		return "video/iso.segment"
	}
	return "video/mp4"
}
