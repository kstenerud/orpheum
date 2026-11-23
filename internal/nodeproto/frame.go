// SPDX-License-Identifier: AGPL-3.0-or-later

package nodeproto

import (
	"encoding/base64"
	"net/url"
	"strconv"
)

// PathBytesFrame is the node byte socket's route for a key frame of a video, the picture a cast's
// remote shows (D545): made at once, outside every queue, as a file's bytes are served, one at a
// time. It answers image/webp; 404 when the video has no picture at or before the place.
const PathBytesFrame = "/bytes/v1/frame"

// The frame route's query parameters beside a file's own: the picture stream, and the place in
// milliseconds from the video's start.
const (
	QueryFrameStreamIndex = "streamIndex"
	QueryFrameAtMs        = "atMs"
)

// FrameRequest asks for the keyframe at or before AtMs of Video's picture stream at StreamIndex.
type FrameRequest struct {
	Video       BytesFile
	StreamIndex int
	AtMs        int64
}

// Target is the frame's path and query on the byte socket; the video is named as BytesFile.Target
// names it.
func (request FrameRequest) Target() string {
	query := url.Values{
		"rootPath": {request.Video.RootPath}, queryPath: {base64.RawURLEncoding.EncodeToString(request.Video.Path)},
		"payloadHash":         {request.Video.PayloadHash},
		QueryFrameStreamIndex: {strconv.Itoa(request.StreamIndex)},
		QueryFrameAtMs:        {strconv.FormatInt(request.AtMs, 10)},
	}
	return PathBytesFrame + "?" + query.Encode()
}
