// SPDX-License-Identifier: AGPL-3.0-or-later

package library

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// ScanRules are the library's rules for what a scan reads (D40, D240 §3): which files are videos
// and which text subtitles, by their extensions, lower case and without the dot. The rest are the
// fixed rules a scan has always kept: the root's .trash, samples, AppleDouble files.
type ScanRules struct {
	VideoExtensions    map[string]bool
	SubtitleExtensions map[string]bool
}

// TakenListing is what a listing leaves to do: the files the core needs read, how many files it
// found, and its end, which completes the scan once they are read.
type TakenListing struct {
	ToExamine   []nodeproto.ExamineFile
	ListedCount int
	Complete    nodeproto.ScanComplete
}

// TakeListing begins the scan of a node's root with its listing and compares what it lists with
// what is stored (D240 §3): the library's rules file what they skip; an image beside the media is
// recorded as it is listed (D215); a video, NFO or subtitle whose size and modified time are as
// stored, and which was read whole, was seen and needs nothing; the rest are returned, to be read.
// Mounts and errors are taken as a scan's are. A listing that was stopped ends as a scan the node
// stopped: unreadable, removing nothing (D39).
func (ingester *Ingester) TakeListing(
	ctx context.Context, nodeID int64, root, scope string, records []nodeproto.ScanRecord, rules ScanRules,
) (TakenListing, error) {
	return ingester.TakeListingReadingAgain(ctx, ListingScan{NodeID: nodeID, Root: root, Scope: scope}, records,
		rules)
}

// ListingScan is the scan a listing is of: its node's root, what it lists of it, and what it reads
// again.
type ListingScan struct {
	NodeID      int64
	Root, Scope string
	ReadAgain   ReadAgain
}

// TakeListingReadingAgain is TakeListing for a scan that reads again what its ReadAgain names, a
// video, NFO or subtitle stored as listed among them, so its probe is compared with the one kept, and,
// when it names any, every NFO and subtitle file the listing holds (D392 §2, §4).
func (ingester *Ingester) TakeListingReadingAgain(
	ctx context.Context, scan ListingScan, records []nodeproto.ScanRecord, rules ScanRules,
) (TakenListing, error) {
	taken := TakenListing{Complete: nodeproto.ScanComplete{RootPath: scan.Root, IsStoppedByErrors: true}}
	err := ingester.write(ctx, scanKey{nodeID: scan.NodeID, root: scan.Root}, func(batch *ingestBatch) error {
		listing := &listingComparison{
			batch: batch, rules: rules, taken: &taken, scope: cleanScope(scan.Scope), listedPaths: map[string]bool{},
			readAgain: scan.ReadAgain.cleaned(),
		}
		for index, record := range records {
			if err := listing.take(ctx, cleanRecord(record)); err != nil {
				return fmt.Errorf("listing record %d (%s): %w", index, record.Type, err)
			}
		}
		if err := listing.concludeInodeMoves(ctx); err != nil {
			return err
		}
		return listing.markSeen(ctx)
	})
	return taken, err
}

// listingComparison is a listing being compared with what is stored.
type listingComparison struct {
	batch *ingestBatch
	rules ScanRules
	taken *TakenListing
	// scope is what the listing covers, relative to the root (D272 §9).
	scope string
	// stored are the root's library's live files on the node, by the bytes they were reported by,
	// once the scan has begun.
	stored map[string]sqlcgen.ListFilesToCompareRow
	// skippedDirectories are the directories a rule skipped, relative to the root: nothing below
	// them is the library's.
	skippedDirectories []string
	// seen are the stored files found as listed, not yet marked seen: marked together, before any
	// record that might read when a file was seen, and at the listing's end (markSeen).
	seen []int64
	// listedPaths are the paths the listing named, by their reported bytes, so a stored file listed
	// where it was is never taken for one moved.
	listedPaths map[string]bool
	// newVideos are the videos listed at a path nothing is stored at, with a device and an inode:
	// each is a stored file moved (concludeInodeMoves) or else read.
	newVideos []listedVideo
	// readAgain is what the listing has read again though stored as listed (D392 §2).
	readAgain ReadAgain
}

// listedVideo is a video a listing found at a path nothing is stored at.
type listedVideo struct {
	listed  *nodeproto.Listed
	located reportedPath
	scan    *scanState
}

