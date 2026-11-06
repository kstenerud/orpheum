// SPDX-License-Identifier: AGPL-3.0-or-later

package together

// Playback is how a device's player plays the film, as it says in its state: the delivery, "direct"
// for the file as it is or "converted" for its ladder, the quality it plays in kb/s, and the link it
// estimated as it chose that quality, in kb/s, each 0 while unknown. The owner: "The controller side
// should get information about playback, such as the transcoding and bandwidth choices" (D371 §4).
type Playback struct {
	Delivery    string `json:"delivery"`
	QualityKbps int64  `json:"qualityKbps,omitempty"`
	LinkKbps    int64  `json:"linkKbps,omitempty"`
	// Sound is how the player's film sounds, the choices its page offers, which a cast's remote shows
	// and changes (SoundChoice); nil where the player does not say.
	Sound *PlaybackSound `json:"sound,omitempty"`
}

// PlaybackSound is how a player's film sounds and what its person may choose of it, as its page
// offers: the dialogue boost, on or off, nil where the film has no boosted mix to switch to (D222,
// D336); levelling (D226); and the audio track playing among the file's own (D230 §5). A Fire TV's
// own controls are hidden while it is controlled, so its sound could only be changed after taking
// control back (the owner, VQ116's run). SoundOffsetMs is how far the device moves its sound from
// its picture, later when positive, which a remote moves for a TV that plays one of them late (the
// owner, 2026-11-06: "Put a control in watch together and cast"); nil where its page offers none.
type PlaybackSound struct {
	IsBoostOn     *bool        `json:"isBoostOn,omitempty"`
	IsLevellingOn bool         `json:"isLevellingOn,omitempty"`
	AudioStreamID int64        `json:"audioStreamId,omitempty"`
	AudioTracks   []SoundTrack `json:"audioTracks,omitempty"`
	SoundOffsetMs *int64       `json:"soundOffsetMs,omitempty"`
}

// SoundTrack is one of a file's own audio tracks as a remote names it: its stream, its language
// (BCP 47, empty where nothing names one), its container's title, and its kind ("plain",
// "commentary" or "described").
type SoundTrack struct {
	StreamID       int64  `json:"streamId"`
	Language       string `json:"language,omitempty"`
	ContainerTitle string `json:"containerTitle,omitempty"`
	Kind           string `json:"kind,omitempty"`
}

// The bounds of a sound a player may say, far past any file's: a film's tracks, a language tag's
// length and a container title's.
const (
	soundTracksMost      = 64
	soundLanguageMost    = 35
	soundTrackTitleMost  = 200
	soundTrackKindPlain  = "plain"
	soundTrackCommentary = "commentary"
	soundTrackDescribed  = "described"
	// The furthest a device's sound moves from its picture either way, as a
	// device's own setting is bounded (PUT /me/devices/{id}).
	soundOffsetMostMs = 2000
)

// isKnown is whether a player's sound is one it may say: within the bounds, of known kinds.
func (sound *PlaybackSound) isKnown() bool {
	if sound == nil {
		return true
	}
	if sound.AudioStreamID < 0 || len(sound.AudioTracks) > soundTracksMost {
		return false
	}
	if sound.SoundOffsetMs != nil && !isSoundOffset(*sound.SoundOffsetMs) {
		return false
	}
	for _, track := range sound.AudioTracks {
		switch track.Kind {
		case "", soundTrackKindPlain, soundTrackCommentary, soundTrackDescribed:
		default:
			return false
		}
		if track.StreamID < 0 || len(track.Language) > soundLanguageMost ||
			len(track.ContainerTitle) > soundTrackTitleMost {
			return false
		}
	}
	return true
}

// SoundChoiceKind is what of a film's sound a choice changes.
type SoundChoiceKind string

// The choices of a film's sound.
const (
	// SoundBoost turns the dialogue boost on or off (IsOn).
	SoundBoost SoundChoiceKind = "boost"
	// SoundLevelling turns levelling on or off (IsOn).
	SoundLevelling SoundChoiceKind = "levelling"
	// SoundAudioTrack plays another of the file's audio tracks (StreamID).
	SoundAudioTrack SoundChoiceKind = "audioTrack"
	// SoundOffset moves the device's sound from its picture (Ms).
	SoundOffset SoundChoiceKind = "soundOffset"
)

// SoundChoice is one choice of how a film sounds, made on a remote for the person's player.
type SoundChoice struct {
	Kind     SoundChoiceKind `json:"kind"`
	IsOn     bool            `json:"isOn,omitempty"`
	StreamID int64           `json:"streamId,omitempty"`
	Ms       int64           `json:"ms,omitempty"`
}

// isKnown is whether a choice is one a remote may make: a known kind, a track by a stream.
func (choice *SoundChoice) isKnown() bool {
	if choice == nil {
		return false
	}
	switch choice.Kind {
	case SoundBoost, SoundLevelling:
		return true
	case SoundAudioTrack:
		return choice.StreamID > 0
	case SoundOffset:
		return isSoundOffset(choice.Ms)
	}
	return false
}

// isSoundOffset is whether ms is a move of a device's sound it may make, within the bound.
func isSoundOffset(ms int64) bool {
	return ms >= -soundOffsetMostMs && ms <= soundOffsetMostMs
}

// The deliveries a player says it plays by.
const (
	DeliveryDirect    = "direct"
	DeliveryConverted = "converted"
)

// isKnown is whether a player's playback is one a device may say: a known delivery, and no negative
// quality or link.
func (playback Playback) isKnown() bool {
	return (playback.Delivery == DeliveryDirect || playback.Delivery == DeliveryConverted) &&
		playback.QualityKbps >= 0 && playback.LinkKbps >= 0 && playback.Sound.isKnown()
}
