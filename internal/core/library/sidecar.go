// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// attachedReason is why a sidecar is filed attached, as every filing but listed says why (D163):
// what kind of file it is, beside which video. Its kind is kept apart, in sidecar_kind (D227).
func attachedReason(kind nodeproto.SidecarKind) string {
	switch kind {
	case nodeproto.SidecarNFO:
		return "an NFO beside its video"
	case nodeproto.SidecarSubtitle:
		return "a subtitle beside its video"
	case nodeproto.SidecarImage:
		return "an image beside its video"
	}
	return "a file beside its video"
}

// recordSidecar records a metadata or subtitle file, keeping an NFO's bytes as the stored input
// identification reads (D182). Which video it belongs to is settled when the scan completes,
// because a sidecar can arrive before its video: "Film.en.srt" sorts before "Film.mkv".
func (batch *ingestBatch) recordSidecar(ctx context.Context, scan *scanState, sidecar *nodeproto.Sidecar) error {
	located := resolveReportedPath(scan.root, sidecar.Path)
	kind, reason := string(sidecar.Kind), attachedReason(sidecar.Kind)
	var hash []byte
	var payloadBytes *int64
	if sidecar.Identity != nil {
		decoded, hashErr := decodePayloadHash(*sidecar.Identity)
		if hashErr != nil {
			return fmt.Errorf("the sidecar %s: %w", located.text, hashErr)
		}
		hash, payloadBytes = decoded, &sidecar.Identity.PayloadBytes
	}
	existing, err := store.OrNotFound(batch.queries.GetLiveFileByReportedPathBytes(ctx,
		sqlcgen.GetLiveFileByReportedPathBytesParams{
			NodeID: batch.nodeID, ReportedPathBytes: located.reportedBytes,
		}))
	if err == nil {
		err = batch.queries.UpdateSidecarFile(ctx, sqlcgen.UpdateSidecarFileParams{
			LibraryID: scan.libraryID, SizeBytes: &sidecar.SizeBytes, ModifiedAt: &sidecar.ModifiedAt,
			PayloadHash: hash, PayloadBytes: payloadBytes, SidecarKind: &kind,
			FilingReason: &reason,
			NfoBody:      sidecar.NFOBody, SeenAt: batch.nowMs, ID: existing.ID,
		})
		if err != nil {
			return fmt.Errorf("update the sidecar %s: %w", located.text, err)
		}
		return nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	_, err = batch.queries.InsertSidecarFile(ctx, sqlcgen.InsertSidecarFileParams{
		LibraryID: scan.libraryID, NodeID: batch.nodeID, Path: located.text, ReportedPathBytes: located.reportedBytes,
		SizeBytes: &sidecar.SizeBytes, ModifiedAt: &sidecar.ModifiedAt, PayloadHash: hash, PayloadBytes: payloadBytes,
		SidecarKind: &kind, FilingReason: &reason,
		NfoBody: sidecar.NFOBody, SeenAt: batch.nowMs,
	})
	if err != nil {
		return fmt.Errorf("insert the sidecar %s: %w", located.text, err)
	}
	return nil
}

// attachSidecars points each sidecar under the root at the video it belongs to: the video in
// its folder whose name it extends ("Film (2020).es.srt", "Film (2020).nfo" for
// "Film (2020).mkv"), or, for a "movie.nfo", the folder's only video. A sidecar that belongs to
// no single video — a tvshow.nfo, a season's — is attached to none. A subtitle's track is kept
// beside its video's own (keepSidecarTrack).
func (batch *ingestBatch) attachSidecars(ctx context.Context, scan *scanState) error {
	files, err := batch.queries.ListLiveFilesForAttaching(ctx, sqlcgen.ListLiveFilesForAttachingParams{
		NodeID: batch.nodeID, LibraryID: scan.libraryID,
	})
	if err != nil {
		return fmt.Errorf("list the files under %s: %w", scan.root, err)
	}
	videosByFolder := map[string][]sqlcgen.ListLiveFilesForAttachingRow{} // by folder
	isInRoot := func(file sqlcgen.ListLiveFilesForAttachingRow) bool {
		_, under := RelativeToRoot(file.ReportedPathBytes, scan.root)
		return under
	}
	for _, file := range files {
		// A subtitle sidecar has a payload hash too; a video is what has one and is not attached.
		if isInRoot(file) && file.PayloadHash != nil && file.Filing != FilingAttached {
			videosByFolder[path.Dir(file.Path)] = append(videosByFolder[path.Dir(file.Path)], file)
		}
	}
	for _, file := range files {
		if file.Filing != FilingAttached || !isInRoot(file) {
			continue
		}
		folderVideos := videosByFolder[path.Dir(file.Path)]
		ownerVideoID := ownerVideoIDOf(file.Path, folderVideos)
		if ptr.ValueOrZero(file.SidecarKind) == string(nodeproto.SidecarSubtitle) {
			if err := batch.keepSidecarTrack(ctx, file, videoOf(ownerVideoID, folderVideos)); err != nil {
				return err
			}
		}
		if samePointee(file.AttachedToFile, ownerVideoID) {
			continue
		}
		attach := sqlcgen.SetFileAttachedToParams{AttachedToFile: ownerVideoID, ID: file.ID}
		err := batch.queries.SetFileAttachedTo(ctx, attach)
		if err != nil {
			return fmt.Errorf("attach %s: %w", file.Path, err)
		}
	}
	return nil
}

// ownerVideoIDOf is the id of the video a sidecar at sidecarPath belongs to, or nil.
func ownerVideoIDOf(sidecarPath string, videos []sqlcgen.ListLiveFilesForAttachingRow) *int64 {
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

// samePointee is whether two optional values are both absent, or both present and equal.
func samePointee[T comparable](stored, derived *T) bool {
	if stored == nil || derived == nil {
		return stored == derived
	}
	return *stored == *derived
}
