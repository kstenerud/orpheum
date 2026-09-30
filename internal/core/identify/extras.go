// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/kstenerud/orpheum/internal/core/library"
	"github.com/kstenerud/orpheum/internal/core/textindex"
	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// foundExtra is a file found to be a film's extra by containment (D162): in an extras subfolder
// of the film's own folder, which types it, or named for the film with an extra's suffix.
type foundExtra struct {
	ownerFolder string // relative to the root
	extraType   string
	evidence    string // what made it one, for its filing's reason (D163)
}

// pendingExtra is an extra waiting for its film to be filed.
type pendingExtra struct {
	file         sqlcgen.ListIdentifyFilesRow
	relativePath string
	extra        foundExtra
}

// extraTypeOfFolder maps D162's closed vocabulary of extras folders to extra types.
func extraTypeOfFolder(name string) (string, bool) {
	switch strings.ToLower(name) {
	case "trailers":
		return extraTrailer, true
	case "featurettes":
		return extraFeaturette, true
	case "deleted scenes":
		return "deleted_scene", true
	case "behind the scenes":
		return "behind_the_scenes", true
	case "interviews":
		return extraInterview, true
	case "scenes":
		return extraScene, true
	case "shorts":
		return extraShort, true
	case "clips":
		return extraClip, true
	case "backdrops":
		return "theme_video", true
	case "theme-music":
		return "theme_song", true
	case "extras", "extra", "other":
		return extraUnknown, true
	}
	return "", false
}

// extraTypeOfSuffix maps an extra's file-name suffix to its type.
func extraTypeOfSuffix(suffix string) string {
	switch suffix {
	case extraTrailer, "teaser", extraClip, extraFeaturette, extraInterview, extraShort, extraScene:
		return suffix
	case "behindthescenes":
		return "behind_the_scenes"
	case "deleted", "deletedscene":
		return "deleted_scene"
	}
	return extraUnknown
}

// detectExtra reports whether a path is an extra by containment, never at a library root (D162):
// in a vocabulary folder inside a title's own folder, or named for the title whose folder it is
// in, with a vocabulary suffix ("Film (1999)/Film (1999)-trailer.mkv"). Either is settled once
// the folder's film is known; with none there it is a film itself.
func detectExtra(relativePath string) (foundExtra, bool) {
	folders := strings.Split(path.Dir(relativePath), "/")
	if path.Dir(relativePath) == "." {
		return foundExtra{}, false
	}
	if bySuffix, isExtra := extraBySuffix(relativePath); isExtra {
		return bySuffix, true
	}
	if len(folders) < 2 {
		return foundExtra{}, false
	}
	kind, isExtra := extraTypeOfFolder(folders[len(folders)-1])
	if !isExtra {
		return foundExtra{}, false
	}
	parsed := ParsePath(relativePath)
	if parsed.ExtraSuffix != "" {
		kind = extraTypeOfSuffix(parsed.ExtraSuffix) // inside an extras folder, a suffix picks the type
	}
	return foundExtra{
		ownerFolder: strings.Join(folders[:len(folders)-1], "/"), extraType: kind,
		evidence: "its " + folders[len(folders)-1] + " folder",
	}, true
}

// extraBySuffix is D162's second evidence: a file whose stem is its folder's title plus a
// vocabulary suffix. The suffix alone never makes an extra; the folder's name must be the rest.
func extraBySuffix(relativePath string) (foundExtra, bool) {
	folder := path.Dir(relativePath)
	stem := strings.TrimSuffix(path.Base(relativePath), path.Ext(relativePath))
	rest, suffix := cutExtraSuffix(stem)
	if suffix == "" || textindex.SameWork(rest) != textindex.SameWork(path.Base(folder)) {
		return foundExtra{}, false
	}
	return foundExtra{ownerFolder: folder, extraType: extraTypeOfSuffix(suffix), evidence: "its name"}, true
}

// fileExtra files an extra under the film in its owner folder. With no film there, containment does
// not hold, and it is filed as a film itself: an extra shown as a film is clutter, a film taken
// for an extra vanishes (D162).
func (filer *libraryFiler) fileExtra(ctx context.Context, pending pendingExtra) error {
	root := strings.TrimSuffix(pending.file.Path, pending.relativePath)
	ownerTitleID, found := filer.titleIDByFolder[path.Clean(root+pending.extra.ownerFolder)]
	if !found {
		return filer.fileFilmOrEpisode(ctx, pending.file, pending.relativePath)
	}
	parsed := ParsePath(pending.relativePath)
	spec := titleSpec{
		kind: library.TitleKindExtra, name: parsed.Name, runtimeMs: probeRuntimeMs(pending.file.ProbeJSON),
		ids:    map[string]string{},
		key:    fmt.Sprintf("extra|%d|%s", ownerTitleID, strings.ToLower(pending.relativePath)),
		source: sourceNone, identification: identificationIdentified,
	}
	titleID, hasCurrentTitle, err := filer.currentTitle(ctx, pending.file)
	if err == nil && !hasCurrentTitle {
		titleID, err = filer.findOrAddTitle(ctx, spec)
	}
	if err != nil {
		return err
	}
	err = filer.queries.UpsertScannerExtraRelation(ctx, sqlcgen.UpsertScannerExtraRelationParams{
		OwnerTitle: ownerTitleID, ExtraTitle: titleID, ExtraType: pending.extra.extraType,
	})
	if err != nil {
		return fmt.Errorf("relate the extra %s: %w", pending.file.Path, err)
	}
	filer.extraReasonByFile[pending.file.ID] = fmt.Sprintf("%s of %s, by %s",
		strings.ReplaceAll(pending.extra.extraType, "_", " "), pending.extra.ownerFolder, pending.extra.evidence)
	return filer.place(ctx, pending.file, titleID, spec)
}
