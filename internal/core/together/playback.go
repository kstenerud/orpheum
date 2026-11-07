// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"context"
	"time"
)

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
	// Picture is what the player shows that its page offers to choose, its subtitles and the
	// original, which a cast's remote shows and changes (PictureChoice); nil where it does not say.
	Picture *PlaybackPicture `json:"picture,omitempty"`
}

// PlaybackSound is how a player's film sounds and what its person may choose of it, as its page
// offers: the dialogue boost, on or off, nil where the film has no boosted mix to switch to (D222,
// D336); levelling (D226); and the audio track playing among the file's own (D230 §5). A Fire TV's
// own controls are hidden while it is controlled, so its sound could only be changed after taking
// control back (the owner, VQ116's run). SoundOffsetMs is how far the device moves its sound from
// its picture, later when positive, which a remote moves for a TV that plays one of them late (the
// owner, 2026-11-06: "Put a control in watch together and cast"); nil where its page offers none.
// Channels are the levels of the sounds it mixes, each set from a cast's remote apart from the TV's
// own volume (D440 §2): the film's alone so far.
type PlaybackSound struct {
	IsBoostOn     *bool          `json:"isBoostOn,omitempty"`
	IsLevellingOn bool           `json:"isLevellingOn,omitempty"`
	AudioStreamID int64          `json:"audioStreamId,omitempty"`
	AudioTracks   []SoundTrack   `json:"audioTracks,omitempty"`
	SoundOffsetMs *int64         `json:"soundOffsetMs,omitempty"`
	Channels      []SoundChannel `json:"channels,omitempty"`
}

// SoundChannel is one sound a player mixes, its level and whether it is muted, as a cast's remote
// shows and sets it (D440 §2, §5): the film's (ChannelFilm), its level the film's volume from 0 to
// 1 as the player's own slider sets it.
type SoundChannel struct {
	Channel string  `json:"channel"`
	Level   float64 `json:"level,omitempty"`
	IsMuted bool    `json:"isMuted,omitempty"`
}

// ChannelFilm is the film's own sound among a player's channels.
const ChannelFilm = "film"

// soundChannelsMost is the most channels a player may say: the film and a session's voices, far
// past any session's.
const soundChannelsMost = 16

// isKnown is whether a channel is one a player mixes, at a level within its bounds.
func (channel SoundChannel) isKnown() bool {
	return channel.Channel == ChannelFilm && channel.Level >= 0 && channel.Level <= 1
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
	if len(sound.Channels) > soundChannelsMost {
		return false
	}
	for _, channel := range sound.Channels {
		if !channel.isKnown() {
			return false
		}
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
	// SoundChannelLevel sets one channel's level and mute on the player (Channel, Level, IsMuted),
	// for the session alone, apart from the TV's own volume (D440 §2).
	SoundChannelLevel SoundChoiceKind = "channel"
)

// SoundChoice is one choice of how a film sounds, made on a remote for the person's player.
type SoundChoice struct {
	Kind     SoundChoiceKind `json:"kind"`
	IsOn     bool            `json:"isOn,omitempty"`
	StreamID int64           `json:"streamId,omitempty"`
	Ms       int64           `json:"ms,omitempty"`
	Channel  string          `json:"channel,omitempty"`
	Level    float64         `json:"level,omitempty"`
	IsMuted  bool            `json:"isMuted,omitempty"`
}

// isKnown is whether a choice is one a remote may make: a known kind, a track by a stream, a
// channel the player mixes at a level within its bounds.
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
	case SoundChannelLevel:
		return SoundChannel{Channel: choice.Channel, Level: choice.Level, IsMuted: choice.IsMuted}.isKnown()
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
		playback.QualityKbps >= 0 && playback.LinkKbps >= 0 && playback.Sound.isKnown() &&
		playback.Picture.isKnown()
}

// takeAside takes what a device says that moves no one's place: a note logged, its standing, a sound
// chosen for its person's player, or how it plays said alone, as a cast's receiver says it in the
// cast's session while it leads its person's Watch Together session (D434 §1).
func (current *session) takeAside(ctx context.Context, now time.Time, who *member, from *device, message Inbound) {
	switch message.Type {
	case MessageNote:
		current.logNote(ctx, now, who, from, message.Note)
	case MessageStanding:
		current.takeStanding(ctx, who, from, message.Standing)
	case MessageSound:
		current.relaySound(ctx, who, from, message.Sound)
	case MessagePicture:
		current.relayPicture(ctx, who, from, message.Picture)
	case MessagePlayback:
		from.takeState(message)
		current.sendRosterIfChanged()
	case MessageHello, MessagePing, MessageState, MessageCommand, MessageSuspend, MessageResume,
		MessageRTC, MessageWelcome, MessagePong, MessageGroup, MessageRoster, MessageRejected, MessageEnded:
		// Handle takes these itself, never here.
	}
}

// relaySound passes a member's choice of their film's sound to their own other devices that play
// it, which carry it out as their own press; in a cast, to its receiver from its remote, whatever its
// part there, as the receiver that leads its person's Watch Together session is only a panel in the
// cast's (D364 §11); another member's devices are never told, and a choice no remote may make is
// refused.
func (current *session) relaySound(ctx context.Context, who *member, from *device, choice *SoundChoice) {
	if !choice.isKnown() {
		from.send(Outbound{V: ProtocolVersion, Type: MessageRejected, Code: codeInvalid})
		return
	}
	current.logger.InfoContext(ctx, "watch together: a remote chose its film's sound",
		append(current.deviceAttributes(who, from),
			"kind", string(choice.Kind), "isOn", choice.IsOn, "streamId", choice.StreamID, "ms", choice.Ms,
			"channel", choice.Channel, "level", choice.Level, "isMuted", choice.IsMuted)...)
	current.relayToOwnPlayers(who, from, Outbound{
		V: ProtocolVersion, Type: MessageSound, FromDeviceID: from.id, Sound: choice,
	})
}

// relayToOwnPlayers sends a remote's choice, relayed, to its member's other devices that play the
// film: in a cast, to its receiver from its remote, whatever its part there, as the receiver that
// leads its person's Watch Together session is only a panel in the cast's (D364 §11); another
// member's devices are never told.
func (current *session) relayToOwnPlayers(who *member, from *device, relayed Outbound) {
	for _, each := range who.devices {
		if each.id != from.id && (each.role.plays() || current.isCast) {
			each.send(relayed)
		}
	}
}
