// SPDX-License-Identifier: AGPL-3.0-or-later

package nodeproto

// PathChanged is where the node posts a Changed: what its watcher saw change under a root (D27,
// D272 §9). Best effort: one lost is caught by the root's periodic scan, which the watcher never
// replaces.
const PathChanged = "/node/v1/changed"

// Changed is the node's word that something changed under Path, a folder relative to RootPath, one
// of the library folders it watches (HelloReply.WatchFolders): the folder holding every change its
// watcher saw there before the folder went quiet, "." for the whole of it, as after the watcher missed
// events. The core decides what to scan.
type Changed struct {
	RootPath string `json:"rootPath"`
	Path     string `json:"path"`
}

// WatchCause is why the node does not watch a root, in a closed set the web client words.
type WatchCause string

// The causes a root is not watched.
const (
	// WatchCauseNetworkFilesystem is a root on NFS, SMB or 9P (a WSL drive), whose changes made elsewhere the kernel
	// never hears of (D27).
	WatchCauseNetworkFilesystem WatchCause = "network-filesystem"
	// WatchCauseWatchLimit is a root with more folders than the kernel lets a user watch
	// (fs.inotify.max_user_watches).
	WatchCauseWatchLimit WatchCause = "watch-limit"
	// WatchCauseUnavailable is a node whose kernel would not give it a watcher at all, or a folder
	// its watcher has not started watching yet.
	WatchCauseUnavailable WatchCause = "unavailable"
	// WatchCauseUnreadable is a root, or a folder in it, the watcher could not read.
	WatchCauseUnreadable WatchCause = "unreadable"
)

// RootWatch is whether the node watches a library folder it was given (HelloReply.WatchFolders), RootPath, for
// changes, and when it does not, why: one per folder, as the status says them.
type RootWatch struct {
	RootPath   string     `json:"rootPath"`
	IsWatching bool       `json:"isWatching"`
	Cause      WatchCause `json:"cause,omitempty"`
	// Reason is the cause in the node's own words, for the log and the admin.
	Reason string `json:"reason,omitempty"`
}
