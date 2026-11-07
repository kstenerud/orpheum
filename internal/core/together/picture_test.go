// SPDX-License-Identifier: AGPL-3.0-or-later

package together

import (
	"reflect"
	"strings"
	"testing"
)

// A cast's remote chooses the receiver's subtitles and the original (D440 §5): each choice reaches
// the person's own player, from the remote, and no one else's; a choice no remote may make is
// refused.
func TestCast_TheRemoteChoosesTheReceiversPicture(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	cast, err := stage.hub.OpenCast(stage.owner)
	if err != nil {
		t.Fatal(err)
	}
	_, receiverBox := stage.attach(t, cast.ReceiverCredential, RolePanel)
	remote, remoteBox := stage.attach(t, cast.SenderCredential, RolePanel)
	_, otherBox := stage.attach(t, stage.opened.MembershipCredential, RoleVideo)
	chosen := []PictureChoice{
		{Kind: PictureSubtitles, Key: "s3"},
		{Kind: PictureSubtitles}, // off
		{Kind: PictureOriginal},
	}
	for _, choice := range chosen {
		stage.hub.Handle(t.Context(), remote, Inbound{Type: MessagePicture, Picture: &choice})
		got := receiverBox.last(MessagePicture)
		if got.Picture == nil || *got.Picture != choice || got.FromDeviceID != remote.DeviceID {
			t.Errorf("the receiver was relayed %+v from %d; want %+v from the remote", got.Picture,
				got.FromDeviceID, choice)
		}
	}
	refused := []*PictureChoice{
		nil,
		{Kind: "zoom"},
		{Kind: PictureOriginal, Key: "s3"},
		{Kind: PictureSubtitles, Key: strings.Repeat("s", subtitleKeyMost+1)},
	}
	for _, each := range refused {
		stage.hub.Handle(t.Context(), remote, Inbound{Type: MessagePicture, Picture: each})
	}
	if count := receiverBox.count(MessagePicture); count != len(chosen) {
		t.Errorf("the receiver was told %d picture choices; want the %d a remote may make", count, len(chosen))
	}
	if count := remoteBox.count(MessageRejected); count != len(refused) {
		t.Errorf("the remote's unknown choices were refused %d times; want %d", count, len(refused))
	}
	if count := remoteBox.count(MessagePicture) + otherBox.count(MessagePicture); count != 0 {
		t.Errorf("the remote and a session outside the cast were told %d picture choices; want none", count)
	}
}

// A receiver's playback may say what it shows that its person may choose, its subtitles and the
// original, which the roster shows its remote; a picture past the bounds is not taken, the last
// known kept.
func TestHub_TheRosterSaysWhatEachPlayerShows(t *testing.T) {
	t.Parallel()
	stage := newStage(t)
	cast, err := stage.hub.OpenCast(stage.owner)
	if err != nil {
		t.Fatal(err)
	}
	receiver, _ := stage.attach(t, cast.ReceiverCredential, RolePanel)
	_, remoteBox := stage.attach(t, cast.SenderCredential, RolePanel)
	shown := func() *PlaybackPicture {
		for _, each := range remoteBox.last(MessageRoster).Roster {
			for _, attached := range each.Devices {
				if attached.ID == receiver.DeviceID && attached.Playback != nil {
					return attached.Playback.Picture
				}
			}
		}
		return nil
	}
	picture := PlaybackPicture{
		SubtitleKey:     "f7",
		Subtitles:       []SubtitleOption{{Key: "s3", Label: "English"}, {Key: "f7", Label: "Deutsch (fetched)"}},
		CanPlayOriginal: true,
	}
	stage.hub.Handle(t.Context(), receiver, Inbound{
		Type: MessagePlayback, Playback: &Playback{Delivery: DeliveryConverted, Picture: &picture},
	})
	if got := shown(); got == nil || !reflect.DeepEqual(*got, picture) {
		t.Fatalf("the remote's roster says the receiver shows %+v; want %+v", got, picture)
	}
	for _, past := range []PlaybackPicture{
		{Subtitles: []SubtitleOption{{Key: "", Label: "No key"}}},
		{Subtitles: []SubtitleOption{{Key: "s1", Label: strings.Repeat("x", subtitleLabelMost+1)}}},
		{SubtitleKey: strings.Repeat("s", subtitleKeyMost+1)},
		{Subtitles: make([]SubtitleOption, pictureSubtitlesMost+1)},
	} {
		stage.hub.Handle(t.Context(), receiver, Inbound{
			Type: MessagePlayback, Playback: &Playback{Delivery: DeliveryConverted, Picture: &past},
		})
		if got := shown(); got == nil || !reflect.DeepEqual(*got, picture) {
			t.Errorf("after a picture past the bounds, %+v, the receiver shows %+v; want the last known", past, got)
		}
	}
}
