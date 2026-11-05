// SPDX-License-Identifier: AGPL-3.0-or-later

package identify

import (
	"context"
	"maps"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// filingUnit is a set of a library's files whose filing depends on each other and on nothing outside
// the set but stored rows the pass looks up (D304 §2): every file of a title or a series, with the
// files beside them. A pass files a unit whole in one write, so no title is resolved from half its
// files, and no one sees a film with half its versions.
type filingUnit struct {
	files []sqlcgen.ListIdentifyFilesRow
}

// filingUnits cuts the library's files into units, in the order of their first file's path. Two
// videos share a unit when they share any of: the folder under a root they are in (a film's folder
// with its versions and extras, a series' folders); the title they are in now; the work their path
// and NFO name, a film's name and year; a provider's id; or, for an episode, its series' name as
// textindex.SameWork folds it. A file beside the media joins the unit of its folder, and at a root
// the unit of the video its name begins with; one beside no video belongs to none, since nothing
// files it.
func (filer *libraryFiler) filingUnits(ctx context.Context) ([]filingUnit, error) {
	units := newUnionFind(len(filer.files))
	firstByKey := map[string]int{}
	join := func(index int, key string) {
		if first, seen := firstByKey[key]; seen {
			units.union(first, index)
		} else {
			firstByKey[key] = index
		}
	}
	videoAtRootByStem := map[string]int{}
	for index, file := range filer.files {
		relativePath, ok := filer.relativePath(file)
		if !ok {
			continue
		}
		if top, _, inFolder := strings.Cut(relativePath, "/"); inFolder {
			join(index, "top:"+strconv.FormatInt(file.NodeID, 10)+":"+top)
		}
		if !isVideo(file) {
			continue
		}
		if !strings.Contains(relativePath, "/") {
			videoAtRootByStem[strings.ToLower(strings.TrimSuffix(file.Path, path.Ext(file.Path)))] = index
		}
		keys, err := filer.unitKeys(ctx, file, relativePath)
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			join(index, key)
		}
	}
	return units.groups(filer.files, filer.unitMembers(units, videoAtRootByStem)), nil
}

// unitMembers says which files belong to a unit: every video and every file in a folder, and a file
// at a root beside a video it is named for, which joins that video's unit.
func (filer *libraryFiler) unitMembers(units *unionFind, videoAtRootByStem map[string]int) []bool {
	member := make([]bool, len(filer.files))
	for index, file := range filer.files {
		relativePath, ok := filer.relativePath(file)
		switch {
		case !ok:
		case isVideo(file) || strings.Contains(relativePath, "/"):
			member[index] = true
		default:
			if video, found := videoNamedBefore(file.Path, videoAtRootByStem); found {
				units.union(video, index)
				member[index] = true
			}
		}
	}
	return member
}

// unitKeys are the keys a video shares a unit by, besides its folder.
func (filer *libraryFiler) unitKeys(
	ctx context.Context, file sqlcgen.ListIdentifyFilesRow, relativePath string,
) ([]string, error) {
	var keys []string
	var currentTitle int64
	if file.VersionID != nil {
		current, err := filer.versionTitle(ctx, *file.VersionID)
		if err != nil {
			return nil, err
		}
		currentTitle = current.WorkID
		keys = append(keys, "title:"+strconv.FormatInt(currentTitle, 10))
	}
	parsed := filer.parse(relativePath)
	if parsed.Kind == library.TitleKindEpisode {
		return filer.episodeUnitKeys(ctx, file, parsed, currentTitle, keys)
	}
	if _, isExtra := detectExtra(relativePath); isExtra {
		return keys, nil // its owner is in its folder
	}
	spec := filer.movieSpec(file, parsed)
	work := movieKey(spec.name, spec.year)
	keys = append(append(keys, "work:"+work), idKeys(spec.ids)...)
	// The title its name and year find, which may be named otherwise now than this file's evidence,
	// and the one a provider's id finds: a file filed into a title shares its unit.
	if titleID, found := filer.titleIDByKey[work]; found {
		keys = append(keys, "title:"+strconv.FormatInt(titleID, 10))
	}
	for provider, id := range spec.ids {
		if titleID, found := filer.heldTitleByProviderID(KindMovie, providerIDOf(provider, id)); found {
			keys = append(keys, "title:"+strconv.FormatInt(titleID, 10))
		}
	}
	return keys, nil
}

