// SPDX-License-Identifier: AGPL-3.0-or-later

package decide

import (
	"slices"

	"github.com/kstenerud/orpheum/internal/nodeproto"
)

// Delivery is how a file plays: its own bytes, or a prepared HLS rendition. It is the decision's
// answer, and the person's override of it.
type Delivery string

// The deliveries.
const (
	Direct Delivery = "direct"
	HLS    Delivery = "hls"
)

// Deliveries are every Delivery: the one list the API documents both the decision and the
// override with.
func Deliveries() []Delivery {
	return []Delivery{Direct, HLS}
}

// Profile is a prepared rendition's recipe (cut-a §6).
type Profile string

// The profiles: the video copied, or re-encoded to H.264 at about 8 Mb/s. Both carry the audio
// as AAC stereo, standard and boosted.
const (
	Remux Profile = "remux"
	H264  Profile = "h264"
)

// Reason says why a file is not played directly (cut-a §5.2). It is shown on screen (D188).
type Reason string

// The reasons.
const (
	ReasonContainer Reason = "container"
	// ReasonAudioCodec is a sound the device cannot play as it is: its codec, or, for AAC, its
	// channels, since the menu names aac-stereo and aac-multi apart (D21, D336 §2).
	ReasonAudioCodec Reason = "audio_codec"
	ReasonVideoCodec Reason = "video_codec"
	ReasonBandwidth  Reason = "bandwidth"
	// ReasonDialogueBoost is the person's dialogue boost, on, over a sound of more than two channels:
	// the server makes the mix that raises its centre (D222, D336 §2).
	ReasonDialogueBoost Reason = "dialogue_boost"
	// ReasonAudioTrack is an audio track a person chose that is not the one the original starts with:
	// a browser plays the original's first, and a prepared copy carries every track (D217 §3, D230).
	ReasonAudioTrack Reason = "audio_track"
	ReasonUserChoice Reason = "user_choice"
)

// Input is everything the decision reads. It holds declared capabilities and the file's own
// facts, never who is asking (D2, D103).
type Input struct {
	Probe           nodeproto.Probe
	PlayableFormats PlayableFormats
	// PreferredDelivery is a person's override of the decision (web-client.md: playback is never a
	// black box): Direct plays the original whatever it holds, HLS prepares a rendition, and empty
	// leaves the decision its own.
	PreferredDelivery Delivery
	// IsDialogueBoostOn is whether the person plays with the dialogue boost (D83): a sound of more
	// than two channels is then made by the server, its centre raised; with it off, the sound plays
	// as it is wherever the playable formats say it can (D336 §1, §2). How the device makes sound
	// plays no part (D336 §4).
	IsDialogueBoostOn bool
	// AudioStreamIndex is the stream index of the audio track a person chose (D230), or nil for the
	// one a player starts with, MainAudio.
	AudioStreamIndex *int
}

// Decision is the answer: how the file plays and why, and the recipe when it is prepared.
type Decision struct {
	Delivery Delivery
	Profile  Profile // empty for Direct
	// NotDirectReasons lists every axis that stopped direct play: container, video, audio, bandwidth,
	// boost, the chosen audio track, then a person's choice.
	NotDirectReasons []Reason
}

// Decide resolves cut-a §6's first question, whether the original plays as it is, and when it
// does not, which profile prepares it. Whether a rendition already exists is the caller's.
func Decide(input Input) Decision {
	if input.PreferredDelivery == Direct {
		return Decision{Delivery: Direct}
	}
	// Where the client is plays no part: a slow link shows itself by its stalls (D247 §11).
	reasons, videoFits := axisReasons(input)
	// The boost changes nothing on a source with no centre channel to raise.
	audio, hasAudio := playedAudio(input)
	if input.IsDialogueBoostOn && hasAudio && audio.Audio.ChannelCount > 2 {
		reasons = append(reasons, ReasonDialogueBoost)
	}
	if main, hasMain := MainAudio(input.Probe); hasAudio && hasMain && audio.StreamIndex != main.StreamIndex {
		reasons = append(reasons, ReasonAudioTrack)
	}
	if input.PreferredDelivery == HLS {
		reasons = append(reasons, ReasonUserChoice)
	}
	if len(reasons) == 0 {
		return Decision{Delivery: Direct}
	}
	profile := Remux
	if !videoFits {
		profile = H264
	}
	return Decision{Delivery: HLS, Profile: profile, NotDirectReasons: reasons}
}

// axisReasons asks the playable formats' three axes of the original: its container, its picture and the
// audio track that plays. It also says whether the picture can be copied as it is.
func axisReasons(input Input) (reasons []Reason, videoFits bool) {
	container, found := ContainerOf(input.Probe.Format)
	if !found || !slices.Contains(input.PlayableFormats.Containers, container) {
		reasons = append(reasons, ReasonContainer)
	}
	videoFits = true
	if video, present := MainVideo(input.Probe); present {
		name, placed := videoEntry(video)
		videoFits = placed && slices.Contains(input.PlayableFormats.VideoCodecs, name)
	}
	if !videoFits {
		reasons = append(reasons, ReasonVideoCodec)
	}
	if audio, present := playedAudio(input); present {
		name, placed := audioEntry(audio)
		if !placed || !slices.Contains(input.PlayableFormats.AudioCodecs, name) {
			reasons = append(reasons, ReasonAudioCodec)
		}
	}
	return reasons, videoFits
}

// playedAudio is the audio track that plays: the one a person chose, when the file has it, or the
// one a player starts with.
func playedAudio(input Input) (nodeproto.ProbeStream, bool) {
	if input.AudioStreamIndex != nil {
		for _, stream := range input.Probe.Streams {
			if stream.Kind == streamKindAudio && stream.StreamIndex == *input.AudioStreamIndex {
				return stream, true
			}
		}
	}
	return MainAudio(input.Probe)
}
