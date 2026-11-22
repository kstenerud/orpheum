// SPDX-License-Identifier: AGPL-3.0-or-later

package nodeproto

import "net/url"

// PathBytesFolders is the node byte socket's route for the folders in a folder: one of its roots, or
// a folder inside one, for an admin choosing a library folder (D359 §2). It is answered outside every
// queue, without a slot or a reader, as a probe is.
const PathBytesFolders = "/bytes/v1/folders"

// queryPath is the byte socket's query parameter naming a path: a file's, a place's or a folder's.
const queryPath = "path"

// FoldersRequest names the folder whose folders are asked for: a clean absolute path, one of the
// node's roots as its hello named it or a folder inside one.
type FoldersRequest struct {
	Path string
}

// Target is the listing's path and query on the byte socket.
func (request FoldersRequest) Target() string {
	return PathBytesFolders + "?" + url.Values{queryPath: {request.Path}}.Encode()
}

// FolderListing is a node's answer to a FoldersRequest. State is a probe's (ProbeState): readable with
// the folders, notAnswering when its disk did not answer in the node's window or its root holds a
// reading that has not returned, missing when the folder is not there, failed otherwise, a link or
// a file among them, since a library folder is named by its real path.
type FolderListing struct {
	Path  string     `json:"path"`
	State ProbeState `json:"state"`
	// Folders are the names of the folders in it, in byte order: no link, even to a folder, and no
	// hidden folder, whose name starts with "." (the owner, 2026-10-27).
	Folders []string `json:"folders,omitempty"`
	// VideoCounts are how many videos each of the folders holds, by its name, in it and in every
	// folder below it, as a scan of it would read them (D539 §7): counted within a budget for the
	// whole listing, so a folder not counted in time is not here, never a part of its count.
	VideoCounts map[string]int64 `json:"videoCounts,omitempty"`
	// Failure says why it is not readable, for any state but readable.
	Failure string `json:"failure,omitempty"`
}
