// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/nodeproto"
	"github.com/kstenerud/orpheum/internal/platform/ptr"
)

// isSeasonFolder is whether a folder is a series' season folder, named as the filename parser
// reads one, or its specials: an image in one is a season's, which nothing shows yet.
func isSeasonFolder(name string) bool {
	return seasonFolderPattern.MatchString(name) || strings.EqualFold(name, "specials")
}

// folderImageNames are the names an image has in a film's or a series' folder, most specific
// first within each kind (D215 §2, Kodi's and Jellyfin's names).
func folderImageNames() []imageName {
	return []imageName{
		{kind: library.ArtPoster, name: "poster"},
		{kind: library.ArtPoster, name: "folder"},
		{kind: library.ArtPoster, name: "cover"},
		{kind: library.ArtBackdrop, name: "fanart"},
		{kind: library.ArtBackdrop, name: "backdrop"},
		{kind: library.ArtBackdrop, name: "background"},
		{kind: library.ArtLogo, name: "clearlogo"},
		{kind: library.ArtLogo, name: "logo"},
	}
}

// videoImageSuffixes are the suffixes an image beside one video has: "<name>-poster.jpg".
func videoImageSuffixes() []imageName {
	return []imageName{
		{kind: library.ArtPoster, name: "-poster"},
		{kind: library.ArtBackdrop, name: "-fanart"},
		{kind: library.ArtBackdrop, name: "-backdrop"},
		{kind: library.ArtLogo, name: "-clearlogo"},
		{kind: library.ArtLogo, name: "-logo"},
		{kind: library.ArtStill, name: "-thumb"},
	}
}

// imageName is a name, or a name's suffix, that says an image's kind.
type imageName struct {
	kind string
	name string
}

// localChoice is one image found for an item's kind, and how specific its name is: the lower,
// the more.
type localChoice struct {
	image LocalImage
	rank  int
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
		choice.image.ItemID, choice.image.FileID = itemID, file.ID
		choice.image.SizeBytes = ptr.ValueOrZero(file.SizeBytes)
		choice.image.ModifiedAtMs = ptr.ValueOrZero(file.ModifiedAt)
		if chosen[itemID] == nil {
			chosen[itemID] = map[string]localChoice{}
		}
		if held, taken := chosen[itemID][choice.image.Kind]; !taken || choice.rank < held.rank {
			chosen[itemID][choice.image.Kind] = choice
		}
	}
	for itemID, kinds := range chosen {
		for _, kind := range slices.Sorted(maps.Keys(kinds)) {
			filer.localArt[itemID] = append(filer.localArt[itemID], kinds[kind].image)
		}
	}
}

// reportLocalImages reports every image the pass found beside the library's media, by item: the
// artwork service fetches what it does not hold, and retires an image no longer found (D215 §1).
func (filer *libraryFiler) reportLocalImages() {
	found := &LocalImages{LibraryID: filer.library.ID}
	for _, itemID := range slices.Sorted(maps.Keys(filer.localArt)) {
		found.Images = append(found.Images, filer.localArt[itemID]...)
	}
	filer.report.BesideTheMedia = found
}

// localImageOwner is the item an image beside the media belongs to, and its kind, by its name.
func (filer *libraryFiler) localImageOwner(
	imagePath string, videoFileIDByStem map[string]int64) (int64, localChoice, bool,
) {
	folder := path.Dir(imagePath)
	stem := strings.TrimSuffix(path.Base(imagePath), path.Ext(imagePath))
	for rank, named := range videoImageSuffixes() {
		rest, cut := strings.CutSuffix(strings.ToLower(stem), named.name)
		if !cut {
			continue
		}
		titleID, placed := filer.titleIDByFile[videoFileIDByStem[strings.ToLower(folder)+"/"+rest]]
		if !placed || !showsArtKind(filer.titleKind(titleID), named.kind) {
			return 0, localChoice{}, false
		}
		return titleID, localChoice{image: LocalImage{Kind: named.kind}, rank: rank}, true
	}
	for rank, named := range folderImageNames() {
		if !strings.EqualFold(stem, named.name) {
			continue
		}
		rank += len(videoImageSuffixes()) // a name for one video is more specific than one for the folder
		if titleID, isFilm := filer.titleIDByFolder[folder]; isFilm {
			return titleID, localChoice{image: LocalImage{Kind: named.kind}, rank: rank}, true
		}
		if seriesID, isSeries := filer.seriesIDByFolder[folder]; isSeries {
			return seriesID, localChoice{image: LocalImage{Kind: named.kind}, rank: rank}, true
		}
	}
	return 0, localChoice{}, false
}

// showsArtKind is whether a title of a kind shows an image of an artwork kind: an episode its still, a
// film its poster, backdrop and logo.
func showsArtKind(titleKind, artKind string) bool {
	if titleKind == library.TitleKindEpisode {
		return artKind == library.ArtStill
	}
	return titleKind == KindMovie && artKind != library.ArtStill
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
