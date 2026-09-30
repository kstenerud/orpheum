// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// SourceLocal is an image beside the media, read and re-encoded by the node (D215).
const SourceLocal = "local"

// isSeasonFolder is whether a folder is a series' season folder, named as the filename parser
// reads one, or its specials: an image in one is a season's, which nothing shows yet.
func isSeasonFolder(name string) bool {
	return seasonFolderPattern.MatchString(name) || strings.EqualFold(name, "specials")
}

// folderImageNames are the names an image has in a film's or a series' folder, most specific
// first within each kind (D215 §2, Kodi's and Jellyfin's names).
func folderImageNames() []ArtRef {
	return []ArtRef{
		{Kind: ArtPoster, Ref: "poster"},
		{Kind: ArtPoster, Ref: "folder"},
		{Kind: ArtPoster, Ref: "cover"},
		{Kind: ArtBackdrop, Ref: "fanart"},
		{Kind: ArtBackdrop, Ref: "backdrop"},
		{Kind: ArtBackdrop, Ref: "background"},
		{Kind: ArtLogo, Ref: "clearlogo"},
		{Kind: ArtLogo, Ref: "logo"},
	}
}

// videoImageSuffixes are the suffixes an image beside one video has: "<name>-poster.jpg".
func videoImageSuffixes() []ArtRef {
	return []ArtRef{
		{Kind: ArtPoster, Ref: "-poster"},
		{Kind: ArtBackdrop, Ref: "-fanart"},
		{Kind: ArtBackdrop, Ref: "-backdrop"},
		{Kind: ArtLogo, Ref: "-clearlogo"},
		{Kind: ArtLogo, Ref: "-logo"},
		{Kind: ArtStill, Ref: "-thumb"},
	}
}

// LocalRef is how a make_artwork_sizes job names an image beside the media: its file, and its size and
// modification time, so a changed file is owed again.
func LocalRef(fileID, sizeBytes, mtimeMs int64) string {
	return fmt.Sprintf("file:%d:%d:%d", fileID, sizeBytes, mtimeMs)
}

// LocalRefFileID is the id of the file a LocalRef names.
func LocalRefFileID(ref string) (int64, bool) {
	var fileID, sizeBytes, mtimeMs int64
	if _, err := fmt.Sscanf(ref, "file:%d:%d:%d", &fileID, &sizeBytes, &mtimeMs); err != nil {
		return 0, false
	}
	return fileID, true
}

// localChoice is one image found for an item's kind, and how specific its name is: the lower,
// the more.
type localChoice struct {
	ref  ArtRef
	rank int
}

// mapLocalArtwork finds, for each film, series and episode, the images beside it by their
// names. Every library takes them at once; which shows is the library's preference (D215 §1).
// Run after every video is placed, before the titles are resolved.
func (filer *libraryFiler) mapLocalArtwork() {
	videoFileIDByStem := map[string]int64{} // a video's folder and stem, folded, to its file
	for _, file := range filer.files {
		if isVideo(file) {
			videoFileIDByStem[strings.ToLower(strings.TrimSuffix(file.Path, path.Ext(file.Path)))] = file.ID
		}
	}
	chosen := map[int64]map[string]localChoice{}
	for _, file := range filer.files {
		isImage := ptr.ValueOrZero(file.SidecarKind) == string(nodeproto.SidecarImage)
		if file.Filing != library.FilingAttached || !isImage {
			continue
		}
		itemID, choice, found := filer.localImageOwner(file.Path, videoFileIDByStem)
		if !found {
			continue
		}
		choice.ref.Ref = LocalRef(file.ID, ptr.ValueOrZero(file.SizeBytes), ptr.ValueOrZero(file.ModifiedAt))
		if chosen[itemID] == nil {
			chosen[itemID] = map[string]localChoice{}
		}
		if held, taken := chosen[itemID][choice.ref.Kind]; !taken || choice.rank < held.rank {
			chosen[itemID][choice.ref.Kind] = choice
		}
	}
	for itemID, kinds := range chosen {
		for _, choice := range kinds {
			filer.localArt[itemID] = append(filer.localArt[itemID], choice.ref)
		}
	}
}

