// SPDX-License-Identifier: AGPL-3.0-or-later

package videofile

import (
	"maps"
	"path"
	"strings"
)

// The extensions a scan reads are the code's, not settings (D360, amending D240 §8): which
// containers FFmpeg reads is no admin's judgement, and a stored list kept a container added later
// from reaching an install that had one. An extension is lower case and without the dot.

// extensions are the extensions of the files a scan reads as videos: the containers people keep
// films in, Matroska and MP4 first, the broadcast and disc formats, and the older ones still found
// in libraries, RealMedia among them (the owner, 2026-10-27). A file with another extension is never
// probed, so a missing one hides films; one also used by files that are no video costs a probe of
// each. Those D360 added are each proven listed, probed, decided and their first chunk made by the
// integration tier's TestPlayback_EachContainerAddedIsReadProbedDecidedAndItsFirstChunkMade.
//
//nolint:gochecknoglobals // a constant set, read for every file a count of a folder's videos meets
var extensions = setOf(
	"mkv", "mp4", "m4v", "avi", "divx", "mov", "wmv", "ts", "m2ts", "mts", "webm", "mpg", "mpeg", "vob",
	"flv", "ogv", "3gp",
	"rm", "rmvb", // RealMedia
	"asf", "dvr-ms", "wtv", // Windows Media and Media Center's recordings
	"ogm", "mxf", "dv", "nut", "ivf", "y4m", "xvid",
	"f4v", "3g2", "qt", "mk3d", // MP4's and Matroska's relatives
	"mpe", "m2p", "vro", "evo", // MPEG's program streams, DVD-VR's and HD DVD's
	"trp", "tp", "m2t", "tod", // transport streams: broadcast recordings and camcorders
)

// DiscImageExtension is a DVD's or a Blu-ray's image, read as a video whatever the video extensions
// say: its node serves its main title (D350).
const DiscImageExtension = "iso"

// Extensions are the extensions of the files a scan reads as videos, a disc's image aside: a copy,
// the caller's to change.
func Extensions() map[string]bool { return maps.Clone(extensions) }

// ExtensionOf is a file name's extension as the extensions name one: lower case, without the dot.
func ExtensionOf(name string) string {
	return strings.ToLower(strings.TrimPrefix(path.Ext(name), "."))
}

// IsVideo is whether a scan reads the file named name as a video: its extension a video one or a
// disc's image, and it neither an AppleDouble file nor a sample.
func IsVideo(name string) bool {
	extension := ExtensionOf(name)
	return !IsAppleDouble(name) && (extensions[extension] || extension == DiscImageExtension) && !IsSample(name)
}

// IsAppleDouble is a macOS resource file beside the one it describes: "._" and its name.
func IsAppleDouble(name string) bool { return strings.HasPrefix(name, "._") }

// IsSample is sample.mkv, x.sample.mkv or x-sample.mkv, in any case (D162).
func IsSample(name string) bool {
	stem := strings.ToLower(strings.TrimSuffix(name, path.Ext(name)))
	return stem == "sample" || strings.HasSuffix(stem, ".sample") || strings.HasSuffix(stem, "-sample")
}

// IsSkippedFolder is whether a scan skips the folder at relative, its path below the scan's root:
// the root's .trash, a sample folder, a system folder or a disc's folder.
func IsSkippedFolder(relative string) bool {
	name := path.Base(relative)
	return IsTrash(relative) || IsSampleFolder(name) || IsSystemFolder(name) || IsDiscFolder(name)
}

// IsTrash is the root's .trash, where an export keeps the files it replaced (D144), by its path below
// the root.
func IsTrash(relative string) bool { return relative == ".trash" }

// IsSampleFolder is a release's sample/, by its name, in any case (D162).
func IsSampleFolder(name string) bool { return strings.EqualFold(name, "sample") }

// IsDiscFolder is a disc's folder, VIDEO_TS or BDMV by its name in any case, which a scan skips as it
// does not skip the disc's image (D306 §5).
func IsDiscFolder(name string) bool {
	return strings.EqualFold(name, "VIDEO_TS") || strings.EqualFold(name, "BDMV")
}

// IsSystemFolder is a folder, by its name, that a NAS or an operating system keeps for itself at
// any depth: Synology's, QNAP's, ZFS's and NetApp's snapshots, Windows' and macOS's bins and
// indexes, and a filesystem's lost+found. Jellyfin, Kodi and Plex's scanners skip the same.
func IsSystemFolder(name string) bool {
	switch strings.ToLower(name) {
	case "@eadir", "#recycle", "@recycle", "#snapshot", "@recently-snapshot", ".@__thumb", ".snapshot", ".snapshots",
		".zfs", "$recycle.bin", "system volume information", "lost+found", ".trashes", ".spotlight-v100", ".fseventsd",
		".appledouble", ".temporaryitems", ".documentrevisions-v100":
		return true
	}
	return false
}

func setOf(names ...string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, name := range names {
		set[name] = true
	}
	return set
}
