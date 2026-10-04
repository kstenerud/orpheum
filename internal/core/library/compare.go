// SPDX-License-Identifier: GPL-3.0-or-later

package library

import (
	"context"
	"fmt"
	"path"
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
	ctx context.Context, nodeID int64, root string, records []nodeproto.ScanRecord, rules ScanRules,
) (TakenListing, error) {
	taken := TakenListing{Complete: nodeproto.ScanComplete{RootPath: root, IsStoppedByErrors: true}}
	err := ingester.write(ctx, scanKey{nodeID: nodeID, root: root}, func(batch *ingestBatch) error {
		listing := &listingComparison{batch: batch, rules: rules, taken: &taken}
		for index, record := range records {
			if err := listing.take(ctx, cleanRecord(record)); err != nil {
				return fmt.Errorf("listing record %d (%s): %w", index, record.Type, err)
			}
		}
		return nil
	})
	return taken, err
}

// listingComparison is a listing being compared with what is stored.
type listingComparison struct {
	batch *ingestBatch
	rules ScanRules
	taken *TakenListing
	// stored are the root's library's live files on the node, by the bytes they were reported by,
	// once the scan has begun.
	stored map[string]sqlcgen.ListFilesToCompareRow
	// skippedDirectories are the directories a rule skipped, relative to the root: nothing below
	// them is the library's.
	skippedDirectories []string
}

// take compares one record of the listing.
func (listing *listingComparison) take(ctx context.Context, record nodeproto.ScanRecord) error {
	batch := listing.batch
	switch record.Type {
	case nodeproto.RecordScanBegin:
		if err := batch.ingestRecord(ctx, record); err != nil {
			return err
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
	return batch.ingestRecord(ctx, record)
}

// readStored reads the files the scan's root's library holds on the node.
func (listing *listingComparison) readStored(ctx context.Context) error {
	scan, batch := listing.batch.scanInProgress(), listing.batch
	if scan == nil {
		return nil
	}
	rows, err := batch.queries.ListFilesToCompare(ctx, sqlcgen.ListFilesToCompareParams{
		NodeID: batch.nodeID, LibraryID: scan.libraryID,
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
	stored, isStored := listing.stored[string(located.reportedBytes)]
	if isStored && isStoredAsListed(stored, kind, listed) {
		err := listing.batch.queries.SetFileSeen(ctx, sqlcgen.SetFileSeenParams{
			SeenAt: listing.batch.nowMs, ID: stored.ID,
		})
		if err != nil {
			return fmt.Errorf("mark %s seen: %w", located.text, err)
		}
		return nil
	}
	listing.taken.ToExamine = append(listing.taken.ToExamine, nodeproto.ExamineFile{Path: listed.Path, Kind: kind})
	return nil
}

// directoryRule is the rule a directory, relative to the root, is skipped by: the root's .trash,
// where an export keeps the files it replaced (D144), or a release's sample/ (D162).
func directoryRule(relative string) (nodeproto.SkipRule, bool) {
	switch {
	case relative == ".trash":
		return nodeproto.SkipTrash, true
	case strings.EqualFold(path.Base(relative), "sample"):
		return nodeproto.SkipSample, true
	}
	return "", false
}

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
	case !rules.VideoExtensions[extension]:
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