// retireLocalArtwork removes the images beside the media that this pass no longer found beside
// their item: a deleted poster.jpg, or a video renamed away from its image. Without it, a removed
// image kept showing, ranked first (D215 §1).
func (filer *libraryFiler) retireLocalArtwork(ctx context.Context) error {
	items, err := filer.queries.ListItemsWithLiveUnchosenLocalArtwork(ctx, &filer.library.ID)
	if err != nil {
		return fmt.Errorf("list the library's local artwork: %w", err)
	}
	for _, itemID := range items {
		found := make([]string, 0, len(filer.localArt[itemID]))
		for _, ref := range filer.localArt[itemID] {
			found = append(found, ref.Ref)
		}
		encoded, err := json.Marshal(found)
		if err != nil {
			return fmt.Errorf("encode item %d's local artwork: %w", itemID, err)
		}
		err = filer.queries.RetireArtworkNotFound(ctx, sqlcgen.RetireArtworkNotFoundParams{
			RemovedAt: &filer.nowMs, ItemID: itemID, Source: SourceLocal, FoundRefs: string(encoded),
		})
		if err != nil {
			return fmt.Errorf("retire item %d's local artwork no longer found: %w", itemID, err)
		}
	}
	return nil
}

// localImageOwner is the item an image beside the media belongs to, and its kind, by its name.
func (filer *libraryFiler) localImageOwner(
	imagePath string, videoFileIDByStem map[string]int64) (int64, localChoice, bool,
) {
	folder := path.Dir(imagePath)
	stem := strings.TrimSuffix(path.Base(imagePath), path.Ext(imagePath))
	for rank, named := range videoImageSuffixes() {
		rest, cut := strings.CutSuffix(strings.ToLower(stem), named.Ref)
		if !cut {
			continue
		}
		titleID, placed := filer.titleIDByFile[videoFileIDByStem[strings.ToLower(folder)+"/"+rest]]
		if !placed || !showsArtKind(filer.titleKind(titleID), named.Kind) {
			return 0, localChoice{}, false
		}
		return titleID, localChoice{ref: ArtRef{Kind: named.Kind}, rank: rank}, true
	}
	for rank, named := range folderImageNames() {
		if !strings.EqualFold(stem, named.Ref) {
			continue
		}
		rank += len(videoImageSuffixes()) // a name for one video is more specific than one for the folder
		if titleID, isFilm := filer.titleIDByFolder[folder]; isFilm {
			return titleID, localChoice{ref: ArtRef{Kind: named.Kind}, rank: rank}, true
		}
		if seriesID, isSeries := filer.seriesIDByFolder[folder]; isSeries {
			return seriesID, localChoice{ref: ArtRef{Kind: named.Kind}, rank: rank}, true
		}
	}
	return 0, localChoice{}, false
}

// showsArtKind is whether a title of a kind shows an image of an artwork kind: an episode its still, a
// film its poster, backdrop and logo.
func showsArtKind(titleKind, artKind string) bool {
	if titleKind == library.TitleKindEpisode {
		return artKind == ArtStill
	}
	return titleKind == KindMovie && artKind != ArtStill
}

// titleKind is a placed title's kind, from its evidence.
func (filer *libraryFiler) titleKind(titleID int64) string {
	if specs := filer.evidence[titleID]; len(specs) > 0 {
		return specs[0].kind
	}
	return ""
}

// seriesFolderOf is the folder a series' images sit in, from one of its episodes: above a season
// folder, else the episode's own.
func seriesFolderOf(episodePath string) string {
	folder := path.Dir(episodePath)
	if isSeasonFolder(path.Base(folder)) {
		return path.Dir(folder)
	}
	return folder
}

// withoutLocalKinds drops the provider's images of the kinds an item has beside it, where the
// library prefers images beside the media: those are not fetched. A TMDB-only library fetches
// them, to replace what is beside the media (D215 §1).
func (filer *libraryFiler) withoutLocalKinds(itemID int64, refs []ArtRef) []ArtRef {
	local := filer.localArt[itemID]
	if len(local) == 0 || filer.library.MetadataSource == sourceTMDB {
		return refs
	}
	var kept []ArtRef
	for _, ref := range refs {
		covered := false
		for _, have := range local {
			covered = covered || have.Kind == ref.Kind
		}
		if !covered {
			kept = append(kept, ref)
		}
	}
	return kept
}