// markSeen marks the stored files found as listed seen now, in one statement (D303, DL7).
func (listing *listingComparison) markSeen(ctx context.Context) error {
	if len(listing.seen) == 0 {
		return nil
	}
	ids, err := json.Marshal(listing.seen)
	if err != nil {
		return fmt.Errorf("list the files seen: %w", err)
	}
	err = listing.batch.queries.SetFilesSeen(ctx, sqlcgen.SetFilesSeenParams{
		SeenAt: listing.batch.nowMs, IdsJSON: string(ids),
	})
	if err != nil {
		return fmt.Errorf("mark %d files seen: %w", len(listing.seen), err)
	}
	listing.seen = listing.seen[:0]
	return nil
}

// take compares one record of the listing.
func (listing *listingComparison) take(ctx context.Context, record nodeproto.ScanRecord) error {
	batch := listing.batch
	switch record.Type {
	case nodeproto.RecordScanBegin:
		if err := batch.ingestRecord(ctx, record); err != nil {
			return err
		}
		if scan := batch.ingester.scans[batch.key]; scan != nil {
			scan.scope, scan.isReadingAgain = listing.scope, len(listing.readAgain) > 0
		}
		return listing.readStored(ctx)
	case nodeproto.RecordListed:
		if scan := batch.scanInProgress(); scan != nil {
			return listing.takeListed(ctx, scan, record.Listed)
		}
		return nil
	case nodeproto.RecordScanComplete:
		listing.taken.Complete = *record.ScanComplete
		return nil
	case nodeproto.RecordObserved, nodeproto.RecordSidecar, nodeproto.RecordSkip, nodeproto.RecordError,
		nodeproto.RecordMount:
	}
	if err := listing.markSeen(ctx); err != nil {
		return err
	}
	return batch.ingestRecord(ctx, record)
}

// readStored reads the places under the scan's root on the node, whichever libraries cover them.
func (listing *listingComparison) readStored(ctx context.Context) error {
	scan, batch := listing.batch.scanInProgress(), listing.batch
	if scan == nil {
		return nil
	}
	after, before := PathRange(scan.root)
	rows, err := batch.queries.ListFilesToCompare(ctx, sqlcgen.ListFilesToCompareParams{
		NodeID: batch.nodeID, After: after, Before: before,
	})
	if err != nil {
		return fmt.Errorf("list the files under %s: %w", scan.root, err)
	}
	listing.stored = make(map[string]sqlcgen.ListFilesToCompareRow, len(rows))
	for _, row := range rows {
		listing.stored[string(row.ReportedPathRaw)] = row
	}
	return nil
}

// takeListed files one listed entry by the library's rules, and says whether it needs reading.
func (listing *listingComparison) takeListed(ctx context.Context, scan *scanState, listed *nodeproto.Listed) error {
	relative := string(listed.Path.Bytes())
	for _, skipped := range listing.skippedDirectories {
		if strings.HasPrefix(relative, skipped+"/") {
			return nil
		}
	}
	if listed.IsDirectory {
		rule, isSkipped := directoryRule(relative)
		if !isSkipped {
			return nil
		}
		listing.skippedDirectories = append(listing.skippedDirectories, relative)
		return listing.batch.recordFiling(ctx, scan, listed.Path, FilingSkipped, string(rule), nil)
	}
	listing.taken.ListedCount++
	kind, rule := listing.rules.kindOf(path.Base(relative))
	switch {
	case rule != "":
		return listing.batch.recordFiling(ctx, scan, listed.Path, FilingSkipped, string(rule), nil)
	case kind == "":
		return listing.batch.recordSidecar(ctx, scan, &nodeproto.Sidecar{
			Path: listed.Path, Kind: nodeproto.SidecarImage, SizeBytes: listed.SizeBytes,
			ModifiedAt: listed.ModifiedAt,
		})
	}
	located := resolveReportedPath(scan.root, listed.Path)
	listing.listedPaths[string(located.reportedBytes)] = true
	stored, isStored := listing.stored[string(located.reportedBytes)]
	if isStored && isStoredAsListed(stored, kind, listed) && !listing.readsAgain(kind, relative) {
		listing.seen = append(listing.seen, stored.ID)
		return nil
	}
	if !isStored && kind == nodeproto.ExamineVideo {
		listing.newVideos = append(listing.newVideos, listedVideo{listed: listed, located: located, scan: scan})
	}
	listing.taken.ToExamine = append(listing.taken.ToExamine, nodeproto.ExamineFile{Path: listed.Path, Kind: kind})
	return nil
}

// readsAgain is whether a file stored as listed, of kind at relative, is read again all the same: a
// video the scan reads again, or any NFO or subtitle file of a scan that reads anything again (D392
// §2, §4).
func (listing *listingComparison) readsAgain(kind nodeproto.ExamineKind, relative string) bool {
	if len(listing.readAgain) == 0 {
		return false
	}
	return kind != nodeproto.ExamineVideo || listing.readAgain.covers(relative)
}