// episodeUnitKeys are the keys an episode shares a unit by: every name its series may be found by,
// its folders' and its tvshow.nfo's, the series those names find, and the series it is in now. A
// series is resolved from all its files' evidence, so one filed from part of them took the name
// a new folder spelled it by over its tvshow.nfo's.
func (filer *libraryFiler) episodeUnitKeys(
	ctx context.Context, file sqlcgen.ListIdentifyFilesRow, parsed ParsedPath, currentTitle int64, keys []string,
) ([]string, error) {
	names := []string{parsed.Name}
	if showNFO, found := filer.showNFOFor(file); found {
		keys = append(keys, idKeys(showNFO.ProviderIDs)...)
		names = append(names, showNFO.Title)
	}
	for _, name := range names {
		if folded := textindex.SameWork(name); folded != "" {
			keys = append(keys, "series:"+folded)
			if seriesID, known := filer.seriesIDByName[folded]; known {
				keys = append(keys, "seriesid:"+strconv.FormatInt(seriesID, 10))
			}
		}
	}
	if currentTitle != 0 {
		place, isEpisode, err := filer.episodePlace(ctx, currentTitle)
		if err != nil {
			return nil, err
		}
		if isEpisode {
			keys = append(keys, "seriesid:"+strconv.FormatInt(place.SequenceID, 10))
		}
	}
	return keys, nil
}

// idKeys are a work's provider ids as unit keys.
func idKeys(ids map[string]string) []string {
	keys := make([]string, 0, len(ids))
	for _, provider := range slices.Sorted(maps.Keys(ids)) {
		if ids[provider] != "" {
			keys = append(keys, "id:"+provider+":"+ids[provider])
		}
	}
	return keys
}

// videoNamedBefore is the video at a root whose name, before its extension, begins a file's own,
// the longest such: "Metropolis (1995).nfo", "Metropolis (1995).en.srt" and "Metropolis (1995)-poster.jpg" are
// Metropolis (1995).mkv's. The file's name is cut back at each separator in turn, so a root of thousands
// of films is not searched whole for each file beside them.
func videoNamedBefore(filePath string, videoByStem map[string]int) (int, bool) {
	stem := strings.ToLower(strings.TrimSuffix(filePath, path.Ext(filePath)))
	for {
		if index, found := videoByStem[stem]; found {
			return index, true
		}
		cut := strings.LastIndexAny(stem, ".-_ ")
		if cut <= strings.LastIndex(stem, "/") {
			return 0, false
		}
		stem = stem[:cut]
	}
}

// unionFind joins indexes into sets.
type unionFind struct {
	parent []int
}

func newUnionFind(size int) *unionFind {
	parent := make([]int, size)
	for index := range parent {
		parent[index] = index
	}
	return &unionFind{parent: parent}
}

func (sets *unionFind) find(index int) int {
	for sets.parent[index] != index {
		sets.parent[index] = sets.parent[sets.parent[index]]
		index = sets.parent[index]
	}
	return index
}

func (sets *unionFind) union(first, second int) {
	if rootFirst, rootSecond := sets.find(first), sets.find(second); rootFirst != rootSecond {
		sets.parent[max(rootFirst, rootSecond)] = min(rootFirst, rootSecond)
	}
}

// groups are the members' sets as units, each in the files' order, ordered by their first file.
func (sets *unionFind) groups(files []sqlcgen.ListIdentifyFilesRow, member []bool) []filingUnit {
	byRoot := map[int]int{}
	var units []filingUnit
	for index, file := range files {
		if !member[index] {
			continue
		}
		root := sets.find(index)
		position, seen := byRoot[root]
		if !seen {
			position = len(units)
			byRoot[root] = position
			units = append(units, filingUnit{})
		}
		units[position].files = append(units[position].files, file)
	}
	slices.SortStableFunc(units, func(first, second filingUnit) int {
		return strings.Compare(first.files[0].Path, second.files[0].Path)
	})
	return units
}
