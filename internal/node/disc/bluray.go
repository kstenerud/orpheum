// SPDX-License-Identifier: AGPL-3.0-or-later

package disc

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// A Blu-ray's clock: playlist times are in 45 kHz ticks.
const blurayTicksPerMs = 45

// maxPlayItems bounds a playlist's play items; the format allows 999.
const maxPlayItems = 999

// The stream coding types of a playlist's stream table whose languages the node reads (Blu-ray's
// STN_table): presentation graphics and text subtitles; every other coding with a language is a
// sound.
const (
	codingPresentationGraphics = 0x90
	codingTextSubtitle         = 0x92
)

// playlist is an MPLS file: its number, its play items and its chapter marks.
type playlist struct {
	number int
	items  []playItem
	marks  []mark
}

// playItem is a clip played between two times, and the languages of the streams it plays.
type playItem struct {
	clip            string
	inTime, outTime int64
	tracks          []Track
}

// mark is a chapter: the play item it is in, and its time there.
type mark struct {
	item int
	time int64
}

// durationMs is how long the playlist plays.
func (list playlist) durationMs() int64 {
	var ticks int64
	for _, item := range list.items {
		ticks += item.outTime - item.inTime
	}
	return ticks / blurayTicksPerMs
}

// isLoop is whether the playlist plays a clip more than once, as the playlists that obfuscate a
// disc's real one, or loop a menu, do.
func (list playlist) isLoop() bool {
	clips := map[string]bool{}
	for _, item := range list.items {
		if clips[item.clip] {
			return true
		}
		clips[item.clip] = true
	}
	return false
}

