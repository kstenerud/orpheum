// SPDX-License-Identifier: AGPL-3.0-or-later

package disctest

import (
	"fmt"
	"slices"
)

// blurayTicksAt is where a playlist's times start, as a disc's often do (in 45 kHz ticks): an item's
// in time is never 0, so a reader that forgets to subtract it is caught.
const blurayTicksAt = 27_000_000

// BluRayItem is a play item: a clip ("00001") played between two times, in milliseconds of it.
type BluRayItem struct {
	Clip         string
	InMs, OutMs  int64
	IsMultiAngle bool
}

// BluRayStream is a sound or a subtitle a playlist's stream table lists: its PID, its coding type
// (0x81 AC-3, 0x83 TrueHD, 0x90 presentation graphics), and its language.
type BluRayStream struct {
	PID      int
	Coding   byte
	Language string
}

// BluRayPlaylist is an MPLS file: its number, items, chapters (as milliseconds into an item) and
// the streams its items play.
type BluRayPlaylist struct {
	Number   int
	Items    []BluRayItem
	Chapters [][2]int64 // {item, ms into the item}
	Sounds   []BluRayStream
	Graphics []BluRayStream
}

// BluRay is a Blu-ray's BDMV: its playlists and its clips' bytes, by name ("00001").
type BluRay struct {
	Playlists []BluRayPlaylist
	Clips     map[string][]byte
}

// Files are the Blu-ray's BDMV files: each playlist and each clip.
func (bluRay BluRay) Files() []File {
	files := []File{{Path: "BDMV/index.bdmv", Data: []byte("INDX0200")}}
	for _, list := range bluRay.Playlists {
		files = append(files, File{Path: fmt.Sprintf("BDMV/PLAYLIST/%05d.mpls", list.Number), Data: list.mpls()})
	}
	names := make([]string, 0, len(bluRay.Clips))
	for name := range bluRay.Clips {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		files = append(files, File{Path: "BDMV/STREAM/" + name + ".m2ts", Data: bluRay.Clips[name]})
	}
	return files
}

// mpls writes the playlist: its header, PlayList and PlayListMark.
func (list BluRayPlaylist) mpls() []byte {
	data := []byte("MPLS0200")
	data = appendBE32(data, 40)
	data = appendBE32(data, 0) // the marks' place, below
	data = append(data, make([]byte, 40-len(data))...)
	var items []byte
	for _, item := range list.Items {
		items = append(items, list.playItem(item)...)
	}
	body := appendBE16([]byte{0, 0}, len(list.Items))
	body = append(append(body, 0, 0), items...)
	data = appendBE32(data, len(body))
	data = append(data, body...)
	putBE32(data[12:], len(data))
	marks := appendBE16(nil, len(list.Chapters))
	for _, chapter := range list.Chapters {
		item := list.Items[chapter[0]]
		marks = append(marks, 0, 1)
		marks = appendBE16(marks, int(chapter[0]))
		marks = appendBE32(marks, int(blurayTicksAt+(item.InMs+chapter[1])*45))
		marks = append(marks, 0xFF, 0xFF, 0, 0, 0, 0)
	}
	data = appendBE32(data, len(marks))
	return append(data, marks...)
}

// playItem writes one play item, with the playlist's stream table.
func (list BluRayPlaylist) playItem(item BluRayItem) []byte {
	body := append([]byte(item.Clip), "M2TS"...)
	flags := 0
	if item.IsMultiAngle {
		flags = 1 << 4
	}
	body = appendBE16(body, flags)
	body = append(body, 0)
	body = appendBE32(body, int(blurayTicksAt+item.InMs*45))
	body = appendBE32(body, int(blurayTicksAt+item.OutMs*45))
	body = append(body, make([]byte, 12)...) // UO mask, random access, still
	if item.IsMultiAngle {
		body = append(body, 2, 0)
		body = append(body, item.Clip+"M2TS\x00"...) // angle 2 plays the same clip
	}
	body = append(body, list.streamTable()...)
	return append(appendBE16(nil, len(body)), body...)
}

// streamTable writes the STN_table: one H.264 picture at PID 0x1011, then the sounds and graphics.
func (list BluRayPlaylist) streamTable() []byte {
	table := []byte{0, 0, 1, octet(len(list.Sounds)), octet(len(list.Graphics)), 0, 0, 0, 0, 0, 0, 0, 0, 0}
	table = append(table, streamEntry(0x1011)...)
	table = append(table, 5, 0x1B, 0x61, 0, 0, 0)
	for _, sound := range list.Sounds {
		table = append(table, streamEntry(sound.PID)...)
		table = append(append(table, 5, sound.Coding, 0x61), sound.Language...)
	}
	for _, graphics := range list.Graphics {
		table = append(table, streamEntry(graphics.PID)...)
		table = append(append(table, 5, graphics.Coding), graphics.Language...)
		table = append(table, 0)
	}
	return append(appendBE16(nil, len(table)), table...)
}

// streamEntry is a stream_entry of a stream of the main clip at pid.
func streamEntry(pid int) []byte {
	return append(appendBE16([]byte{9, 1}, pid), 0, 0, 0, 0, 0, 0)
}

// M2TS is n packets of an M2TS stream holding text, each a 4-byte header with its copy permission
// bits clear and a transport packet starting with its sync byte: what a decrypted clip looks like
// to a check for AACS, served back to a test to see which clips it was given.
func M2TS(text string, n int) []byte {
	data := make([]byte, n*192)
	for index := range n {
		packet := data[index*192:]
		packet[4] = 0x47
		copy(packet[8:], fmt.Sprintf("%s:%d|", text, index))
	}
	return data
}