// storeUnread stores a new video as the listing found it, unread: its path, size, modified time,
// device and inode, with no payload hash yet. The pass files it from its path at once, and it is
// read later; its read fills its row in (D304 §1, §4).
func (listing *listingComparison) storeUnread(ctx context.Context, video listedVideo) error {
	reason := notIdentifiedYet
	_, err := listing.batch.queries.InsertFile(ctx, sqlcgen.InsertFileParams{
		NodeID: listing.batch.nodeID, Path: video.located.text,
		ReportedPathRaw: video.located.reportedBytes, SizeBytes: &video.listed.SizeBytes,
		ModifiedAt: &video.listed.ModifiedAt, Filing: FilingVideo, FilingReason: &reason,
		SeenAt:           listing.batch.nowMs,
		FilesystemDevice: storedNumber(video.listed.FilesystemDevice), Inode: storedNumber(video.listed.Inode),
	})
	if err != nil {
		return fmt.Errorf("store %s unread: %w", video.located.text, err)
	}
	listing.batch.noteNewFile(video.scan)
	return nil
}

// diskPlace is a file's device and inode.
type diskPlace struct{ device, inode int64 }

// movableVideos are the stored videos, read whole, the listing did not name where they are stored,
// by the device and inode they were read at: each may have moved to a path the listing named.
func (listing *listingComparison) movableVideos() map[diskPlace]sqlcgen.ListFilesToCompareRow {
	movable := map[diskPlace]sqlcgen.ListFilesToCompareRow{}
	for reported, stored := range listing.stored {
		if listing.listedPaths[reported] || stored.FilesystemDevice == nil || stored.Inode == nil ||
			!isStoredAsListed(stored, nodeproto.ExamineVideo, &nodeproto.Listed{
				SizeBytes: ptr.ValueOrZero(stored.SizeBytes), ModifiedAt: ptr.ValueOrZero(stored.ModifiedAt),
			}) {
			continue
		}
		movable[diskPlace{*stored.FilesystemDevice, *stored.Inode}] = stored
	}
	return movable
}

// concludeInodeMoves settles each new video the listing held: a stored video, read whole, on the
// same device and inode with the same size and modified time, that the listing did not name where
// it is stored and that the scan covers, moved there, and its row and history follow it without
// its being read again (D304 §5, D240 §3); any other is stored unread and read. A folder's scan
// never takes a file outside the folder for one moved into it (D272 §9).
func (listing *listingComparison) concludeInodeMoves(ctx context.Context) error {
	if len(listing.newVideos) == 0 {
		return nil
	}
	movable := listing.movableVideos()
	moved := map[string]bool{} // by the path listed, those not to read
	for _, video := range listing.newVideos {
		device, inode := storedNumber(video.listed.FilesystemDevice), storedNumber(video.listed.Inode)
		var stored sqlcgen.ListFilesToCompareRow
		found := device != nil && inode != nil
		if found {
			stored, found = movable[diskPlace{*device, *inode}]
		}
		if !found || !isAsListed(stored, video.listed) || !video.scan.covers(stored.ReportedPathRaw) {
			if err := listing.storeUnread(ctx, video); err != nil {
				return err
			}
			continue
		}
		delete(movable, diskPlace{*device, *inode})
		moved[video.listed.Path.Text] = true
		err := listing.batch.queries.MoveFileTo(ctx, sqlcgen.MoveFileToParams{
			Path: video.located.text, ReportedPathRaw: video.located.reportedBytes, SeenAt: listing.batch.nowMs,
			ID: stored.ID,
		})
		if err != nil {
			return fmt.Errorf("move file %d to %s: %w", stored.ID, video.located.text, err)
		}
		listing.batch.noteNewFile(video.scan) // its pass identifies it again from its new path
	}
	listing.taken.ToExamine = slices.DeleteFunc(listing.taken.ToExamine, func(file nodeproto.ExamineFile) bool {
		return file.Kind == nodeproto.ExamineVideo && moved[file.Path.Text]
	})
	return nil
}

