// SPDX-License-Identifier: AGPL-3.0-or-later

package hash

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func identityOf(t *testing.T, file []byte) Identity {
	t.Helper()
	identity, err := PayloadIdentity(bytes.NewReader(file), int64(len(file)))
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

// film is five clusters of 700 KB: 3.5 MB of payload, so the two 1 MB windows leave the middle
// unread, as they would in a real film.
func film() matroska {
	return matroska{title: "Short", blocks: payloadBlocks(3_500_000, 700_000), cues: true, uncuedAtEnd: 2}
}

// D179 §3's table: every in-place edit mkvpropedit made kept the payload identity.
func TestPayloadIdentity_TagEditsKeepTheIdentity(t *testing.T) {
	t.Parallel()
	original := identityOf(t, film().build())
	if original.Anchor != AnchorMatroska {
		t.Fatalf("anchored on %s, want the Matroska clusters", original.Anchor)
	}
	edits := map[string]func(m *matroska){
		"set a title of the same length": func(m *matroska) { m.title = "Title" },
		"set a long title, moved to the end": func(m *matroska) {
			m.title = string(bytes.Repeat([]byte("long "), 1200))
			m.titleMovedToEnd = true
		},
		"add tags at the end": func(m *matroska) {
			m.trailing = append(m.trailing, ebml(idTags, bytes.Repeat([]byte{0x33}, 19_000)))
		},
		"add an attachment at the end": func(m *matroska) {
			m.trailing = append(m.trailing, ebml(idAttachments, bytes.Repeat([]byte{0x77}, 300_000)))
		},
	}
	for name, edit := range edits {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			edited := film()
			edit(&edited)
			file := edited.build()
			if got := identityOf(t, file); got != original {
				t.Errorf("identity changed: %x/%d → %x/%d",
					original.PayloadHash, original.PayloadBytes, got.PayloadHash, got.PayloadBytes)
			}
			// The positive sibling: the edit did change the file, and its head and size with it.
			if bytes.Equal(file, film().build()) {
				t.Error("the edit left the file unchanged; the test proves nothing")
			}
		})
	}
}

func TestPayloadIdentity_ARemuxOrAChangedPayloadIsANewIdentity(t *testing.T) {
	t.Parallel()
	original := identityOf(t, film().build())
	remuxed := film()
	remuxed.blocks = payloadBlocks(3_500_000, 500_000) // the same media, re-laid into 7 clusters
	changedFirst := film()
	changedFirst.blocks[0] = bytes.Clone(changedFirst.blocks[0])
	changedFirst.blocks[0][10] ^= 0xFF
	changedLast := film()
	last := len(changedLast.blocks) - 1
	changedLast.blocks[last] = bytes.Clone(changedLast.blocks[last])
	changedLast.blocks[last][len(changedLast.blocks[last])-10] ^= 0xFF

	changes := map[string]matroska{"remuxed": remuxed, "first cluster": changedFirst, "last cluster": changedLast}
	for name, changed := range changes {
		if got := identityOf(t, changed.build()); got.PayloadHash == original.PayloadHash {
			t.Errorf("%s: the identity did not change", name)
		}
	}
}

func TestPayloadIdentity_WithoutCuesTheWalkFindsTheSameSpan(t *testing.T) {
	t.Parallel()
	withCues := identityOf(t, film().build())
	uncued := film()
	uncued.cues = false
	if got := identityOf(t, uncued.build()); got != withCues {
		t.Errorf("without cues %x/%d, with cues %x/%d",
			got.PayloadHash, got.PayloadBytes, withCues.PayloadHash, withCues.PayloadBytes)
	}
	// A cue pointing at the first cluster makes the walk from there find the same last cluster.
	firstOnly := film()
	firstOnly.blocks = firstOnly.blocks[:1]
	firstOnly.uncuedAtEnd = 0
	if identityOf(t, firstOnly.build()).PayloadBytes >= withCues.PayloadBytes {
		t.Error("a one-cluster file has as long a payload as a five-cluster one")
	}
}

