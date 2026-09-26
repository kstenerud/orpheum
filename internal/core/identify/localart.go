// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// SourceLocal is an image beside the media, read and re-encoded by the node (D215).
const SourceLocal = "local"

// seasonFolder is a series' season folder: an image in one is a season's, which nothing shows yet.
var seasonFolder = regexp.MustCompile(`(?i)^(season|series|s)\s*\d+$|^specials$`)

// folderImageKinds are the names an image has in a film's or a series' folder, most specific
// first within each kind (D215 §2, Kodi's and Jellyfin's names).
func folderImageKinds() []ArtRef {
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

// videoImageKinds are the suffixes an image beside one video has: "<name>-poster.jpg".
func videoImageKinds() []ArtRef {
	return []ArtRef{
		{Kind: ArtPoster, Ref: "-poster"},
		{Kind: ArtBackdrop, Ref: "-fanart"},
		{Kind: ArtBackdrop, Ref: "-backdrop"},
		{Kind: ArtLogo, Ref: "-clearlogo"},
		{Kind: ArtLogo, Ref: "-logo"},
		{Kind: ArtStill, Ref: "-thumb"},
	}
}

// LocalRef is how an artwork job names an image beside the media: its file, and its size and
// modification time, so a changed file is owed again.
func LocalRef(fileID, size, mtimeMs int64) string {
	return fmt.Sprintf("file:%d:%d:%d", fileID, size, mtimeMs)
}

// LocalFile is the file a LocalRef names.
func LocalFile(ref string) (int64, bool) {
	var fileID, size, mtime int64
	if _, err := fmt.Sscanf(ref, "file:%d:%d:%d", &fileID, &size, &mtime); err != nil {
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
func (work *run) mapLocalArtwork() {
	videos := map[string]int64{} // a video's folder and stem, folded, to its file
	for _, file := range work.files {
		if isVideo(file) {
			videos[strings.ToLower(strings.TrimSuffix(file.Path, path.Ext(file.Path)))] = file.ID
		}
	}
	chosen := map[int64]map[string]localChoice{}
	for _, file := range work.files {
		if file.Filing != filingAttached || derefText(file.FilingReason) != string(nodeproto.SidecarImage) {
			continue
		}
		itemID, choice, found := work.localImage(file.Path, videos)
		if !found {
			continue
		}
		choice.ref.Ref = LocalRef(file.ID, deref(file.SizeBytes), deref(file.MtimeMs))
		if chosen[itemID] == nil {
			chosen[itemID] = map[string]localChoice{}
		}
		if held, taken := chosen[itemID][choice.ref.Kind]; !taken || choice.rank < held.rank {
			chosen[itemID][choice.ref.Kind] = choice
		}
	}
	for itemID, kinds := range chosen {
		for _, choice := range kinds {
			work.localArt[itemID] = append(work.localArt[itemID], choice.ref)
		}
	}
}

// localImage is the item an image beside the media belongs to, and its kind, by its name.
func (work *run) localImage(imagePath string, videos map[string]int64) (int64, localChoice, bool) {
	folder := path.Dir(imagePath)
	stem := strings.TrimSuffix(path.Base(imagePath), path.Ext(imagePath))
	for rank, named := range videoImageKinds() {
		rest, cut := strings.CutSuffix(strings.ToLower(stem), named.Ref)
		if !cut {
			continue
		}
		titleID, placed := work.fileTitle[videos[strings.ToLower(folder)+"/"+rest]]
		if !placed || !fits(work.titleKind(titleID), named.Kind) {
			return 0, localChoice{}, false
		}
		return titleID, localChoice{ref: ArtRef{Kind: named.Kind}, rank: rank}, true
	}
	for rank, named := range folderImageKinds() {
		if !strings.EqualFold(stem, named.Ref) {
			continue
		}
		rank += len(videoImageKinds()) // a name for one video is more specific than one for the folder
		if titleID, isFilm := work.folderTitle[folder]; isFilm {
			return titleID, localChoice{ref: ArtRef{Kind: named.Kind}, rank: rank}, true
		}
		if seriesID, isSeries := work.seriesFolders[folder]; isSeries {
			return seriesID, localChoice{ref: ArtRef{Kind: named.Kind}, rank: rank}, true
		}
	}
	return 0, localChoice{}, false
}

// fits is whether a title of a kind shows an image of an artwork kind: an episode its still, a
// film its poster, backdrop and logo.
func fits(titleKind, artKind string) bool {
	if titleKind == kindEpisode {
		return artKind == ArtStill
	}
	return titleKind == KindMovie && artKind != ArtStill
}

// titleKind is a placed title's kind, from its evidence.
func (work *run) titleKind(titleID int64) string {
	if specs := work.evidence[titleID]; len(specs) > 0 {
		return specs[0].kind
	}
	return ""
}

// seriesFolderOf is the folder a series' images sit in, from one of its episodes: above a season
// folder, else the episode's own.
func seriesFolderOf(episodePath string) string {
	folder := path.Dir(episodePath)
	if seasonFolder.MatchString(path.Base(folder)) {
		return path.Dir(folder)
	}
	return folder
}

// withoutLocalKinds drops the provider's images of the kinds an item has beside it, where the
// library prefers images beside the media: those are not fetched. A TMDB-only library fetches
// them, to replace what is beside the media (D215 §1).
func (work *run) withoutLocalKinds(itemID int64, refs []ArtRef) []ArtRef {
	local := work.localArt[itemID]
	if len(local) == 0 || work.library.MetadataSource == sourceTMDB {
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