// blurayMainTitle is a Blu-ray's longest playlist that plays no clip twice.
func blurayMainTitle(disc *volume) (Title, error) {
	entries, err := disc.children("BDMV/PLAYLIST")
	if err != nil {
		return Title{}, err
	}
	names := make([]string, 0, len(entries))
	for name := range entries {
		if strings.HasSuffix(name, ".MPLS") {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	if len(names) > maxPlaylists {
		return Title{}, fmt.Errorf("%w: %d playlists", ErrMalformed, len(names))
	}
	var best playlist
	var bestMs int64 = -1
	for _, name := range names {
		list, err := disc.readPlaylist(entries, name)
		if errors.Is(err, ErrMalformed) {
			continue // a broken playlist is not the film; the others may be
		}
		if err != nil {
			return Title{}, err
		}
		if ms := list.durationMs(); !list.isLoop() && ms > bestMs {
			best, bestMs = list, ms
		}
	}
	if bestMs <= 0 {
		return Title{}, fmt.Errorf("%w: no playlist plays anything", ErrNoTitle)
	}
	return disc.assemblePlaylist(best)
}

// readPlaylist reads and parses the playlist named name among entries.
func (disc *volume) readPlaylist(entries map[string]longAD, name string) (playlist, error) {
	data, err := disc.readEntry(entries, name)
	if err != nil {
		return playlist{}, err
	}
	number, err := strconv.Atoi(strings.TrimSuffix(name, ".MPLS"))
	if err != nil {
		return playlist{}, fmt.Errorf("%w: a playlist named %s", ErrMalformed, name)
	}
	return parsePlaylist(fieldsOf(data), number)
}

// parsePlaylist reads an MPLS file: its play items (PlayList) and its entry marks (PlayListMark).
func parsePlaylist(file *field, number int) (playlist, error) {
	if string(file.bytes(0, 4)) != "MPLS" {
		return playlist{}, fmt.Errorf("%w: playlist %05d is not an MPLS file", ErrMalformed, number)
	}
	list := playlist{number: number}
	listAt, marksAt := int(file.be32(8)), int(file.be32(12))
	offset := listAt + 10
	for range min(file.be16(listAt+6), maxPlayItems) {
		list.items = append(list.items, parsePlayItem(file, offset, len(list.items) == 0))
		offset += 2 + file.be16(offset)
	}
	for index := range file.be16(marksAt + 4) {
		entry := marksAt + 6 + 14*index
		if file.u8(entry+1) == 1 { // an entry mark: a chapter
			list.marks = append(list.marks, mark{item: file.be16(entry + 2), time: file.be32(entry + 4)})
		}
	}
	for _, item := range list.items {
		if item.outTime < item.inTime {
			return playlist{}, fmt.Errorf("%w: a play item that ends before it starts", ErrMalformed)
		}
	}
	return list, file.err(fmt.Sprintf("playlist %05d", number))
}

// parsePlayItem reads the play item at offset: its clip and times, and, for the first, which alone
// the title takes its languages from, its stream table.
func parsePlayItem(file *field, offset int, isFirst bool) playItem {
	item := playItem{
		clip:    strings.ToUpper(string(file.bytes(offset+2, 5))),
		inTime:  file.be32(offset + 14),
		outTime: file.be32(offset + 18),
	}
	table := offset + 34
	if (file.be16(offset+11)>>4)&1 == 1 { // multi-angle: its other angles' clips come first
		table = offset + 36 + 10*max(file.u8(offset+34)-1, 0)
	}
	if isFirst {
		item.tracks = parseStreamTable(file, table)
	}
	return item
}

// parseStreamTable reads a play item's STN_table for its sounds' and subtitles' PIDs and languages.
func parseStreamTable(file *field, table int) []Track {
	videos, sounds, subtitles := file.u8(table+4), file.u8(table+5), file.u8(table+6)
	offset := table + 16
	var tracks []Track
	for index := range videos + sounds + subtitles {
		pid, coding, language := parseStreamEntry(file, offset)
		offset = nextStreamEntry(file, offset)
		switch {
		case index < videos:
		case index < videos+sounds:
			tracks = append(tracks, Track{ID: int64(pid), Kind: "audio", Language: language})
		case coding == codingPresentationGraphics || coding == codingTextSubtitle:
			tracks = append(tracks, Track{ID: int64(pid), Kind: "subtitle", Language: language})
		}
	}
	return tracks
}

// parseStreamEntry reads one stream_entry and its stream_attributes at offset: the stream's PID,
// its coding type and its language.
func parseStreamEntry(file *field, offset int) (pid, coding int, language string) {
	switch file.u8(offset + 1) {
	case 1:
		pid = file.be16(offset + 2)
	case 3:
		pid = file.be16(offset + 3)
	default:
		pid = file.be16(offset + 4)
	}
	attributes := offset + 1 + file.u8(offset)
	coding = file.u8(attributes + 1)
	switch coding {
	case codingPresentationGraphics:
		language = threeLetters(file.bytes(attributes+2, 3))
	case codingTextSubtitle:
		language = threeLetters(file.bytes(attributes+3, 3))
	case 0x01, 0x02, 0x1B, 0xEA, 0x24, 0x20:
	default:
		language = threeLetters(file.bytes(attributes+3, 3))
	}
	return pid, coding, language
}

// nextStreamEntry is where the stream_entry after the one at offset starts: past its entry and
// its attributes, each led by its length.
func nextStreamEntry(file *field, offset int) int {
	attributes := offset + 1 + file.u8(offset)
	return attributes + 1 + file.u8(attributes)
}

// threeLetters is an ISO 639-2 code as a Blu-ray writes it, or "" for anything else.
func threeLetters(code []byte) string {
	for _, letter := range code {
		if letter < 'a' || letter > 'z' {
			return ""
		}
	}
	return string(code)
}

// assemblePlaylist makes the playlist's title: its clips' files whole, in play order, its chapters
// at its entry marks, and the languages of its first play item's streams.
func (disc *volume) assemblePlaylist(list playlist) (Title, error) {
	streams, err := disc.children("BDMV/STREAM")
	if err != nil {
		return Title{}, err
	}
	title := Title{Kind: BluRay, Number: list.number, DurationMs: list.durationMs(), ChaptersMs: []int64{0}}
	itemStartMs := make([]int64, 0, len(list.items))
	var startMs int64
	for _, item := range list.items {
		icb, found := streams[item.clip+".M2TS"]
		if !found {
			return Title{}, fmt.Errorf("%w: no clip %s.m2ts", ErrMalformed, item.clip)
		}
		clip, err := disc.readFileEntry(icb)
		if err != nil {
			return Title{}, err
		}
		title.Extents = appendJoined(title.Extents, clip.extents...)
		itemStartMs = append(itemStartMs, startMs)
		startMs += (item.outTime - item.inTime) / blurayTicksPerMs
	}
	for _, chapter := range list.marks {
		if chapter.item < len(list.items) && chapter.time >= list.items[chapter.item].inTime {
			ms := itemStartMs[chapter.item] + (chapter.time-list.items[chapter.item].inTime)/blurayTicksPerMs
			if ms < title.DurationMs {
				title.ChaptersMs = append(title.ChaptersMs, ms)
			}
		}
	}
	slices.Sort(title.ChaptersMs)
	title.ChaptersMs = slices.Compact(title.ChaptersMs)
	title.Tracks = list.items[0].tracks
	return title, nil
}
