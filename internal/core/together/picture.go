// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import "context"

// PlaybackPicture is what a player shows over and of its film that its person may choose, as its
// page offers: the subtitle track showing, by the key its page names it by ("" for none), among the
// tracks the film offers, each with the name its page gives it; and whether its page offers the
// film's own file in place of its ladder (D418 §4), the one quality its person chooses. A cast's
// remote shows and changes them, the receiver carrying each choice out as its own press
// (PictureChoice; D440 §5). SubtitleOffsetMs is how far its subtitles are moved from the film, later
// when positive, as the player's subtitle timing sets it.
type PlaybackPicture struct {
	SubtitleKey      string           `json:"subtitleKey,omitempty"`
	SubtitleOffsetMs int64            `json:"subtitleOffsetMs,omitempty"`
	Subtitles        []SubtitleOption `json:"subtitles,omitempty"`
	CanPlayOriginal  bool             `json:"canPlayOriginal,omitempty"`
}

// SubtitleOption is one subtitle track a player offers, as a remote names it: its page's key for it
// and the name its page gives it, in its person's language.
type SubtitleOption struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// The bounds of a picture a player may say, far past any film's: its subtitle tracks, a key's
// length and a name's.
const (
	pictureSubtitlesMost = 64
	subtitleKeyMost      = 40
	subtitleLabelMost    = 200
	// The furthest a film's subtitles move either way, as the server keeps a file's subtitle
	// offset (the web's longestSubtitleOffsetMs).
	subtitleOffsetMostMs = 600_000
)

// isKnown is whether a player's picture is one it may say: within the bounds.
func (picture *PlaybackPicture) isKnown() bool {
	if picture == nil {
		return true
	}
	if len(picture.SubtitleKey) > subtitleKeyMost || len(picture.Subtitles) > pictureSubtitlesMost ||
		!isSubtitleOffset(picture.SubtitleOffsetMs) {
		return false
	}
	for _, each := range picture.Subtitles {
		if each.Key == "" || len(each.Key) > subtitleKeyMost || len(each.Label) > subtitleLabelMost {
			return false
		}
	}
	return true
}

// PictureChoiceKind is what of a film's picture a choice changes.
type PictureChoiceKind string

// The choices of a film's picture.
const (
	// PictureSubtitles shows the subtitle track Key, or none for "".
	PictureSubtitles PictureChoiceKind = "subtitles"
	// PictureOriginal plays the film's own file in place of its ladder (D418 §4).
	PictureOriginal PictureChoiceKind = "original"
	// PictureSubtitleOffset moves the subtitles Ms from the film, later when positive, as the
	// player's subtitle timing does.
	PictureSubtitleOffset PictureChoiceKind = "subtitleOffset"
	// PictureSubtitleFetched says a subtitle was fetched for the film playing, which its player
	// reads its tracks again to offer (Find subtitles, D224).
	PictureSubtitleFetched PictureChoiceKind = "subtitleFetched"
)

// PictureChoice is one choice of what a film shows, made on a remote for the person's player.
type PictureChoice struct {
	Kind PictureChoiceKind `json:"kind"`
	Key  string            `json:"key,omitempty"`
	Ms   int64             `json:"ms,omitempty"`
}

// isKnown is whether a choice is one a remote may make: a known kind, a key within the bound.
func (choice *PictureChoice) isKnown() bool {
	if choice == nil {
		return false
	}
	switch choice.Kind {
	case PictureSubtitles:
		return len(choice.Key) <= subtitleKeyMost && choice.Ms == 0
	case PictureSubtitleOffset:
		return choice.Key == "" && isSubtitleOffset(choice.Ms)
	case PictureOriginal, PictureSubtitleFetched:
		return choice.Key == "" && choice.Ms == 0
	}
	return false
}

// isSubtitleOffset is whether ms is a move of a film's subtitles a player may make, within the bound.
func isSubtitleOffset(ms int64) bool {
	return ms >= -subtitleOffsetMostMs && ms <= subtitleOffsetMostMs
}

// relayPicture passes a member's choice of what their film shows to their own other devices that
// play it, as relaySound passes a sound's (relayToOwnPlayers); a choice no remote may make is
// refused.
func (current *session) relayPicture(ctx context.Context, who *member, from *device, choice *PictureChoice) {
	if !choice.isKnown() {
		from.send(Outbound{V: ProtocolVersion, Type: MessageRejected, Code: codeInvalid})
		return
	}
	current.logger.InfoContext(ctx, "watch together: a remote chose its film's picture",
		append(current.deviceAttributes(who, from), "kind", string(choice.Kind), "key", choice.Key, "ms", choice.Ms)...)
	current.relayToOwnPlayers(who, from, Outbound{
		V: ProtocolVersion, Type: MessagePicture, FromDeviceID: from.id, Picture: choice,
	})
}
