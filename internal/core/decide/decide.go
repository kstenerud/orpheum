// SPDX-License-Identifier: GPL-3.0-or-later

package decide

import (
	"slices"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// Kind is how a file plays: its own bytes, or a prepared HLS rendition.
type Kind string

// The kinds.
const (
	Direct Kind = "direct"
	HLS    Kind = "hls"
)

// Profile is a prepared rendition's recipe (cut-a §6).
type Profile string

// The profiles: the video copied, or re-encoded to H.264 at about 8 Mb/s. Both carry the audio
// as AAC stereo, standard and boosted.
const (
	Remux    Profile = "remux"
	H264At8M Profile = "h264-8m"
)

// Reason says why a file is not played directly (cut-a §5.2). It is shown on screen (D188).
type Reason string

// The reasons.
const (
	ReasonContainer     Reason = "container"
	ReasonAudioCodec    Reason = "audio_codec"
	ReasonVideoCodec    Reason = "video_codec"
	ReasonBandwidth     Reason = "bandwidth"
	ReasonDialogueBoost Reason = "dialogue_boost"
	ReasonUserChoice    Reason = "user_choice"
)

// Prefer is a person's override of the decision (web-client.md: playback is never a black box).
type Prefer string

// The overrides: none, the original whatever it holds, or a prepared rendition.
const (
	PreferAuto    Prefer = ""
	PreferDirect  Prefer = "direct"
	PreferPrepare Prefer = "prepare"
)

// Link is where the client is: on the house's network, or outside it.
type Link string

// The links.
const (
	Local  Link = "local"
	Remote Link = "remote"
)

// Input is everything the decision reads. It holds declared capabilities and the file's own
// facts, never who is asking (D2, D103).
type Input struct {
	Probe  nodeproto.Probe
	Menu   Menu
	Link   Link
	Prefer Prefer
	// RemoteCeilingKbps is the bit rate above which a file is re-encoded for a remote link.
	RemoteCeilingKbps int64
	// Boost is whether the dialogue boost is wanted on this device's sound: on, and the device
	// on stereo speakers (cut-a §6, D184).
	Boost bool
}

// Decision is the answer: how the file plays and why, and the recipe when it is prepared.
type Decision struct {
	Kind    Kind
	Profile Profile // empty for Direct
	// Reasons lists every axis that stopped direct play: container, video, audio, bandwidth,
	// boost, then a person's choice.
	Reasons []Reason
}

// Decide resolves cut-a §6's first question, whether the original plays as it is, and when it
// does not, which profile prepares it. Whether a rendition already exists is the caller's.
func Decide(input Input) Decision {
	if input.Prefer == PreferDirect {
		return Decision{Kind: Direct}
	}
	reasons, videoFits := axisReasons(input)
	tooFast := input.Link == Remote && input.RemoteCeilingKbps > 0 &&
		input.Probe.Format.BitRate > input.RemoteCeilingKbps*1000
	if tooFast {
		reasons = append(reasons, ReasonBandwidth)
	}
	// The boost changes nothing on a source with no centre channel to raise.
	if audio, hasAudio := mainAudio(input.Probe); input.Boost && hasAudio && audio.Audio.Channels > 2 {
		reasons = append(reasons, ReasonDialogueBoost)
	}
	if input.Prefer == PreferPrepare {
		reasons = append(reasons, ReasonUserChoice)
	}
	if len(reasons) == 0 {
		return Decision{Kind: Direct}
	}
	profile := Remux
	if !videoFits || tooFast {
		profile = H264At8M
	}
	return Decision{Kind: HLS, Profile: profile, Reasons: reasons}
}

// axisReasons asks the menu's three axes of the original: its container, its picture and the
// track a player starts with. It also says whether the picture can be copied as it is.
func axisReasons(input Input) (reasons []Reason, videoFits bool) {
	container, found := ContainerOf(input.Probe.Format)
	if !found || !slices.Contains(input.Menu.Containers, container) {
		reasons = append(reasons, ReasonContainer)
	}
	videoFits = true
	if video, present := mainVideo(input.Probe); present {
		name, placed := videoName(video)
		videoFits = placed && slices.Contains(input.Menu.Video, name)
	}
	if !videoFits {
		reasons = append(reasons, ReasonVideoCodec)
	}
	if audio, present := mainAudio(input.Probe); present {
		name, placed := audioName(audio)
		if !placed || !slices.Contains(input.Menu.Audio, name) {
			reasons = append(reasons, ReasonAudioCodec)
		}
	}
	return reasons, videoFits
}