// directoryRule is the rule a directory, relative to the root, is skipped by: the root's .trash,
// where an export keeps the files it replaced (D144), a release's sample/ (D162), a folder a NAS
// keeps for itself, or a disc's folder (D306 §5).
func directoryRule(relative string) (nodeproto.SkipRule, bool) {
	switch {
	case relative == ".trash":
		return nodeproto.SkipTrash, true
	case strings.EqualFold(path.Base(relative), "sample"):
		return nodeproto.SkipSample, true
	case isSystemFolder(path.Base(relative)):
		return nodeproto.SkipSystem, true
	case strings.EqualFold(path.Base(relative), "VIDEO_TS") || strings.EqualFold(path.Base(relative), "BDMV"):
		return nodeproto.SkipDiscFolder, true
	}
	return "", false
}

// isSystemFolder is a folder, by its name, that a NAS or an operating system keeps for itself at
// any depth: Synology's, QNAP's, ZFS's and NetApp's snapshots, Windows' and macOS's bins and
// indexes, and a filesystem's lost+found. Jellyfin, Kodi and Plex's scanners skip the same.
func isSystemFolder(name string) bool {
	switch strings.ToLower(name) {
	case "@eadir", "#recycle", "@recycle", "#snapshot", "@recently-snapshot", ".@__thumb", ".snapshot", ".snapshots",
		".zfs", "$recycle.bin", "system volume information", "lost+found", ".trashes", ".spotlight-v100", ".fseventsd",
		".appledouble", ".temporaryitems", ".documentrevisions-v100":
		return true
	}
	return false
}

// discImageExtension is a DVD's or a Blu-ray's image, read as a video whatever the video extensions
// say: its node serves its main title (D350).
const discImageExtension = "iso"

// kindOf is how a file named name is read: as a video, an NFO or a subtitle; "" and no rule for an
// image beside the media, which is not read until it is needed (D215); or the rule that skips it.
func (rules ScanRules) kindOf(name string) (nodeproto.ExamineKind, nodeproto.SkipRule) {
	if strings.HasPrefix(name, "._") {
		return "", nodeproto.SkipAppleDouble
	}
	extension := strings.ToLower(strings.TrimPrefix(path.Ext(name), "."))
	switch {
	case extension == "nfo":
		return nodeproto.ExamineNFO, ""
	case rules.SubtitleExtensions[extension]:
		return nodeproto.ExamineSubtitle, ""
	case isImage(extension):
		return "", ""
	case !rules.VideoExtensions[extension] && extension != discImageExtension:
		return "", nodeproto.SkipExtension
	case isSampleFile(name):
		return "", nodeproto.SkipSample
	}
	return nodeproto.ExamineVideo, ""
}

// isStoredAsListed is whether a stored file is the one listed, read whole, as a file of kind: its size and
// modified time as stored, its identity confirmed (D179), and what reading it gave kept: a video's
// payload hash, an NFO's bytes, a subtitle's hash. A file that was skipped or could not be read is
// read again.
func isStoredAsListed(stored sqlcgen.ListFilesToCompareRow, kind nodeproto.ExamineKind, listed *nodeproto.Listed) bool {
	if !isAsListed(stored, listed) || stored.IsIdentityConfirmed == 0 {
		return false
	}
	sidecarKind := ptr.ValueOrZero(stored.SidecarKind)
	switch kind {
	case nodeproto.ExamineVideo:
		return sidecarKind == "" && stored.HasPayloadHash == 1 && stored.Filing != FilingSkipped &&
			stored.Filing != FilingErrored && stored.Filing != FilingAttached
	case nodeproto.ExamineNFO:
		return sidecarKind == string(nodeproto.SidecarNFO) && stored.Filing == FilingAttached && stored.HasNFOBody == 1
	case nodeproto.ExamineSubtitle:
		return sidecarKind == string(nodeproto.SidecarSubtitle) && stored.Filing == FilingAttached &&
			stored.HasPayloadHash == 1
	}
	return false
}

// isAsListed is whether a stored file's size and modified time are the listed ones.
func isAsListed(stored sqlcgen.ListFilesToCompareRow, listed *nodeproto.Listed) bool {
	return stored.SizeBytes != nil && *stored.SizeBytes == listed.SizeBytes && stored.ModifiedAt != nil &&
		*stored.ModifiedAt == listed.ModifiedAt
}

// isImage is an image extension a poster or backdrop beside the media is saved with (D215).
func isImage(extension string) bool {
	switch extension {
	case "jpg", "jpeg", "png", "webp", "tbn":
		return true
	}
	return false
}

// isSampleFile is sample.mkv, x.sample.mkv or x-sample.mkv, in any case (D162).
func isSampleFile(name string) bool {
	stem := strings.ToLower(strings.TrimSuffix(name, path.Ext(name)))
	return stem == "sample" || strings.HasSuffix(stem, ".sample") || strings.HasSuffix(stem, "-sample")
}