func TestPayloadIdentity_AFileItCannotFollowIsHashedWhole(t *testing.T) {
	t.Parallel()
	complete := film().build()
	cases := map[string][]byte{
		"truncated mid-cluster": complete[:len(complete)/2],
		"a cluster of unknown size": func() []byte {
			m := matroska{title: "Live", blocks: payloadBlocks(1000, 1000)}
			file := m.build()
			at := bytes.Index(file, []byte{0x1F, 0x43, 0xB6, 0x75})
			return append(file[:at:at], withUnknownSize(file[at:], 4)...)
		}(),
		"not a container at all": bytes.Repeat([]byte("MPEG-TS-ish "), 1000),
		"empty":                  {},
	}
	for name, file := range cases {
		identity := identityOf(t, file)
		if identity.Anchor != AnchorWholeFile || identity.PayloadBytes != int64(len(file)) {
			t.Errorf("%s: anchored on %s with length %d, want the whole %d-byte file",
				name, identity.Anchor, identity.PayloadBytes, len(file))
		}
	}
	if identityOf(t, complete).PayloadHash == identityOf(t, complete[:len(complete)/2]).PayloadHash {
		t.Error("a truncated file has the complete file's identity")
	}
}

// mp4 writes a file of ftyp, moov and mdat boxes, moov before or after mdat.
func mp4(moov, mdat []byte, moovLast bool) []byte {
	boxOf := func(kind string, data []byte) []byte {
		header := binary.BigEndian.AppendUint32(nil, uint32(8+len(data))) //nolint:gosec // test sizes are small
		return append(append(header, kind...), data...)
	}
	file := boxOf("ftyp", []byte("isom\x00\x00\x02\x00"))
	if moovLast {
		return append(append(file, boxOf("mdat", mdat)...), boxOf("moov", moov)...)
	}
	return append(append(file, boxOf("moov", moov)...), boxOf("mdat", mdat)...)
}

func TestPayloadIdentity_MP4IsAnchoredOnMdat(t *testing.T) {
	t.Parallel()
	media := bytes.Join(payloadBlocks(3_000_000, 3_000_000), nil)
	original := identityOf(t, mp4([]byte("tags: old title"), media, false))
	if original.Anchor != AnchorMP4 || original.PayloadBytes != int64(len(media)) {
		t.Fatalf("anchored on %s with length %d, want mdat's %d bytes",
			original.Anchor, original.PayloadBytes, len(media))
	}
	retagged := identityOf(t, mp4(bytes.Repeat([]byte("a much longer title "), 500), media, true))
	if retagged != original {
		t.Error("rewriting and moving moov changed the identity")
	}
	changed := bytes.Clone(media)
	changed[5] ^= 0xFF
	if identityOf(t, mp4([]byte("tags: old title"), changed, false)).PayloadHash == original.PayloadHash {
		t.Error("changing the head of mdat left the identity unchanged")
	}
}

func TestPayloadIdentity_ASmallPayloadIsReadWhole(t *testing.T) {
	t.Parallel()
	small := matroska{title: "Tiny", blocks: payloadBlocks(1_500_000, 500_000), cues: true}
	original := identityOf(t, small.build())
	// Under two windows, head and tail must not overlap or skip: a change anywhere shows.
	middle := small
	middle.blocks = [][]byte{small.blocks[0], bytes.Clone(small.blocks[1]), small.blocks[2]}
	middle.blocks[1][250_000] ^= 0xFF
	if identityOf(t, middle.build()).PayloadHash == original.PayloadHash {
		t.Error("a change in the middle of a 1.5 MB payload left the identity unchanged")
	}
}

func FuzzPayload(f *testing.F) {
	f.Add(film().build()[:4096])
	f.Add(mp4([]byte("moov"), []byte("mdat"), false))
	f.Add([]byte{0x1A, 0x45, 0xDF, 0xA3, 0x80, 0x18, 0x53, 0x80, 0x67, 0xFF})
	f.Fuzz(func(t *testing.T, file []byte) {
		identity, err := PayloadIdentity(bytes.NewReader(file), int64(len(file)))
		if err != nil {
			t.Fatal(err)
		}
		if identity.PayloadBytes < 0 || identity.PayloadBytes > int64(len(file)) {
			t.Errorf("a payload of %d bytes in a %d-byte file", identity.PayloadBytes, len(file))
		}
	})
}
