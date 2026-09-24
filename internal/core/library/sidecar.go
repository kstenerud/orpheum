// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// filingAttached is a sidecar's filing: it belongs to a video beside it (schema.md, attached).
const filingAttached = "attached"

// sidecar records a metadata or subtitle file, keeping an NFO's bytes as the stored input
// identification reads (D182). Which video it belongs to is settled when the scan completes,
// because a sidecar can arrive before its video: "Film.en.srt" sorts before "Film.mkv".
func (work *batch) sidecar(ctx context.Context, scan *scanState, sidecar *nodeproto.Sidecar) error {
	filePath, raw := nodePath(scan.root, sidecar.Path)
	kind := string(sidecar.Kind)
	existing, err := store.One(work.queries.GetLiveFileByPath(ctx, sqlcgen.GetLiveFileByPathParams{
		NodeID: work.nodeID, Path: filePath,
	}))
	if err == nil {
		err = work.queries.UpdateSidecarFile(ctx, sqlcgen.UpdateSidecarFileParams{
			LibraryID: scan.libraryID, PathRaw: raw, Size: &sidecar.Size, Mtime: &sidecar.MtimeMs, Kind: &kind,
			Body: sidecar.Body, Seen: work.now, ID: existing.ID,
		})
		if err != nil {
			return fmt.Errorf("update the sidecar %s: %w", filePath, err)
		}
		return nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	_, err = work.queries.InsertSidecarFile(ctx, sqlcgen.InsertSidecarFileParams{
		LibraryID: scan.libraryID, NodeID: work.nodeID, Path: filePath, PathRaw: raw, Size: &sidecar.Size,
		Mtime: &sidecar.MtimeMs, Kind: &kind, Body: sidecar.Body, Seen: work.now,
	})
	if err != nil {
		return fmt.Errorf("insert the sidecar %s: %w", filePath, err)
	}
	return nil
}

// attachSidecars points each sidecar under the root at the video it belongs to: the video in
// its folder whose name it extends ("Film (2020).es.srt", "Film (2020).nfo" for
// "Film (2020).mkv"), or, for a "movie.nfo", the folder's only video. A sidecar that belongs to
// no single video — a tvshow.nfo, a season's — is attached to none.
func (work *batch) attachSidecars(ctx context.Context, scan *scanState) error {
	files, err := work.queries.ListLiveFilesForAttaching(ctx, sqlcgen.ListLiveFilesForAttachingParams{
		NodeID: work.nodeID, LibraryID: scan.libraryID,
	})
	if err != nil {
		return fmt.Errorf("list the files under %s: %w", scan.root, err)
	}
	videos := map[string][]sqlcgen.ListLiveFilesForAttachingRow{} // by folder
	for _, file := range files {
		if under(file.Path, scan.root) && file.PayloadHash != nil {
			videos[path.Dir(file.Path)] = append(videos[path.Dir(file.Path)], file)
		}
	}
	for _, file := range files {
		if file.Filing != filingAttached || !under(file.Path, scan.root) {
			continue
		}
		owner := ownerOf(file.Path, videos[path.Dir(file.Path)])
		if sameOwner(file.AttachedTo, owner) {
			continue
		}
		err := work.queries.SetFileAttachedTo(ctx, sqlcgen.SetFileAttachedToParams{AttachedTo: owner, ID: file.ID})
		if err != nil {
			return fmt.Errorf("attach %s: %w", file.Path, err)
		}
	}
	return nil
}

// ownerOf is the id of the video a sidecar at sidecarPath belongs to, or nil.
func ownerOf(sidecarPath string, videos []sqlcgen.ListLiveFilesForAttachingRow) *int64 {
	name := path.Base(sidecarPath)
	var owner *int64
	longest := 0
	for _, video := range videos {
		stem := strings.TrimSuffix(path.Base(video.Path), path.Ext(video.Path))
		// The longest stem wins, so "Film - 720p.srt" goes to "Film - 720p.mkv", not "Film.mkv".
		if strings.HasPrefix(name, stem+".") && len(stem) > longest {
			id := video.ID
			owner, longest = &id, len(stem)
		}
	}
	if owner == nil && strings.EqualFold(name, "movie.nfo") && len(videos) == 1 {
		id := videos[0].ID
		owner = &id
	}
	return owner
}

func sameOwner(current, owner *int64) bool {
	if current == nil || owner == nil {
		return current == owner
	}
	return *current == *owner
}
