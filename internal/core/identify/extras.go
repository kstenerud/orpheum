// SPDX-License-Identifier: GPL-3.0-or-later

package identify

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/kstenerud/orpheum/internal/store/sqlcgen"
)

// extraKind is a file found inside a film's own folder in an extras subfolder: containment makes
// it an extra, and the folder types it (D162).
type extraKind struct {
	ownerFolder string // relative to the root
	kind        string
}

// pendingExtra is an extra waiting for its film to be filed.
type pendingExtra struct {
	file  sqlcgen.ListIdentifyFilesRow
	rel   string
	extra extraKind
}

// extraFolderKind maps D162's closed vocabulary of extras folders to extra types.
func extraFolderKind(name string) (string, bool) {
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

// suffixKind maps an extra's file-name suffix to its type.
func suffixKind(suffix string) string {
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

// extraOf reports whether a path is an extra by containment: in a vocabulary folder inside a
// title's own folder, never at a library root (D162). A "Film-trailer.mkv" beside its film is
// the other admissible evidence, settled once the folder's film is known (extraBySuffix).
func extraOf(rel string) (extraKind, bool) {
	folders := strings.Split(path.Dir(rel), "/")
	if path.Dir(rel) == "." || len(folders) < 2 {
		return extraKind{}, false
	}
	kind, isExtra := extraFolderKind(folders[len(folders)-1])
	if !isExtra {
		return extraKind{}, false
	}
	parsed := ParsePath(rel)
	if parsed.ExtraSuffix != "" {
		kind = suffixKind(parsed.ExtraSuffix) // inside an extras folder, a suffix picks the type
	}
	return extraKind{ownerFolder: strings.Join(folders[:len(folders)-1], "/"), kind: kind}, true
}

// extra files an extra under the film in its owner folder. With no film there, containment does
// not hold, and it is filed as a film itself: an extra shown as a film is clutter, a film taken
// for an extra vanishes (D162).
func (work *run) extra(ctx context.Context, pending pendingExtra) error {
	root := strings.TrimSuffix(pending.file.Path, pending.rel)
	ownerTitle, found := work.folderTitle[path.Clean(root+pending.extra.ownerFolder)]
	if !found {
		return work.work(ctx, pending.file, pending.rel)
	}
	parsed := ParsePath(pending.rel)
	spec := titleSpec{
		kind: kindExtra, name: parsed.Name, runtimeMs: probeRuntime(pending.file.Probe), ids: map[string]string{},
		key:    fmt.Sprintf("extra|%d|%s", ownerTitle, strings.ToLower(pending.rel)),
		source: sourceNone, ident: identIdentified,
	}
	titleID, kept, err := work.currentTitle(ctx, pending.file)
	if err == nil && !kept {
		titleID, err = work.titleFor(ctx, spec)
	}
	if err != nil {
		return err
	}
	err = work.queries.UpsertScannerExtraRelation(ctx, sqlcgen.UpsertScannerExtraRelationParams{
		OwnerTitle: ownerTitle, ExtraTitle: titleID, Type: pending.extra.kind,
	})
	if err != nil {
		return fmt.Errorf("relate the extra %s: %w", pending.file.Path, err)
	}
	return work.place(ctx, pending.file, titleID, spec)
}
