// SPDX-License-Identifier: GPL-3.0-or-later

package nodeproto

import (
	"encoding/base64"
	"net/url"
)

// PathBytesFile is the node byte socket's route for a file's bytes (cut-a §4).
const PathBytesFile = "/bytes/v1/file"

// BytesFile names a file on the byte socket: Root, one of the node's roots as its hello named
// it; Path, the file's path relative to that root, as bytes, so a name that is not UTF-8 travels
// intact (D49); and Hash, the payload hash in hex the core knows the file by (D179, D100).
type BytesFile struct {
	Root        string `json:"root"`
	Path        []byte `json:"path"`
	PayloadHash string `json:"payloadHash"`
}

// Target is the file's path and query on the byte socket; Path travels as unpadded base64url.
func (file BytesFile) Target() string {
	query := url.Values{
		"root": {file.Root}, "path": {base64.RawURLEncoding.EncodeToString(file.Path)},
		"payloadHash": {file.PayloadHash},
	}
	return PathBytesFile + "?" + query.Encode()
}
