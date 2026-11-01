// SPDX-License-Identifier: AGPL-3.0-or-later

package rtcapi

import (
	"github.com/pion/webrtc/v4"
)

// videoCodec is the one video codec the core answers a client's video m-line with: VP8, which every
// browser offers. The core never sends on it. The client offers the m-line because Chrome grows a
// connection's socket receive buffer from 64 KiB only for a connection with video
// (kVideoRtpRecvBufferSize), and with the small one dropped the core's packets before its data
// channels read them (docs/contributors/idiosyncrasies.md; D397).
var videoCodec = webrtc.RTPCodecParameters{ //nolint:gochecknoglobals // a constant pion takes by value
	RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000},
	PayloadType:        96,
}

// newAPI is a pion API with engine's settings that answers a client's video m-line with VP8 alone,
// and sends nothing on it: the core adds no track. The default engine would answer the m-line too,
// naming every codec pion knows, for a line nothing is ever sent on.
func newAPI(engine webrtc.SettingEngine) *webrtc.API {
	media := &webrtc.MediaEngine{}
	// A fresh engine refuses a codec only for a second of its payload type: never, with one; and
	// TestVideo_AClientsVideoLineIsAnsweredAndNothingSentOnIt fails without it.
	_ = media.RegisterCodec(videoCodec, webrtc.RTPCodecTypeVideo) //nolint:errcheck // see above
	return webrtc.NewAPI(webrtc.WithSettingEngine(engine), webrtc.WithMediaEngine(media))
}
